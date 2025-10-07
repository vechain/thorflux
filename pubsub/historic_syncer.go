package pubsub

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/stats/pos"
	"github.com/vechain/thorflux/types"
	"golang.org/x/sync/errgroup"
)

const querySize = config.DefaultQuerySize

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

	slog.Info("historic syncer initialized",
		"head", head.Number,
		"minBlock", minBlock,
		"hayabusaForkedBlock", forkBlock,
		"hayabusaActiveBlock", activeBlock,
	)

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
			amount := uint32(querySize)
			if s.Head().Number-s.minBlock < querySize {
				amount = s.Head().Number - s.minBlock
			}
			blocks, err := s.fetchBlocksAsync(ctx, amount, s.Head())
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

func (s *HistoricSyncer) fetchBlocksAsync(ctx context.Context, amount uint32, head *api.JSONExpandedBlock) ([]*Block, error) {
	var mu sync.Mutex
	blks := make(map[thor.Bytes32]*Block)

	group, _ := errgroup.WithContext(ctx)
	// +1 so we can populate parent
	for i := range amount + 1 {
		blockNum := head.Number - i - 1
		group.Go(func() error {
			block, err := s.client.ExpandedBlock(fmt.Sprintf("%d", blockNum))
			if err != nil {
				return err
			}

			seed, err := fetchSeed(block.ParentID, s.client)
			if err != nil {
				return err
			}

			var stakerInfo *types.StakerInformation
			if blockNum >= s.hayabusaForkedBlock {
				stakerInfo, err = pos.FetchValidations(block.ID, s.client)
				if err != nil {
					slog.Warn("failed to fetch staker info for block", "block", block.Number, "error", err)
					return errors.Wrap(err, "failed to fetch staker info")
				}
			}

			mu.Lock()
			blks[block.ID] = &Block{
				Block:  block,
				Seed:   seed,
				Staker: stakerInfo,
			}
			mu.Unlock()

			return nil
		})
	}

	err := group.Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch blocks async: %w", err)
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
		s.fillParent(entry, blks[entry.Block.ParentID])
		entry.HayabusaStatus = types.HayabusaStatus{
			Active: entry.Block.Number >= s.hayabusaActiveBlock,
			Forked: entry.Block.Number >= s.hayabusaForkedBlock,
		}
		results = append(results, entry)
		current = entry.Block.ParentID
	}

	return results, nil
}

func (s *HistoricSyncer) fillParent(current, parent *Block) {
	if parent != nil {
		current.Prev = parent.Block
		current.ParentStaker = parent.Staker
		return
	}
	slog.Warn("missing parent block during async fetch", "parent", current.Block.ParentID)
	var err error
	current.Prev, err = s.client.ExpandedBlock(current.Block.ParentID.String())
	if err != nil {
		slog.Error("failed to fetch parent block", "parent", current.Block.ParentID, "error", err)
		return
	}
	if current.Prev.Number >= s.hayabusaForkedBlock {
		current.ParentStaker, err = pos.FetchValidations(current.Block.ParentID, s.client)
		if err != nil {
			slog.Warn("failed to fetch parent staker info", "parent", current.Block.ParentID, "error", err)
		}
	}
}

// fetchHayabusaBlocks finds the Hayabusa fork block and the DPoS active block using a modified binary search
func fetchHayabusaBlocks(client *thorclient.Client, staker *builtin.Staker, max uint32, min uint32) (uint32, uint32, error) {
	forkBlockCond := func(rev string) bool {
		return isHayabusaForked(client, rev)
	}

	activeBlockCond := func(rev string) bool {
		return isDposActive(staker, rev)
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

func binarySearchChainCondition(max, min uint32, condition func(string) bool) (uint32, error) {
	low := min
	high := max
	var mid uint32

	// First, check if the condition is already true at the minimum block
	// If so, we can quickly return min block
	condAtMin := condition(fmt.Sprintf("%d", min))
	if condAtMin {
		// Condition is true at min, so the transition happened before min
		// We can't find it in this range, return min as the best guess
		return min, nil
	}

	// Check if condition is false at max - if so, the transition hasn't happened yet
	condAtMax := condition(fmt.Sprintf("%d", max))
	if !condAtMax {
		// Condition is false at max, so the transition hasn't happened yet
		return math.MaxUint32, nil
	}

	// Now we know: condition is false at min, true at max
	// Find the first block where condition becomes true
	for low < high {
		mid = (low + high) / 2
		cond := condition(fmt.Sprintf("%d", mid))

		if cond {
			// Condition is true at mid, check if this is the transition point
			if mid == min {
				// We're at the minimum, this must be the transition
				return mid, nil
			}

			// Check the previous block to see if this is the transition
			prev := mid - 1
			if prev >= min {
				prevCond := condition(fmt.Sprintf("%d", prev))
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
