package pubsub

import (
	"fmt"
	"github.com/vechain/thor/v2/api"

	"github.com/vechain/thorflux/types"
)

type EventBlockService struct {
	blockFetcher        *BlockFetcher
	hayabusaActiveBlock uint32
}

func NewEventBlockService(blockFetcher *BlockFetcher, hayabusaActiveBlock uint32) *EventBlockService {
	return &EventBlockService{
		blockFetcher:        blockFetcher,
		hayabusaActiveBlock: hayabusaActiveBlock,
	}
}

func (e *EventBlockService) ProcessBlock(blockNum uint32) (*BlockEvent, error) {
	// Fetch current block (N)
	currentResult, err := e.blockFetcher.FetchBlock(blockNum)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch block %d: %w", blockNum, err)
	}

	// Fetch previous block (N-1) if blockNum > 1
	var prevBlock *api.JSONExpandedBlock
	var parentStaker *types.StakerInformation
	var parentAuthNodes types.AuthorityNodeList
	var parentSeed []byte

	if blockNum > 1 {
		prevResult, err := e.blockFetcher.FetchBlock(blockNum - 1)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch previous block %d: %w", blockNum-1, err)
		}

		prevBlock = prevResult.Block
		parentStaker = prevResult.Staker
		parentAuthNodes = prevResult.AuthNodes
		parentSeed = prevResult.Seed
	}

	return &BlockEvent{
		Block:  currentResult.Block,
		Seed:   currentResult.Seed,
		Staker: currentResult.Staker,
		HayabusaStatus: types.HayabusaStatus{
			Active: blockNum >= e.hayabusaActiveBlock,
			Forked: blockNum >= e.blockFetcher.hayabusaForkedBlock,
		},
		Prev:            prevBlock,
		ParentStaker:    parentStaker,
		AuthNodes:       currentResult.AuthNodes,
		ParentAuthNodes: parentAuthNodes,
		ParentSeed:      parentSeed,
	}, nil
}
