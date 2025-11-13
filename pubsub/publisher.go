package pubsub

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"
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
	forwardSyncer  *ForwardSyncer
	backwardSyncer *BackwardSyncer
	blockChan      chan *BlockEvent
}

func NewPublisher(thorURL string, genesisCfg *genesis.CustomGenesis, backSyncBlocks uint32, db *influxdb.DB) (*Publisher, chan *BlockEvent, error) {
	client := thorclient.New(thorURL)

	blockChan := make(chan *BlockEvent, config.DefaultChannelBuffer)
	first, err := client.ExpandedBlock("finalized")
	if err != nil {
		return nil, nil, err
	}

	hayabusaForkBlock := genesisCfg.ForkConfig.HAYABUSA
	hayabusaActiveBlock := genesisCfg.ForkConfig.HAYABUSA + *genesisCfg.Config.HayabusaTP

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
	slog.Info("🚀 creating publisher",
		"start", first.Number,
		"blocks", backSyncBlocks,
		"minimum", first.Number-backSyncBlocks,
		"hayabusaForkBlock", hayabusaForkBlock,
		"hayabusaActiveBlock", hayabusaActiveBlock,
	)

	// Create block fetcher with LRU cache
	blockFetcher := NewBlockFetcher(client, hayabusaForkBlock)
	
	// Create event block service
	eventBlockService := NewEventBlockService(blockFetcher, hayabusaActiveBlock)

	// Create backward syncer (historical sync)
	backwardSyncer := NewBackwardSyncer(
		eventBlockService,
		blockChan,
		first,
		backSyncBlocks,
	)

	// Create forward syncer (real-time sync)
	forwardSyncer := NewForwardSyncer(
		client,
		eventBlockService,
		blockChan,
		previous,
	)

	return &Publisher{
		forwardSyncer:  forwardSyncer,
		backwardSyncer: backwardSyncer,
		blockChan:      blockChan,
	}, blockChan, nil
}

func (p *Publisher) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.backwardSyncer.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		p.forwardSyncer.Start(ctx)
	}()
	go func() {
		wg.Wait()
		close(p.blockChan)
		slog.Info("🔒 block channel closed")
	}()
}
