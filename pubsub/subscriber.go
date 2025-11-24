package pubsub

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/stats/authority"
	"github.com/vechain/thorflux/stats/blockstats"
	"github.com/vechain/thorflux/stats/liveness"
	"github.com/vechain/thorflux/stats/pos"
	"github.com/vechain/thorflux/stats/priceapi"
	"github.com/vechain/thorflux/stats/slots"
	"github.com/vechain/thorflux/stats/transactions"
	"github.com/vechain/thorflux/stats/utilisation"
	"github.com/vechain/thorflux/types"
)

type Handler func(event *types.Event) []*write.Point

type Subscriber struct {
	blockChan  chan *BlockEvent
	db         *influxdb.DB
	chainTag   string
	handlers   map[string]Handler
	client     *thorclient.Client
	workerPool *WorkerPool
	synced     bool
}

func NewSubscriber(thorURL string, db *influxdb.DB, blockChan chan *BlockEvent, ownersRepo string) (*Subscriber, error) {
	tclient := thorclient.New(thorURL)

	chainTag, err := tclient.ChainTag()
	if err != nil {
		return nil, err
	}

	// register handler, execution order not guaranteed
	handlers := map[string]Handler{
		"authority":    authority.NewList(thorclient.New(thorURL), ownersRepo).Write,
		"pos":          pos.NewStaker(thorclient.New(thorURL)).Write,
		"transactions": transactions.Write,
		"liveness":     liveness.New(thorclient.New(thorURL)).Write,
		"blocks":       blockstats.Write,
		"utilisation":  utilisation.Write,
		"slots":        slots.New().Write,
		"price":        priceapi.New(db).Write,
	}

	// Create worker pool for concurrent handler execution
	workerPool := NewWorkerPool(config.DefaultWorkerPoolSize, config.DefaultTaskQueueSize, db)

	return &Subscriber{
		blockChan:  blockChan,
		db:         db,
		chainTag:   strconv.Itoa(int(chainTag)),
		handlers:   handlers,
		client:     tclient,
		workerPool: workerPool,
	}, nil
}

// Subscribe listens for new BlockEvents and processes them using registered handlers.
func (s *Subscriber) Subscribe(ctx context.Context) {
	defer s.workerPool.Shutdown()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Subscriber context cancelled, shutting down worker pool")
			return
		case b, ok := <-s.blockChan:
			if !ok {
				slog.Info("block channel closed, subscriber stopping")
				return
			}
			t := time.Unix(int64(b.Block.Timestamp), 0)

			// todo properly handle this
			if b.Fork.Occurred {
				slog.Warn("fork detected", "block", b.Block.Number)
				if err := NewForkHandler(s.db, s.client).Resolve(b.Fork.Best, b.Fork.SideChain, b.Fork.Finalized); err != nil {
					slog.Error("failed to resolve fork", "error", err)
				}
				continue
			}

			if b.Block.Number%config.LogIntervalBlocks == 0 || time.Since(t) < config.RecentBlockThreshold {
				slog.Info("🪣 writing to bucket", "number", b.Block.Number)
			}
			if !s.synced && time.Since(t) < time.Second*15 {
				slog.Info("✅ subscriber fully synced", "block_number", b.Block.Number)
				s.synced = true
			}
			if s.synced && time.Since(t) > time.Minute {
				slog.Warn("⚠️ subscriber out of sync", "block_number", b.Block.Number)
				s.synced = false
			}

			defaultTags := map[string]string{
				"signer": b.Block.Signer.String(),
			}

			event := &types.Event{
				DefaultTags:     defaultTags,
				Block:           b.Block,
				Seed:            b.Seed,
				Prev:            b.Prev,
				Timestamp:       t,
				HayabusaStatus:  b.HayabusaStatus,
				Staker:          b.Staker,
				ParentStaker:    b.ParentStaker,
				AuthNodes:       b.AuthNodes,
				ParentAuthNodes: b.ParentAuthNodes,
				FutureSeed:      b.FutureSeed,
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
			}
		}
	}
}
