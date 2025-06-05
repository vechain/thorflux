package pos

import (
	"fmt"

	"log/slog"
	"math/big"
	"strings"

	"github.com/vechain/thor/v2/api/blocks"
	builtin2 "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
)

type DataExtractor struct {
	Thor *thorclient.Client
}

func (posData *DataExtractor) ExtractCandidates(staker *builtin.Staker) (map[thor.Bytes32]*builtin.Validator, error) {
	var (
		entry  *builtin.Validator
		prevID thor.Bytes32
		err    error
	)
	entry, prevID, err = staker.FirstActive()
	if err != nil && strings.Contains(err.Error(), "no active validators") {
		slog.Info("No active validators found, skipping initialization")
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	validators := make(map[thor.Bytes32]*builtin.Validator)
	validators[prevID] = entry
	for !prevID.IsZero() {
		entry, prevID, err = staker.Next(prevID)
		if err != nil && strings.Contains(err.Error(), "no next validator") {
			break
		}
		if err != nil {
			return nil, err
		}
		validators[prevID] = entry
	}
	return validators, nil
}

func ExpectedValidator(candidates map[thor.Bytes32]*builtin.Validator, currentBlock *blocks.JSONExpandedBlock, seed []byte) (*thor.Address, error) {
	proposers := make(map[thor.Bytes32]*staker.Validation)
	for id, v := range candidates {
		// scheduler doesn't need any other fields
		proposers[id] = &staker.Validation{
			Master: *v.Master,
			Online: v.Online,
			Weight: v.Weight,
		}
	}
	sched, err := pos.NewScheduler(currentBlock.Signer, proposers, currentBlock.Number, currentBlock.Timestamp, seed)
	if err != nil {
		return nil, err
	}
	for id, v := range candidates {
		if sched.IsScheduled(currentBlock.Timestamp+10, id) {
			return v.Master, nil
		}
	}
	slog.Warn("No expected validator found for current block", "block", currentBlock.ID, "seed", fmt.Sprintf("%x", seed))
	return nil, fmt.Errorf("no expected validator found for current block %s", currentBlock.ID)
}

func (posData *DataExtractor) IsHayabusaFork() bool {
	code, err := posData.Thor.AccountCode(&builtin2.Staker.Address)
	if err != nil {
		slog.Error("Failed to get contract code", "error", err)
		return false
	}

	if len(code.Code) == 0 || code.Code == "0x" {
		return false
	}

	return true
}

func (posData *DataExtractor) IsHayabusaActive() bool {
	posActiveTimeKey := thor.BytesToBytes32([]byte("hayabusa-energy-growth-stop-time"))
	posActivated, err := posData.Thor.AccountStorage(&builtin2.Energy.Address, &posActiveTimeKey)
	if err != nil {
		slog.Error("Failed to get pos activated time", "error", err)
		return false
	}

	if len(posActivated.Value) == 0 || posActivated.Value == "0x" {
		return false
	}

	posTime, err := thor.ParseBytes32(posActivated.Value)
	if err != nil {
		return false
	}

	posTimeParsed := big.NewInt(0).SetBytes(posTime.Bytes())

	return posTimeParsed.Cmp(big.NewInt(0)) > 0
}
