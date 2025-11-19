package pubsub

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thorflux/config"
)

const backwardWorkers = 100

type BackwardSyncer struct {
	eventBlockService *EventBlockService
	blockChan         chan *BlockEvent
	minBlock          uint32
	head              uint32
	backoff           atomic.Bool
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
		backoff:           atomic.Bool{},
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

			if s.backoff.Load() {
				slog.Warn("⏸️ backward worker backing off due to rate limit", "worker", workerID)
				time.Sleep(time.Minute)
			}

			// process block with panic recovery and re-queuing
			func(num uint32) {
				processed := false
				defer func() {
					if r := recover(); r != nil {
						slog.Error("⁉️ panic in backward worker", "block", num, "worker", workerID, "error", r)
					}
					if !processed {
						workChan <- num // Re-queue the block for processing
						slog.Warn("😩 re-queued block for processing", "block", num, "worker", workerID)
					}
				}()
				// Process block
				blockEvent, err := s.eventBlockService.ProcessBlock(num)
				if err != nil {
					if isRateLimitError(err) {
						s.backoff.Store(true)
						return
					}
					slog.Error("failed to process block", "block", num, "worker", workerID, "error", err)
					return
				}

				// Send to channel
				select {
				case <-ctx.Done():
					return
				case s.blockChan <- blockEvent:
					if num%config.LogIntervalBlocks == 0 {
						slog.Info("📚 processed backwards block", "block", num, "worker", workerID)
					}
					processed = true
				}
			}(blockNum)

		}
	}
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "rate limit exceeded") ||
		strings.Contains(message, "go away") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "429")
}
