package pos

import (
	"fmt"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"log/slog"
)

type Staker struct {
	staker *builtin.Staker
	client *thorclient.Client
	cache  *validators
}

func NewStaker(client *thorclient.Client) (*Staker, error) {
	staker, err := builtin.NewStaker(client)
	if err != nil {
		return nil, err
	}
	cache, err := newValidatorCache(staker, client)
	if err != nil {
		slog.Error("Failed to create validator cache", "error", err)
		return nil, err
	}
	return &Staker{staker: staker, client: client, cache: cache}, nil
}

func (s *Staker) GetValidators(block, parent *blocks.JSONExpandedBlock) (map[thor.Bytes32]*builtin.Validator, error) {
	return s.cache.Get(block, block.Timestamp-parent.Timestamp > 10)
}

func (s *Staker) NextValidator(block, parent *blocks.JSONExpandedBlock, seed []byte) (*thor.Address, error) {
	validators, err := s.GetValidators(block, parent)
	if err != nil {
		return nil, fmt.Errorf("failed to get validators: %w", err)
	}
	proposers := make(map[thor.Bytes32]*staker.Validation)
	for id, v := range validators {
		// scheduler doesn't need any other fields
		proposers[id] = &staker.Validation{
			Master: *v.Master,
			Online: v.Online,
			Weight: v.Weight,
		}
	}

	sched, err := pos.NewScheduler(block.Signer, proposers, block.Number, block.Timestamp, seed)
	if err != nil {
		return nil, err
	}
	for id, v := range validators {
		if sched.IsScheduled(block.Timestamp+10, id) {
			return v.Master, nil
		}
	}
	slog.Warn("No expected validator found for current block", "block", block.ID, "seed", fmt.Sprintf("%x", seed))
	return nil, fmt.Errorf("no expected validator found for current block %s", block.ID)
}
