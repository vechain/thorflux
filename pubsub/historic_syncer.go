package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

const querySize = 200

// HistoricSyncer fetches blocks in parallel from the current head to a specified minimum block number
// It will exit when it syncs down to the minimum block number
type HistoricSyncer struct {
	client              *thorclient.Client
	staker              *builtin.Staker
	blockChan           chan *Block
	head                *atomic.Pointer[api.JSONExpandedBlock]
	minBlock            uint32
	hayabusaForkedBlock uint32
	hayabusaActiveBlock uint32
}

func NewHistoricSyncer(
	client *thorclient.Client,
	staker *builtin.Staker,
	blockChan chan *Block,
	head *api.JSONExpandedBlock,
	backSyncBlocks uint32,
) (*HistoricSyncer, error) {
	h := &atomic.Pointer[api.JSONExpandedBlock]{}
	h.Store(head)
	var minBlock uint32
	if head.Number > backSyncBlocks {
		minBlock = head.Number - backSyncBlocks
	}

	forkBlock, activeBlock, err := fetchHayabusaBlocks(client, staker, head.Number, minBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch hayabusa blocks: %w", err)
	}

	return &HistoricSyncer{
		client:              client,
		staker:              staker,
		blockChan:           blockChan,
		head:                h,
		minBlock:            minBlock,
		hayabusaForkedBlock: forkBlock,
		hayabusaActiveBlock: activeBlock,
	}, nil
}

func (s *HistoricSyncer) Head() *api.JSONExpandedBlock {
	return s.head.Load()
}

// syncBack fetches blocks in parallel and writes them to the chan
func (s *HistoricSyncer) syncBack(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("fast sync - context done")
			return
		default:
			if s.backSyncComplete() {
				slog.Info("fast sync complete")
				return
			}
			slog.Info("🛵 fetching blocks async", "prev", s.Head().Number)
			blocks, err := s.fetchBlocksAsync(querySize, s.Head())
			if err != nil {
				slog.Error("failed to fetch blocks", "error", err)
				time.Sleep(config.LongRetryDelay)
			} else {
				s.head.Store(blocks[len(blocks)-1].Block)
				for _, block := range blocks {
					s.blockChan <- block
				}
			}
		}
	}
}

func (s *HistoricSyncer) backSyncComplete() bool {
	return s.Head().Number <= s.minBlock || s.Head().Number == 1
}

func (s *HistoricSyncer) fetchBlocksAsync(amount uint32, head *api.JSONExpandedBlock) ([]*Block, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var err error
	blks := make(map[thor.Bytes32]*Block)

	// +1 so we can populate parent
	for i := range amount + 1 {
		wg.Add(1)
		go func(i uint32) {
			defer wg.Done()
			block, fetchErr := s.client.ExpandedBlock(fmt.Sprintf("%d", i))
			if fetchErr != nil {
				mu.Lock()
				err = fetchErr
				mu.Unlock()
				return
			}

			seed, seedErr := fetchSeed(block.ParentID, s.client)
			if seedErr != nil {
				mu.Lock()
				err = fmt.Errorf("failed to fetch seed for block %d: %w", block.Number, seedErr)
				mu.Unlock()
				return
			}

			mu.Lock()
			blks[block.ID] = &Block{
				Block: block,
				Seed:  seed,
			}
			mu.Unlock()
		}(head.Number - i - 1)
	}

	wg.Wait()
	if err != nil {
		return nil, err
	}

	// Block entry should include parent, so here we are just linking them together
	results := make([]*Block, 0, len(blks))
	current := head.ParentID
	for uint32(len(results)) < amount {
		entry, ok := blks[current]
		if !ok {
			slog.Warn("missing block during async fetch", "current", current)
			return nil, fmt.Errorf("missing block during async fetch")
		}
		parentEntry, ok := blks[entry.Block.ParentID]
		if !ok {
			slog.Warn("missing parent block during async fetch", "parent", entry.Block.ParentID)
			var err error
			entry.Prev, err = s.client.ExpandedBlock(entry.Block.ParentID.String())
			if err != nil {
				return nil, fmt.Errorf("failed to fetch parent block %s: %w", entry.Block.ParentID, err)
			}
		} else {
			entry.Prev = parentEntry.Block
		}
		entry.HayabusaStatus = types.HayabusaStatus{
			Active: entry.Block.Number >= s.hayabusaActiveBlock,
			Forked: entry.Block.Number >= s.hayabusaForkedBlock,
		}
		results = append(results, entry)
		current = entry.Block.ParentID
	}

	return results, nil
}

// fetchHayabusaBlocks finds the Hayabusa fork block and the DPoS active block using a modified binary search
func fetchHayabusaBlocks(client *thorclient.Client, staker *builtin.Staker, max uint32, min uint32) (uint32, uint32, error) {
	forkBlockCond := func(rev string) (bool, error) {
		forked, err := isHayabusaForked(client, rev)
		return forked, err
	}

	activeBlockCond := func(rev string) (bool, error) {
		active, err := isDposActive(staker, rev)
		return active, err
	}

	forkBlock, err := binarySearchChainCondition(max, min, forkBlockCond)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to find hayabusa fork block: %w", err)
	}

	activeBlock, err := binarySearchChainCondition(max, min, activeBlockCond)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to find hayabusa active block: %w", err)
	}

	return forkBlock, activeBlock, nil
}

func binarySearchChainCondition(max, min uint32, condition func(string) (bool, error)) (uint32, error) {
	low := min
	high := max
	var mid uint32

	// First, check if the condition is already true at the minimum block
	// If so, we can quickly return min block
	condAtMin, _ := condition(fmt.Sprintf("%d", min))
	if condAtMin {
		// Condition is true at min, so the transition happened before min
		// We can't find it in this range, return min as the best guess
		return min, nil
	}

	// Check if condition is false at max - if so, the transition hasn't happened yet
	condAtMax, err := condition(fmt.Sprintf("%d", max))
	if !condAtMax || err != nil {
		// Condition is false at max, so the transition hasn't happened yet
		return math.MaxUint32, nil
	}

	// Now we know: condition is false at min, true at max
	// Find the first block where condition becomes true
	for low < high {
		mid = (low + high) / 2
		cond, _ := condition(fmt.Sprintf("%d", mid))

		if cond {
			// Condition is true at mid, check if this is the transition point
			if mid == min {
				// We're at the minimum, this must be the transition
				return mid, nil
			}

			// Check the previous block to see if this is the transition
			prev := mid - 1
			if prev >= min {
				prevCond, _ := condition(fmt.Sprintf("%d", prev))
				if !prevCond {
					// Found the transition: false at prev, true at mid
					return mid, nil
				}
			}

			// Transition happened at or before mid, search left
			high = mid
		} else {
			// Condition is false at mid, transition happened after mid
			low = mid + 1
		}
	}
	return low, nil
}
