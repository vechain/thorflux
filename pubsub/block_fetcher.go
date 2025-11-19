package pubsub

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"github.com/vechain/thorflux/stats/slots"

	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/common"
	"github.com/vechain/thorflux/stats/pos"
	"github.com/vechain/thorflux/types"
	"golang.org/x/sync/singleflight"
)

const cacheSize = 100

type BlockFetcher struct {
	client              *thorclient.Client
	cache               *lru.Cache[uint32, *FetchResult]
	hayabusaForkedBlock uint32
	sf                  singleflight.Group
	hayabusaActiveBlock uint32
}

type FetchResult struct {
	Block      *api.JSONExpandedBlock
	Seed       []byte
	Staker     *types.StakerInformation
	AuthNodes  types.AuthorityNodeList
	FutureSeed []byte
}

func NewBlockFetcher(client *thorclient.Client, hayabusaForkedBlock uint32, hayabusaActiveBlock uint32) *BlockFetcher {
	cache, _ := lru.New[uint32, *FetchResult](cacheSize)

	return &BlockFetcher{
		client:              client,
		cache:               cache,
		hayabusaForkedBlock: hayabusaForkedBlock,
		hayabusaActiveBlock: hayabusaActiveBlock,
	}
}

func (b *BlockFetcher) FetchBlock(blockNum uint32) (*FetchResult, error) {
	key := fmt.Sprintf("block_%d", blockNum)

	result, err, _ := b.sf.Do(key, func() (interface{}, error) {
		// Check cache first (inside singleflight to prevent race)
		if cached, exists := b.cache.Get(blockNum); exists {
			return cached, nil
		}

		// Fetch with retry
		var fetchResult *FetchResult
		err := common.Retry(func() error {
			// Fetch block
			block, err := b.client.ExpandedBlock(fmt.Sprintf("%d", blockNum))
			if err != nil {
				return fmt.Errorf("failed to fetch block: %w", err)
			}

			// Fetch seed of the current block
			seed, err := fetchSeed(block.ParentID, b.client)
			if err != nil {
				return fmt.Errorf("failed to fetch seed: %w", err)
			}

			// update the future seed if it's a boundary seed
			futureSeed := seed
			if (blockNum+1)%thor.SeederInterval() == 0 {
				futureSeed, err = fetchSeed(block.ID, b.client)
				if err != nil {
					return fmt.Errorf("failed to fetch future seed: %w", err)
				}
			}

			// Fetch staker info if needed
			var stakerInfo *types.StakerInformation
			if blockNum >= b.hayabusaForkedBlock {
				stakerInfo, err = pos.FetchValidations(block.ID, b.client)
				if err != nil {
					return fmt.Errorf("failed to fetch staker info: %w", err)
				}
			}

			// Fetch Auth nodes info if needed
			var authNodes types.AuthorityNodeList
			if blockNum <= b.hayabusaActiveBlock {
				authNodes, err = slots.FetchAuthorityNodes(b.client, block.ID)
				if err != nil {
					slog.Warn("failed to fetch auth nodes info for block", "block", block.Number, "error", err)
					return errors.Wrap(err, "failed to fetch auth nodes info")
				}
			}

			fetchResult = &FetchResult{
				Block:      block,
				Seed:       seed,
				Staker:     stakerInfo,
				AuthNodes:  authNodes,
				FutureSeed: futureSeed,
			}
			return nil
		}, 2*time.Minute, time.Minute*20) // large retry duration due to rate limiting

		if err != nil {
			return nil, err
		}

		// Store in cache
		b.cache.Add(blockNum, fetchResult)

		return fetchResult, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*FetchResult), nil
}

func fetchSeed(parentID thor.Bytes32, client *thorclient.Client) ([]byte, error) {
	blockNum := binary.BigEndian.Uint32(parentID[:]) + 1
	epoch := blockNum / thor.SeederInterval()
	if epoch <= 1 {
		return []byte{}, nil
	}
	seedNum := (epoch - 1) * thor.SeederInterval()

	seedBlock, err := client.Block(fmt.Sprintf("%d", seedNum))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch seed block: %w", err)
	}
	seedID := seedBlock.ID

	rawBlock := api.JSONRawBlockSummary{}
	res, status, err := client.RawHTTPClient().RawHTTPGet("/blocks/" + hex.EncodeToString(seedID.Bytes()) + "?raw=true")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch raw block: %w", err)
	}
	if status != 200 {
		return nil, fmt.Errorf("failed to fetch raw block: %s", res)
	}
	if err = json.Unmarshal(res, &rawBlock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw block: %w", err)
	}
	data, err := hex.DecodeString(rawBlock.Raw[2:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode raw block data: %w", err)
	}
	header := block.Header{}
	err = rlp.DecodeBytes(data, &header)
	if err != nil {
		return nil, fmt.Errorf("failed to decode block header: %w", err)
	}

	return header.Beta()
}
