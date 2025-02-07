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

const querySize = 200

type Sync struct {
	thor      *thorclient.Client
	influx    *influxdb.DB
	prev      *atomic.Value
	blockChan chan *blockType.Block
	context   context.Context
}

func New(thor *thorclient.Client, influx *influxdb.DB, start *blocks.JSONExpandedBlock, ctx context.Context) *Sync {
	prev := &atomic.Value{}
	prev.Store(start)
	blockChan := make(chan *blockType.Block, 2000)
	return &Sync{thor: thor, influx: influx, prev: prev, blockChan: blockChan, context: ctx}
}

func (s *Sync) Index() {
	go s.writeBlocks()
	slog.Info("starting fast sync", "prev", s.prev.Load())
	s.fastSync()
	slog.Info("fast sync complete, starting regular sync", "prev", s.prev.Load())
	s.sync()
}

func (s *Sync) previous() *blocks.JSONExpandedBlock {
	return s.prev.Load().(*blocks.JSONExpandedBlock)
}

// sync fetches blocks one by one and ensures we are always 6 blocks behind to avoid forks
func (s *Sync) sync() {
	for {
		select {
		case <-s.context.Done():
			slog.Info("sync - context done")
			return
		default:
			prev := s.previous()
			prevTime := time.Unix(int64(prev.Timestamp), 0).UTC()
			if time.Now().UTC().Sub(prevTime) < 10*time.Second {
				time.Sleep(10 * time.Second)
				continue
			}
			next, err := s.thor.ExpandedBlock(fmt.Sprintf("%d", prev.Number+1))
			if err != nil {
				slog.Error("failed to fetch block", "error", err)
				time.Sleep(5 * time.Second)
				continue
			}

			if next.ParentID != prev.ID { // fork detected
				slog.Warn("fork detected", "prev", prev.Number, "next", next.Number)

				var (
					finalized *blocks.JSONExpandedBlock
				)

				for {
					finalized, err = s.thor.ExpandedBlock("finalized")
					if err != nil {
						slog.Error("failed to fetch finalized block", "error", err)
						time.Sleep(5 * time.Second)
						continue
					}
					break
				}

				s.blockChan <- &blockType.Block{
					ExpandedBlock: next,
					ForkDetected:  true,
				}
				s.prev.Store(finalized)
				continue
			}

			slog.Info("✅ fetched block", "block", next.Number)

			rawBlock := blocks.JSONRawBlockSummary{}
			rawBytes, status, err := s.thor.RawHTTPClient().RawHTTPGet("/blocks/" + fmt.Sprintf("%d", next.Number) + "?raw=true")
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

			s.prev.Store(next)
			s.blockChan <- &blockType.Block{
				ExpandedBlock: next,
				RawHeader:     &header,
				ForkDetected:  false,
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
			slog.Info("🔬 fetching blocks", "prev", s.previous().Number)
			blocks, err := s.fetchBlocksAsync(querySize, s.previous().Number+1)
			if err != nil {
				slog.Error("failed to fetch blocks", "error", err)
				time.Sleep(5 * time.Second)
			} else {
				s.prev.Store(blocks[len(blocks)-1].ExpandedBlock)
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
	if best.Number-s.previous().Number > 1000 {
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
