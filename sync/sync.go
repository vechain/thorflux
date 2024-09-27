package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/darrenvechain/thor-go-sdk/client"
	"github.com/darrenvechain/thor-go-sdk/thorgo"
	"github.com/vechain/thorflux/influxdb"
)

const querySize = 100

type Sync struct {
	thor      *thorgo.Thor
	influx    *influxdb.DB
	prev      *atomic.Uint64
	blockChan chan *client.ExpandedBlock
	context   context.Context
}

func New(thor *thorgo.Thor, influx *influxdb.DB, start uint64, ctx context.Context) *Sync {
	prev := &atomic.Uint64{}
	prev.Store(start - 1)
	blockChan := make(chan *client.ExpandedBlock, 5000)
	return &Sync{thor: thor, influx: influx, prev: prev, blockChan: blockChan, context: ctx}
}

func (s *Sync) Index() {
	go s.writeBlocks()
	slog.Info("starting fast sync", "prev", s.prev.Load())
	s.fastSync()
	slog.Info("fast sync complete, starting regular sync", "prev", s.prev.Load())
	s.sync()
}

// sync fetches blocks one by one and ensures we are always 6 blocks behind to avoid forks
func (s *Sync) sync() {
	for {
		select {
		case <-s.context.Done():
			slog.Info("sync - context done")
			return
		default:
			block, err := s.thor.Client().ExpandedBlock(fmt.Sprintf("%d", s.prev.Load()+1))
			if err != nil {
				slog.Error("failed to fetch block", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}
			blockTime := time.Unix(int64(block.Timestamp), 0).UTC()
			diff := time.Now().UTC().Sub(blockTime)
			if diff < 60*time.Second {
				time.Sleep(60*time.Second - diff)
			}
			slog.Info("✅ fetched block", "block", block.Number)
			s.prev.Store(block.Number)
			s.blockChan <- block
		}
	}
}

// fastSync fetches blocks in parallel and writes them to the chan
func (s *Sync) fastSync() {
	for {
		select {
		case <-s.context.Done():
			slog.Info("fast sync - context done")
			return
		default:
			if s.shouldQuit() {
				slog.Info("fast sync complete")
				return
			}
			slog.Info("🔬 fetching blocks", "prev", s.prev.Load())
			blocks, err := s.fetchBlocksAsync(querySize, s.prev.Load()+1)
			if err != nil {
				slog.Error("failed to fetch blocks", "error", err)
				time.Sleep(5 * time.Second)
			} else {
				s.prev.Store(s.prev.Load() + querySize)
				for _, block := range blocks {
					s.blockChan <- block
				}
			}
		}
	}
}

func (s *Sync) shouldQuit() bool {
	best, err := s.thor.Client().BestBlock()
	if err != nil {
		slog.Error("failed to get best block", "error", err)
		return false
	}
	if best.Number-s.prev.Load() > 1000 {
		return false
	}
	return true
}

func (s *Sync) writeBlocks() {
	for {
		select {
		case <-s.context.Done():
			return
		case block, ok := <-s.blockChan:
			if !ok {
				return
			}
			s.influx.WriteBlock(block)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *Sync) fetchBlocksAsync(amount int, startBlock uint64) ([]*client.ExpandedBlock, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var err error
	blocks := make([]*client.ExpandedBlock, amount)

	for i := 0; i < amount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			block, fetchErr := s.thor.Client().ExpandedBlock(fmt.Sprintf("%d", startBlock+uint64(i)))
			if fetchErr != nil {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}
			blocks[i] = block
		}(i)
	}

	wg.Wait()
	if err != nil {
		return nil, err
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Number < blocks[j].Number
	})

	return blocks, nil
}
