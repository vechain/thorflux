package pubsub

import (
	"context"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thorflux/config"
	"log/slog"
	"sync"
)

const backwardWorkers = 100

type BackwardSyncer struct {
	eventBlockService *EventBlockService
	blockChan         chan *BlockEvent
	minBlock          uint32
	head              uint32
}

func NewBackwardSyncer(
	eventBlockService *EventBlockService,
	blockChan chan *BlockEvent,
	head *api.JSONExpandedBlock,
	backSyncBlocks uint32,
) *BackwardSyncer {
	var minBlock uint32
	if head.Number > backSyncBlocks {
		minBlock = head.Number - backSyncBlocks
	}

	slog.Info("📚 backward syncer initialized",
		"head", head.Number,
		"minBlock", minBlock,
	)

	return &BackwardSyncer{
		eventBlockService: eventBlockService,
		blockChan:         blockChan,
		minBlock:          minBlock,
		head:              head.Number,
	}
}

func (s *BackwardSyncer) Start(ctx context.Context) {
	slog.Info("📚 backward sync started", "head", s.head, "minBlock", s.minBlock)

	// Channel for work distribution
	workChan := make(chan uint32, backwardWorkers*2)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < backwardWorkers; i++ {
		wg.Add(1)
		go s.worker(ctx, workChan, &wg, i)
	}

	// Generate work (block numbers to process)
	go func() {
		defer close(workChan)
		for blockNum := s.head; blockNum > s.minBlock && blockNum > 0; blockNum-- {
			select {
			case <-ctx.Done():
				return
			case workChan <- blockNum:
			}
		}
	}()

	// Wait for all workers to complete
	wg.Wait()
	slog.Info("📚 backward sync complete")
}

func (s *BackwardSyncer) worker(ctx context.Context, workChan chan uint32, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("📚 backward worker cancelled", "worker", workerID)
			return
		case blockNum, ok := <-workChan:
			if !ok {
				slog.Debug("📚 backward worker finished", "worker", workerID)
				return
			}

			// Process block
			blockEvent, err := s.eventBlockService.ProcessBlock(blockNum)
			if err != nil {
				slog.Error("📚 failed to process block", "block", blockNum, "worker", workerID, "error", err)
				continue
			}

			// Send to channel
			select {
			case <-ctx.Done():
				return
			case s.blockChan <- blockEvent:
				if blockNum%config.LogIntervalBlocks == 0 {
					slog.Info("📚 processed block", "block", blockNum, "worker", workerID)
				}
			}
		}
	}
}
