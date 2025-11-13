package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/httpclient"
	"github.com/vechain/thorflux/config"
)

type ForwardSyncer struct {
	client            *thorclient.Client
	eventBlockService *EventBlockService
	blockChan         chan *BlockEvent
	prev              *atomic.Pointer[api.JSONExpandedBlock]
}

func NewForwardSyncer(
	client *thorclient.Client,
	eventBlockService *EventBlockService,
	blockChan chan *BlockEvent,
	startingBlock *api.JSONExpandedBlock,
) *ForwardSyncer {
	prev := &atomic.Pointer[api.JSONExpandedBlock]{}
	prev.Store(startingBlock)

	slog.Info("⏩ forward syncer 2 initialized",
		"startingBlock", startingBlock.Number,
	)

	return &ForwardSyncer{
		client:            client,
		eventBlockService: eventBlockService,
		blockChan:         blockChan,
		prev:              prev,
	}
}

func (f *ForwardSyncer) previous() *api.JSONExpandedBlock {
	return f.prev.Load()
}

func (f *ForwardSyncer) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("⏩ forward sync 2 - context done")
			return
		default:
			prev := f.previous()
			prevTime := time.Unix(int64(prev.Timestamp), 0).UTC()
			
			// Wait for block interval
			if time.Since(prevTime) < time.Duration(thor.BlockInterval()) {
				time.Sleep(time.Until(prevTime.Add(time.Duration(thor.BlockInterval()))))
				continue
			}

			// Try to fetch next block
			nextBlockNum := prev.Number + 1
			next, err := f.client.ExpandedBlock(fmt.Sprintf("%d", nextBlockNum))
			if err != nil {
				if f.databaseAhead(err) {
					slog.Error("⏩ database ahead, restarting service", "previous", prev.Number)
					os.Exit(1)
				}
				if !errors.Is(err, httpclient.ErrNotFound) {
					slog.Error("⏩ failed to fetch block", "error", err, "block", nextBlockNum)
				}
				time.Sleep(config.DefaultRetryDelay)
				continue
			}

			// Check for fork
			if next.ParentID != prev.ID {
				slog.Warn("⚠️ fork detected", "prev", prev.Number, "next", next.Number)

				var finalized *api.JSONExpandedBlock
				for {
					finalized, err = f.client.ExpandedBlock("finalized")
					if err != nil {
						slog.Error("⏩ failed to fetch finalized block", "error", err)
						time.Sleep(config.DefaultRetryDelay)
						continue
					}
					break
				}

				// Send fork event
				f.blockChan <- &BlockEvent{
					Block: finalized,
					Fork: ForkEvent{
						Occurred:  true,
						Best:      next,
						Finalized: finalized,
						SideChain: prev,
					},
				}
				f.prev.Store(finalized)
				continue
			}

			// Process block using EventBlockService
			blockEvent, err := f.eventBlockService.ProcessBlock(nextBlockNum)
			if err != nil {
				slog.Error("⏩ failed to process block", "block", nextBlockNum, "error", err)
				time.Sleep(config.DefaultRetryDelay)
				continue
			}

			// Log progress
			t := time.Unix(int64(next.Timestamp), 0).UTC()
			if next.Number%config.LogIntervalBlocks == 0 || time.Now().UTC().Sub(t) < config.RecentBlockThresholdMinutes {
				slog.Info("✅ fetched block", "number", next.Number)
			}

			// Send block event
			select {
			case <-ctx.Done():
				return
			case f.blockChan <- blockEvent:
				f.prev.Store(next)
			}
		}
	}
}

func (f *ForwardSyncer) databaseAhead(blockErr error) bool {
	if !errors.Is(blockErr, httpclient.ErrNotFound) {
		return false
	}
	best, err := f.client.Block("best")
	if err != nil {
		slog.Error("⏩ failed to get best block when checking for database ahead", "error", err)
		return false
	}
	return f.previous().Number > best.Number
}