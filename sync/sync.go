package sync

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/api/blocks"
	block2 "github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thorclient"
	blockType "github.com/vechain/thorflux/block"
	"github.com/vechain/thorflux/influxdb"
)

const querySize = 500

type Sync struct {
	thor      *thorclient.Client
	influx    *influxdb.DB
	prev      *atomic.Uint32
	blockChan chan *blockType.Block
	context   context.Context
}

func New(thor *thorclient.Client, influx *influxdb.DB, start uint32, ctx context.Context) *Sync {
	prev := &atomic.Uint32{}
	prev.Store(start - 1)
	blockChan := make(chan *blockType.Block, 5000)
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

			blockNumber := s.prev.Load() + 1
			block, err := s.thor.ExpandedBlock(fmt.Sprintf("%d", blockNumber))
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

			rawBlock := blocks.JSONRawBlockSummary{}
			rawBytes, status, err := s.thor.RawHTTPClient().RawHTTPGet("/blocks/" + fmt.Sprintf("%d", blockNumber) + "?raw=true")
			if err != nil || status != 200 {
				slog.Error("failed to fetch raw block", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if err := json.Unmarshal(rawBytes, &rawBlock); err != nil {
				slog.Error("failed to unmarshal raw block", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}

			data, err := hex.DecodeString(rawBlock.Raw[2:])
			if err != nil {
				panic(err)
			}
			header := block2.Header{}
			err = rlp.DecodeBytes(data, &header)

			s.prev.Store(block.Number)
			s.blockChan <- &blockType.Block{
				ExpandedBlock: block,
				RawHeader:     &header,
			}
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
	best, err := s.thor.Block("best")
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

func (s *Sync) fetchBlocksAsync(amount int, startBlock uint32) ([]*blockType.Block, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var err error
	blks := make([]*blockType.Block, 0)

	for i := 0; i < amount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			block, fetchErr := s.thor.ExpandedBlock(fmt.Sprintf("%d", startBlock+uint32(i)))
			if fetchErr != nil {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}

			rawBlock := blocks.JSONRawBlockSummary{}
			rawBytes, status, fetchErr := s.thor.RawHTTPClient().RawHTTPGet("/blocks/" + fmt.Sprintf("%d", block.Number) + "?raw=true")
			if fetchErr != nil || status != 200 {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}

			if fetchErr := json.Unmarshal(rawBytes, &rawBlock); fetchErr != nil {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}

			data, fetchErr := hex.DecodeString(rawBlock.Raw[2:])
			if fetchErr != nil {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}

			header := block2.Header{}
			fetchErr = rlp.DecodeBytes(data, &header)
			if fetchErr != nil {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}

			mu.Lock()
			blks = append(blks, &blockType.Block{
				ExpandedBlock: block,
				RawHeader:     &header,
			})
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	if err != nil {
		return nil, err
	}

	sort.Slice(blks, func(i, j int) bool {
		return blks[i].ExpandedBlock.Number < blks[j].ExpandedBlock.Number
	})

	return blks, nil
}
