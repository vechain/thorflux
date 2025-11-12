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
	"sync"
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
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/stats/pos"
	"github.com/vechain/thorflux/types"
)

type ForkEvent struct {
	Occurred  bool
	Best      *api.JSONExpandedBlock // the best block at which we detected the fork
	Finalized *api.JSONExpandedBlock // the block that is now finalized
	SideChain *api.JSONExpandedBlock // the block that was on the main chain but is now on a side chain
}

type BlockEvent struct {
	Fork           ForkEvent
	Block          *api.JSONExpandedBlock
	Prev           *api.JSONExpandedBlock
	HayabusaStatus types.HayabusaStatus
	Staker         *types.StakerInformation
	ParentStaker   *types.StakerInformation
	Seed           []byte
}

type Publisher struct {
	history        *HistoricSyncer
	prev           *atomic.Pointer[api.JSONExpandedBlock]
	parentStaker   *atomic.Pointer[types.StakerInformation]
	client         *thorclient.Client
	blockChan      chan *BlockEvent
	staker         *builtin.Staker
	hayabusaStatus types.HayabusaStatus
}

func NewPublisher(thorURL string, backSyncBlocks uint32, db *influxdb.DB) (*Publisher, chan *BlockEvent, error) {
	client := thorclient.New(thorURL)
	staker, err := builtin.NewStaker(client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create staker contract: %w", err)
	}
	blockChan := make(chan *BlockEvent, config.DefaultChannelBuffer)
	first, err := client.ExpandedBlock("finalized")
	if err != nil {
		return nil, nil, err
	}
	var previous *api.JSONExpandedBlock
	if first.Number == 0 {
		previous = first
		first, err = client.ExpandedBlock("1")
		if err != nil {
			return nil, nil, err
		}
	} else {
		previous, err = client.ExpandedBlock(strconv.FormatUint(uint64(first.Number-1), 10))
		if err != nil {
			return nil, nil, err
		}
	}
	prev := &atomic.Pointer[api.JSONExpandedBlock]{}
	prev.Store(previous)
	latest, err := db.Latest()
	if err != nil {
		slog.Warn("failed to get latest block from database")
	}
	if backSyncBlocks > first.Number {
		backSyncBlocks = first.Number - 1
	}
	if latest > first.Number-backSyncBlocks {
		if latest >= first.Number {
			// Database is ahead of finalized, no back sync needed
			backSyncBlocks = 0
		} else {
			backSyncBlocks = first.Number - latest
		}
	}
	slog.Info("creating historic syncer", "start", first.Number, "blocks", backSyncBlocks, "minimum", first.Number-backSyncBlocks)
	history, err := NewHistoricSyncer(client, staker, blockChan, first, backSyncBlocks)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialise historic syncer: %w", err)
	}
	parentStaker := &atomic.Pointer[types.StakerInformation]{}
	return &Publisher{
		history:      history,
		prev:         prev,
		client:       client,
		blockChan:    blockChan,
		staker:       staker,
		parentStaker: parentStaker,
	}, blockChan, nil
}

func (p *Publisher) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.history.syncBack(ctx)
	}()
	go func() {
		defer wg.Done()
		p.sync(ctx)
	}()
	go func() {
		wg.Wait()
		close(p.blockChan)
		slog.Info("block channel closed")
	}()
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

				p.blockChan <- &BlockEvent{
					Block: finalized,
					Fork: ForkEvent{
						Occurred:  true,
						Best:      next,
						Finalized: finalized,
						SideChain: prev,
					},
				}
				p.prev.Store(finalized)
				continue
			}
			p.checkHayabusaStatus(next.ID)

			t := time.Unix(int64(next.Timestamp), 0).UTC()
			if next.Number%config.LogIntervalBlocks == 0 || time.Now().UTC().Sub(t) < config.RecentBlockThresholdMinutes {
				slog.Info("✅ fetched block", "number", next.Number)
			}
			var stakerInfo *types.StakerInformation
			if p.hayabusaStatus.Forked {
				stakerInfo, err = pos.FetchValidations(next.ID, p.client)
				if err != nil {
					slog.Error("failed to fetch staker info", "block", next.Number, "error", err)
				}
			}

			p.blockChan <- &BlockEvent{
				Block:          next,
				Prev:           prev,
				Seed:           seed,
				HayabusaStatus: p.hayabusaStatus,
				Staker:         stakerInfo,
				ParentStaker:   p.parentStaker.Load(),
			}
			p.prev.Store(next)
			p.parentStaker.Store(stakerInfo)
		}
	}
}

func (p *Publisher) checkHayabusaStatus(blockID thor.Bytes32) {
	if p.hayabusaStatus.Forked && p.hayabusaStatus.Active {
		return
	}
	if !p.hayabusaStatus.Forked {
		p.hayabusaStatus.Forked = isHayabusaForked(p.client, blockID.String())
	}
	if !p.hayabusaStatus.Active {
		p.hayabusaStatus.Active = isDposActive(p.staker, blockID.String())
	}
}

func (p *Publisher) databaseAhead(blockErr error) bool {
	if !errors.Is(blockErr, httpclient.ErrNotFound) {
		return false
	}
	best, err := p.client.Block("best")
	if err != nil {
		slog.Error("failed to get best block when checking for database ahead", "error", err)
		return false
	}
	return p.previous().Number > best.Number
}

func isDposActive(staker *builtin.Staker, revision string) bool {
	_, id, err := staker.Revision(revision).FirstActive()
	if err != nil {
		return false
	}
	return !id.IsZero()
}

func isHayabusaForked(client *thorclient.Client, revision string) bool {
	code, err := client.AccountCode(&builtin2.Staker.Address, thorclient.Revision(revision))
	if err != nil {
		slog.Warn("failed to fetch staker code to check hayabusa fork status", "error", err)
		return false
	}
	return len(code.Code) > 100
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
