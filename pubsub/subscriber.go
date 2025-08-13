package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
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
	blockChan chan *Block
	db        *influxdb.DB
	prevBlock *atomic.Pointer[api.JSONExpandedBlock]
	genesis   *api.JSONCollapsedBlock
	chainTag  string
	handlers  map[string]Handler
	client    *thorclient.Client
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

	return &Subscriber{
		blockChan: blockChan,
		db:        db,
		prevBlock: &atomic.Pointer[api.JSONExpandedBlock]{},
		genesis:   genesis,
		chainTag:  chainTag,
		handlers:  handlers,
		client:    tclient,
	}, nil
}

func (s *Subscriber) Subscribe(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-s.blockChan:
			t := time.Unix(int64(b.Block.Timestamp), 0)

			if b.ForkDetected {
				slog.Warn("fork detected", "block", b.Block.Number)
				s.db.ResolveFork(t)
				s.prevBlock.Store(b.Block)
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

			if s.prevBlock.Load() == nil && b.Block.Number > 0 {
				prev, err := s.client.ExpandedBlock(strconv.FormatUint(uint64(b.Block.Number-1), 10))
				if err != nil {
					slog.Error("failed to fetch previous block", "block_number", b.Block.Number-1, "error", err)
					continue
				}
				s.prevBlock.Store(prev)
			}

			event := &types.Event{
				Block:          b.Block,
				Seed:           b.Seed,
				HayabusaForked: b.HayabusaForked,
				DPOSActive:     b.DPOSActive,
				WriteAPI:       s.db.WriteAPI(),
				Prev:           s.prevBlock.Load(),
				ChainTag:       s.chainTag,
				Genesis:        s.genesis,
				DefaultTags:    defaultTags,
				Timestamp:      t,
			}

			wg := &sync.WaitGroup{}
			for name, handler := range s.handlers {
				wg.Add(1)
				go func(eventType string, handler Handler) {
					defer wg.Done()
					if err := handler(event); err != nil {
						slog.Error("failed to handle event", "event_type", eventType, "error", err)
					}
				}(name, handler)
			}
			wg.Wait()
			s.prevBlock.Store(b.Block)
		}
	}
}
