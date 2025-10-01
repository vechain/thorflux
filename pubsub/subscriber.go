package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/vechain/thor/v2/api"

	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/stats/authority"
	"github.com/vechain/thorflux/stats/blockstats"
	liveness2 "github.com/vechain/thorflux/stats/liveness"
	"github.com/vechain/thorflux/stats/pos"
	"github.com/vechain/thorflux/stats/transactions"
	"github.com/vechain/thorflux/stats/utilisation"
	"github.com/vechain/thorflux/types"
)

type Handler func(event *types.Event) error

type Subscriber struct {
	blockChan  chan *Block
	db         *influxdb.DB
	genesis    *api.JSONCollapsedBlock
	chainTag   string
	handlers   map[string]Handler
	client     *thorclient.Client
	workerPool *WorkerPool
}

func NewSubscriber(thorURL string, db *influxdb.DB, blockChan chan *Block) (*Subscriber, error) {
	tclient := thorclient.New(thorURL)

	genesis, err := tclient.Block("0")
	if err != nil {
		slog.Error("failed to get genesis block", "error", err)
		return nil, err
	}
	chainTag := fmt.Sprintf("%d", genesis.ID[31])

	liveness := liveness2.New(thorclient.New(thorURL))
	poa := authority.NewList(thorclient.New(thorURL))
	hayabusa, err := pos.NewStaker(thorclient.New(thorURL))
	if err != nil {
		slog.Error("failed to create staker instance", "error", err)
		return nil, err
	}

	handlers := make(map[string]Handler)
	handlers["authority"] = poa.Write
	handlers["pos"] = hayabusa.Write
	handlers["transactions"] = transactions.Write
	handlers["liveness"] = liveness.Write
	handlers["blocks"] = blockstats.Write
	handlers["utilisation"] = utilisation.Write

	// Create worker pool for concurrent handler execution
	workerPool := NewWorkerPool(config.DefaultWorkerPoolSize, config.DefaultTaskQueueSize)

	return &Subscriber{
		blockChan:  blockChan,
		db:         db,
		genesis:    genesis,
		chainTag:   chainTag,
		handlers:   handlers,
		client:     tclient,
		workerPool: workerPool,
	}, nil
}

func (s *Subscriber) Subscribe(ctx context.Context) {
	defer s.workerPool.Shutdown()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Subscriber context cancelled, shutting down worker pool")
			return
		case b := <-s.blockChan:
			t := time.Unix(int64(b.Block.Timestamp), 0)

			if b.ForkDetected {
				slog.Warn("fork detected", "block", b.Block.Number)
				s.db.ResolveFork(t)
				continue
			}

			if b.Block.Number%config.LogIntervalBlocks == 0 || time.Since(t) < config.RecentBlockThreshold {
				slog.Info("🪣 writing to bucket", "number", b.Block.Number)
			}

			defaultTags := map[string]string{
				"chain_tag":    s.chainTag,
				"signer":       b.Block.Signer.String(),
				"block_number": fmt.Sprintf("%d", b.Block.Number),
			}

			if b.Prev == nil {
				slog.Warn("previous block is nil", "block_number", b.Block.Number)
				prev, err := s.client.ExpandedBlock(strconv.FormatUint(uint64(b.Block.Number-1), 10))
				if err != nil {
					slog.Error("failed to fetch previous block", "block_number", b.Block.Number-1, "error", err)
					continue
				}
				b.Prev = prev
			}

			event := &types.Event{
				Block:          b.Block,
				Seed:           b.Seed,
				WriteAPI:       s.db.WriteAPI(),
				Prev:           b.Prev,
				ChainTag:       s.chainTag,
				Genesis:        s.genesis,
				DefaultTags:    defaultTags,
				Timestamp:      t,
				HayabusaStatus: b.HayabusaStatus,
			}

			// Create tasks for all handlers
			tasks := make([]Task, 0, len(s.handlers))
			for name, handler := range s.handlers {
				tasks = append(tasks, Task{
					EventType: name,
					Handler:   handler,
					Event:     event,
				})
			}

			// Submit all tasks to worker pool
			if err := s.workerPool.SubmitBatch(tasks); err != nil {
				slog.Error("Failed to submit tasks to worker pool", "error", err, "block_number", b.Block.Number)
				// Fallback to synchronous execution if worker pool is full
				for _, task := range tasks {
					if err := task.Handler(task.Event); err != nil {
						slog.Error("Failed to handle event (fallback)", "event_type", task.EventType, "error", err)
					}
				}
			}
		}
	}
}
