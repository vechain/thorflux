package pubsub

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	builtin2 "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

type Block struct {
	ForkDetected   bool
	Block          *api.JSONExpandedBlock
	Prev           *api.JSONExpandedBlock
	HayabusaStatus types.HayabusaStatus
	Seed           []byte
}

type Publisher struct {
	history   *HistoricSyncer
	first     *api.JSONExpandedBlock
	prev      *atomic.Pointer[api.JSONExpandedBlock]
	client    *thorclient.Client
	blockChan chan *Block
	staker    *builtin.Staker

	hayabusaStatus types.HayabusaStatus
}

func NewPublisher(thorURL string, backSyncBlocks uint32) (*Publisher, chan *Block, error) {
	client := thorclient.New(thorURL)
	staker, err := builtin.NewStaker(client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create staker contract: %w", err)
	}
	blockChan := make(chan *Block, config.DefaultChannelBuffer)
	finalized, err := client.ExpandedBlock("finalized")
	if err != nil {
		return nil, nil, err
	}
	previous, err := client.ExpandedBlock(strconv.FormatUint(uint64(finalized.Number-1), 10))
	if err != nil {
		return nil, nil, err
	}
	prev := &atomic.Pointer[api.JSONExpandedBlock]{}
	prev.Store(previous)
	history, err := NewHistoricSyncer(client, staker, blockChan, finalized, backSyncBlocks)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialise historic syncer: %w", err)
	}
	return &Publisher{
		history:   history,
		first:     previous,
		prev:      prev,
		client:    client,
		blockChan: blockChan,
		staker:    staker,
	}, blockChan, nil
}

func (p *Publisher) Start(ctx context.Context) {
	go p.history.syncBack(ctx)
	go p.sync(ctx)
}

func (p *Publisher) previous() *api.JSONExpandedBlock {
	return p.prev.Load()
}

// sync fetches blocks one by one and writes them to the chan
func (p *Publisher) sync(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("sync - context done")
			return
		default:
			prev := p.previous()
			prevTime := time.Unix(int64(prev.Timestamp), 0).UTC()
			if time.Since(prevTime) < time.Duration(thor.BlockInterval()) {
				time.Sleep(time.Until(prevTime.Add(time.Duration(thor.BlockInterval()))))
				continue
			}
			next, err := p.client.ExpandedBlock(fmt.Sprintf("%d", prev.Number+1))
			if err != nil {
				if p.databaseAhead(err) {
					slog.Error("database ahead, restarting service", "previous", prev.Number)
					os.Exit(1)
				}
				if !errors.Is(err, httpclient.ErrNotFound) {
					slog.Error("failed to fetch block", "error", err, "block", prev.Number+1)
				}
				time.Sleep(config.DefaultRetryDelay)
				continue
			}
			seed, err := fetchSeed(next.ParentID, p.client)
			if err != nil {
				slog.Error("failed to fetch seed for block", "block", next.Number, "error", err)
				time.Sleep(config.DefaultRetryDelay)
				continue
			}

			if next.ParentID != prev.ID { // fork detected
				slog.Warn("fork detected", "prev", prev.Number, "next", next.Number)

				var finalized *api.JSONExpandedBlock

				for {
					finalized, err = p.client.ExpandedBlock("finalized")
					if err != nil {
						slog.Error("failed to fetch finalized block", "error", err)
						time.Sleep(config.DefaultRetryDelay)
						continue
					}
					break
				}

				p.blockChan <- &Block{
					Block:        finalized,
					ForkDetected: true,
				}
				p.prev.Store(finalized)
				continue
			}
			p.checkHayabusaStatus(next.ID)

			t := time.Unix(int64(next.Timestamp), 0).UTC()
			if next.Number%config.LogIntervalBlocks == 0 || time.Now().UTC().Sub(t) < config.RecentBlockThresholdMinutes {
				slog.Info("✅ fetched block", "number", next.Number)
			}
			p.blockChan <- &Block{
				Block:          next,
				Prev:           prev,
				ForkDetected:   false,
				Seed:           seed,
				HayabusaStatus: p.hayabusaStatus,
			}
			p.prev.Store(next)
		}
	}
}

func (p *Publisher) checkHayabusaStatus(blockID thor.Bytes32) {
	if p.hayabusaStatus.Forked && p.hayabusaStatus.Active {
		return
	}
	if !p.hayabusaStatus.Forked {
		p.hayabusaStatus.Forked, _ = isHayabusaForked(p.client, blockID.String())
	}
	if !p.hayabusaStatus.Active {
		p.hayabusaStatus.Active, _ = isDposActive(p.staker, blockID.String())
	}
}

func isDposActive(staker *builtin.Staker, revision string) (bool, error) {
	_, id, err := staker.Revision(revision).FirstActive()
	if err != nil {
		slog.Error("failed to fetch first active staker to check dpos status", "error", err)
		return false, err
	}
	return !id.IsZero(), nil
}

func isHayabusaForked(client *thorclient.Client, revision string) (bool, error) {
	code, err := client.AccountCode(&builtin2.Staker.Address, thorclient.Revision(revision))
	if err != nil {
		slog.Error("failed to fetch staker code to check hayabusa fork status", "error", err)
		return false, err
	}
	return len(code.Code) > 100, nil
}

func (p *Publisher) databaseAhead(blockErr error) bool {
	best, err := p.client.Block("best")
	if err != nil {
		slog.Error("failed to get best block when checking for database ahead", "error", err)
		return false
	}
	return blockErr.Error() == config.ErrBlockNotFound && p.previous().Number > best.Number
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
