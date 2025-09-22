package pubsub

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/block"
	builtin2 "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thorflux/config"
)

type Block struct {
	ForkDetected   bool
	Block          *api.JSONExpandedBlock
	Seed           []byte
	HayabusaForked bool
	DPOSActive     bool
}

const querySize = config.DefaultQuerySize

type Publisher struct {
	thor      *thorclient.Client
	prev      *atomic.Pointer[api.JSONExpandedBlock]
	blockChan chan *Block
	staker    *builtin.Staker

	hayabusaForkBlock uint32
	dposActiveBlock   uint32
}

type DB interface {
	Latest() (uint32, error)
}

func New(thorURL string, db DB, blockAmount uint32) (*Publisher, chan *Block, error) {
	tclient := thorclient.New(thorURL)

	prev, err := db.Latest()
	if err != nil {
		slog.Error("failed to get latest block from DB", "error", err)
		return nil, nil, err
	}
	best, err := tclient.Block("best")
	if err != nil {
		slog.Error("failed to get best block from thor", "error", err)
		return nil, nil, err
	}
	slog.Info(fmt.Sprintf("best block is %d", best.Number))
	var startBlock uint32
	if blockAmount > best.Number-1 {
		startBlock = 1
	} else {
		startBlock = best.Number - blockAmount
	}
	if prev > startBlock {
		startBlock = prev
	}
	start, err := tclient.ExpandedBlock(fmt.Sprintf("%d", startBlock))
	if err != nil {
		slog.Error("failed to get block from thor", "block", startBlock, "error", err)
		return nil, nil, err
	}
	previous := &atomic.Pointer[api.JSONExpandedBlock]{}
	previous.Store(start)
	blockChan := make(chan *Block, config.DefaultChannelBuffer)

	staker, err := builtin.NewStaker(tclient)
	if err != nil {
		slog.Error("failed to create staker instance", "error", err)
		return nil, nil, err
	}

	slog.Info("starting block sync",
		"start", startBlock,
		"best", best.Number,
		"prev", prev,
		"missing-blocks", best.Number-startBlock,
	)

	return &Publisher{
		thor:              tclient,
		prev:              previous,
		blockChan:         blockChan,
		hayabusaForkBlock: math.MaxUint32,
		dposActiveBlock:   math.MaxUint32,
		staker:            staker,
	}, blockChan, nil
}

func (s *Publisher) Publish(ctx context.Context) {
	slog.Info("starting fast sync", "prev", s.previous().Number)
	s.fastSync(ctx)
	slog.Info("fast sync complete, starting regular sync", "prev", s.previous().Number)
	s.sync(ctx)
}

func (s *Publisher) previous() *api.JSONExpandedBlock {
	return s.prev.Load()
}

// sync fetches blocks one by one and ensures we are always 6 blocks behind to avoid forks
func (s *Publisher) sync(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("sync - context done")
			return
		default:
			prev := s.previous()
			prevTime := time.Unix(int64(prev.Timestamp), 0).UTC()
			if time.Since(prevTime) < time.Duration(thor.BlockInterval()) {
				time.Sleep(time.Until(prevTime.Add(time.Duration(thor.BlockInterval()))))
				continue
			}
			next, err := s.thor.ExpandedBlock(fmt.Sprintf("%d", prev.Number+1))
			if err != nil {
				if s.databaseAhead(err) {
					slog.Error("database ahead, restarting service", "previous", prev.Number)
					os.Exit(1)
				}
				slog.Error("failed to fetch block", "error", err, "block", prev.Number+1)
				time.Sleep(config.DefaultRetryDelay)
				continue
			}
			seed, err := s.fetchSeed(next.ParentID)
			if err != nil {
				slog.Error("failed to fetch seed for block", "block", next.Number, "error", err)
				time.Sleep(config.DefaultRetryDelay)
				continue
			}

			if next.ParentID != prev.ID { // fork detected
				slog.Warn("fork detected", "prev", prev.Number, "next", next.Number)

				var (
					finalized *api.JSONExpandedBlock
				)

				for {
					finalized, err = s.thor.ExpandedBlock("finalized")
					if err != nil {
						slog.Error("failed to fetch finalized block", "error", err)
						time.Sleep(config.DefaultRetryDelay)
						continue
					}
					break
				}

				s.blockChan <- &Block{
					Block:        finalized,
					ForkDetected: true,
					Seed:         seed,
				}
				s.prev.Store(finalized)
				continue
			}
			forked, active, err := s.fetchHayabusaStatus(next)
			if err != nil {
				slog.Error("failed to fetch hayabusa status for block", "block", next.Number, "error", err)
				time.Sleep(config.DefaultRetryDelay)
				continue
			}

			t := time.Unix(int64(next.Timestamp), 0).UTC()
			if next.Number%config.LogIntervalBlocks == 0 || time.Now().UTC().Sub(t) < config.RecentBlockThresholdMinutes {
				slog.Info("✅ fetched block", "number", next.Number)
			}

			s.prev.Store(next)
			s.blockChan <- &Block{
				Block:          next,
				ForkDetected:   false,
				Seed:           seed,
				HayabusaForked: forked,
				DPOSActive:     active,
			}
		}
	}
}

// fastSync fetches blocks in parallel and writes them to the chan
func (s *Publisher) fastSync(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("fast sync - context done")
			return
		default:
			if s.fastSyncComplete() {
				slog.Info("fast sync complete")
				return
			}
			slog.Info("🛵 fetching blocks async", "prev", s.previous().Number)
			blocks, err := s.fetchBlocksAsync(querySize, s.previous().Number+1)
			if err != nil {
				slog.Error("failed to fetch blocks", "error", err)
				time.Sleep(config.LongRetryDelay)
			} else {
				s.prev.Store(blocks[len(blocks)-1].Block)
				for _, block := range blocks {
					s.blockChan <- block
				}
			}
		}
	}
}

func (s *Publisher) fastSyncComplete() bool {
	best, err := s.thor.Block("best")
	if err != nil {
		slog.Error("failed to get best block when checking for fast sync complete", "error", err)
		return false
	}
	return best.Number-s.previous().Number <= config.MaxBlocksBehind
}

func (s *Publisher) databaseAhead(blockErr error) bool {
	best, err := s.thor.Block("best")
	if err != nil {
		slog.Error("failed to get best block when checking for database ahead", "error", err)
		return false
	}
	return blockErr.Error() == config.ErrBlockNotFound && s.previous().Number > best.Number
}

func (s *Publisher) fetchBlocksAsync(amount int, startBlock uint32) ([]*Block, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var err error
	blks := make([]*Block, 0, amount)

	for i := range amount {
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

			seed, err := s.fetchSeed(block.ParentID)
			if err != nil {
				mu.Lock()
				// nolint:ineffassign,staticcheck
				err = fmt.Errorf("failed to fetch seed for block %d: %w", block.Number, err)
				mu.Unlock()
				return
			}

			forked, active, err := s.fetchHayabusaStatus(block)
			if err != nil {
				mu.Lock()
				// nolint:ineffassign,staticcheck
				err = fmt.Errorf("failed to fetch hayabusa status for block %d: %w", block.Number, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			blks = append(blks, &Block{
				Block:          block,
				Seed:           seed,
				HayabusaForked: forked,
				DPOSActive:     active,
			})
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	if err != nil {
		return nil, err
	}

	sort.Slice(blks, func(i, j int) bool {
		return blks[i].Block.Number < blks[j].Block.Number
	})

	return blks, nil
}

func (s *Publisher) fetchSeed(parentID thor.Bytes32) ([]byte, error) {
	blockNum := binary.BigEndian.Uint32(parentID[:]) + 1
	epoch := blockNum / thor.SeederInterval()
	if epoch <= 1 {
		return []byte{}, nil
	}
	seedNum := (epoch - 1) * thor.SeederInterval()

	seedBlock, err := s.thor.Block(fmt.Sprintf("%d", seedNum))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch seed block: %w", err)
	}
	seedID := seedBlock.ID

	rawBlock := api.JSONRawBlockSummary{}
	res, status, err := s.thor.RawHTTPClient().RawHTTPGet("/blocks/" + hex.EncodeToString(seedID.Bytes()) + "?raw=true")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch raw block: %w", err)
	}
	if status != 200 {
		return nil, fmt.Errorf("failed to fetch raw block: %s", res)
	}
	if err = json.Unmarshal(res, &rawBlock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw block: %w", err)
	}
	data, err := hex.DecodeString(rawBlock.Raw[2:])
	if err != nil {
		return nil, fmt.Errorf("failed to decode raw block data: %w", err)
	}
	header := block.Header{}
	err = rlp.DecodeBytes(data, &header)
	if err != nil {
		return nil, fmt.Errorf("failed to decode block header: %w", err)
	}

	return header.Beta()
}

func (s *Publisher) fetchHayabusaStatus(block *api.JSONExpandedBlock) (bool, bool, error) {
	forked := s.hayabusaForkBlock < block.Number

	active := s.dposActiveBlock < block.Number

	if !forked {
		code, err := s.thor.AccountCode(&builtin2.Staker.Address, thorclient.Revision(block.ID.String()))
		if err != nil {
			return false, false, fmt.Errorf("failed to get account code: %w", err)
		}
		forked = len(code.Code) > 100
		if forked {
			s.hayabusaForkBlock = block.Number
		}
	}

	if !active {
		_, id, err := s.staker.Revision(block.ID.String()).FirstActive()
		active = err == nil && !id.IsZero()
		if active {
			s.dposActiveBlock = block.Number
		}
	}

	return forked, active, nil
}
