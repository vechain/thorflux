package pos

import (
	"context"
	_ "embed"

	"log/slog"
	"math"
	"math/big"
	"strconv"
	"time"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thorflux/types"
	"github.com/vechain/thorflux/vetutil"
)

func (s *Staker) Write(event *types.Event) error {
	if !event.HayabusaForked {
		return nil
	}

	// Execute all write operations concurrently
	type writeResult struct {
		name string
		err  error
	}

	start := time.Now()
	stakerInfo, err := s.FetchAll(event.Block.ID)
	if err != nil {
		slog.Error("Failed to fetch all stakers", "error", err)
		return err
	}
	slog.Info("Fetched all stakers", "block", event.Block.Number, "count", len(stakerInfo.Validations), "duration", time.Since(start))

	resultChan := make(chan writeResult, 4)

	// Launch concurrent goroutines
	go func() {
		err := s.writeValidatorOverview(event, stakerInfo)
		resultChan <- writeResult{"writeValidatorOverview", err}
	}()
	go func() {
		err := s.writeEnergyStats(event, stakerInfo)
		resultChan <- writeResult{"writeEnergyStats", err}
	}()
	go func() {
		err := s.writeSingleValidatorStats(event, stakerInfo.Validations)
		resultChan <- writeResult{"writeSingleValidatorStats", err}
	}()
	go func() {
		err := s.writeBlockStats(event)
		resultChan <- writeResult{"writeBlockStats", err}
	}()

	// Wait for all results and collect errors
	var errors []error
	for i := 0; i < 3; i++ {
		result := <-resultChan
		if result.err != nil {
			errors = append(errors, result.err)
		}
	}

	// Return the first error if any occurred
	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

func iterateEvent(block *api.JSONExpandedBlock, eventSignature thor.Bytes32, callback func(*api.JSONEvent)) {
	for _, tx := range block.Transactions {
		for _, output := range tx.Outputs {
			for _, log := range output.Events {
				if eventSignature.IsZero() { // If eventSignature is zero, we return all events
					callback(log)
				} else if log.Topics[0] == eventSignature { // If eventSignature is not zero, we filter by the event signature
					callback(log)
				}
			}
		}
	}
}

func (s *Staker) writeBlockStats(event *types.Event) error {
	if !event.HayabusaForked {
		return nil
	}

	abi := s.staker.Raw().ABI()

	eventsByHash := make(map[thor.Bytes32]*ethabi.Event)
	for _, e := range abi.Events {
		eventsByHash[thor.Bytes32(e.Id())] = &e
	}

	flags := make(map[string]interface{})
	iterateEvent(event.Block, thor.Bytes32{}, func(log *api.JSONEvent) {
		ev, ok := eventsByHash[log.Topics[0]]
		if !ok {
			return
		}
		prev, ok := flags[ev.Name]
		if !ok {
			flags[ev.Name] = 1
		} else {
			flags[ev.Name] = prev.(int) + 1
		}
	})

	if len(flags) == 0 {
		return nil
	}

	point := influxdb2.NewPoint(
		"staker_events",
		map[string]string{
			"chain_tag": event.ChainTag,
		},
		flags,
		time.Unix(int64(event.Block.Timestamp), 0),
	)

	return event.WriteAPI.WritePoint(context.Background(), point)
}

func (s *Staker) writeValidatorOverview(event *types.Event, info *StakerInformation) error {
	block := event.Block
	epoch := block.Number / s.epochLength

	leaderGroup := make(map[thor.Address]*builtin.Validator)

	onlineValidators := 0
	onlineStake := big.NewInt(0)
	onlineWeight := big.NewInt(0)

	offlineValidators := 0
	offlineStake := big.NewInt(0)
	offlineWeight := big.NewInt(0)

	for _, v := range info.Validations {
		if v.Status != builtin.StakerStatusActive {
			continue
		}
		leaderGroup[v.Address] = v.Validator
		if v.Online {
			onlineValidators++
			onlineStake.Add(onlineStake, v.TotalStaked)
			onlineWeight.Add(onlineWeight, v.Weight)
		} else {
			offlineValidators++
			offlineStake.Add(offlineStake, v.TotalStaked)
			offlineWeight.Add(offlineWeight, v.Weight)
		}
	}

	flags := map[string]interface{}{
		"total_stake":   vetutil.ScaleToVET(big.NewInt(0).Add(info.TotalVET, info.QueuedVET)),
		"active_stake":  vetutil.ScaleToVET(info.TotalVET),
		"active_weight": vetutil.ScaleToVET(info.TotalWeight),
		"queued_stake":  vetutil.ScaleToVET(info.QueuedVET),
		"queued_weight": vetutil.ScaleToVET(info.QueuedWeight),
		// TODO: This is Circulating VTHO, not VET
		"circulating_vet":    vetutil.ScaleToVET(info.TotalSupplyVTHO),
		"online_stake":       vetutil.ScaleToVET(onlineStake),
		"offline_stake":      vetutil.ScaleToVET(offlineStake),
		"online_weight":      vetutil.ScaleToVET(onlineWeight),
		"offline_weight":     vetutil.ScaleToVET(offlineWeight),
		"epoch":              strconv.FormatUint(uint64(epoch), 10),
		"active_validators":  len(leaderGroup),
		"online_validators":  onlineValidators,
		"offline_validators": offlineValidators,
	}

	if event.DPOSActive {
		signer, ok := leaderGroup[event.Block.Signer]
		if ok {
			flags["signer_weight"] = vetutil.ScaleToVET(signer.Weight)
			signerProbability := big.NewFloat(0).Mul(big.NewFloat(0).SetInt(signer.Weight), big.NewFloat(100))
			signerProbability = signerProbability.Quo(signerProbability, big.NewFloat(0).SetInt(onlineWeight))
			probability, _ := signerProbability.Float64()
			flags["signer_probability"] = probability
		}
	}

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"validator_overview",
		map[string]string{
			"chain_tag": event.ChainTag,
		},
		flags,
		time.Unix(int64(block.Timestamp), 0),
	)

	return event.WriteAPI.WritePoint(context.Background(), heatmapPoint)
}

func (s *Staker) writeEnergyStats(event *types.Event, info *StakerInformation) error {
	if !event.DPOSActive {
		return nil
	}

	defer func() {
		s.prevVTHOSupply.Store(info.TotalSupplyVTHO)
		s.prevVTHOBurned.Store(info.TotalBurnedVTHO)
	}()

	block := event.Block
	epoch := block.Number / s.epochLength

	if (s.prevVTHOSupply.Load() == nil || s.prevVTHOBurned.Load() == nil) || event.Block.ParentID != event.Prev.ID {
		if err := s.setPrevTotals(event.Block.ParentID); err != nil {
			slog.Error("Failed to set previous totals", "error", err)
			return err
		}
	}

	// Extract values from results
	totalSupply := info.TotalSupplyVTHO
	totalBurned := info.TotalBurnedVTHO
	parentTotalSupply := s.prevVTHOSupply.Load()
	parentTotalBurned := s.prevVTHOBurned.Load()

	// Validate data before processing
	if parentTotalSupply == nil || parentTotalSupply.Cmp(big.NewInt(0)) <= 0 ||
		parentTotalBurned == nil || parentTotalBurned.Cmp(big.NewInt(0)) <= 0 {
		return nil
	}

	vthoIssued := big.NewInt(0).Sub(totalSupply, parentTotalSupply)
	vthoBurned := big.NewInt(0).Sub(totalBurned, parentTotalBurned)

	vthoBurnedDivider := vthoBurned
	if vthoBurned == nil || vthoBurned.Cmp(big.NewInt(0)) == 0 {
		vthoBurnedDivider = big.NewInt(1)
	}

	issuedBurnedRatio, _ := new(big.Rat).
		Quo(
			new(big.Rat).SetInt(big.NewInt(0).Abs(vthoIssued)),
			new(big.Rat).SetInt(vthoBurnedDivider),
		).Float64()

	validatorsShare := big.NewInt(0).Mul(vthoIssued, big.NewInt(3))
	validatorsShare = validatorsShare.Div(validatorsShare, big.NewInt(10))

	delegatorsShare := new(big.Int).Sub(vthoIssued, validatorsShare)

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"hayabusa_gas",
		map[string]string{
			"chain_tag": event.ChainTag,
		},
		map[string]interface{}{
			"vtho_issued":         vetutil.ScaleToVET(vthoIssued),
			"vtho_burned":         vetutil.ScaleToVET(vthoBurned),
			"issued_burned_ratio": issuedBurnedRatio,
			"validators_share":    vetutil.ScaleToVET(validatorsShare),
			"delegators_share":    vetutil.ScaleToVET(delegatorsShare),
			"epoch":               strconv.FormatUint(uint64(epoch), 10),
		},
		event.Timestamp,
	)

	return event.WriteAPI.WritePoint(context.Background(), heatmapPoint)
}

func statusToString(status builtin.StakerStatus) string {
	switch status {
	case builtin.StakerStatusQueued:
		return "queued"
	case builtin.StakerStatusActive:
		return "active"
	case builtin.StakerStatusExited:
		return "exited"
	default:
		return "unknown"
	}
}

func (s *Staker) writeSingleValidatorStats(ev *types.Event, validators []*Validation) error {
	queueOrder := make(map[thor.Address]int)
	queueCount := 0
	for _, validator := range validators {
		if validator.Status == builtin.StakerStatusQueued {
			queueOrder[validator.Address] = queueCount
			queueCount++
		}
	}

	type delegationAdded struct {
		count  int
		stake  uint64
		weight uint64
	}

	queuedDelegators := make(map[thor.Address]*delegationAdded)
	delegationAddedEv := s.staker.Raw().ABI().Events["DelegationAdded"].Id()

	// this looks for DelegationAdded and adds the count
	if delegationAddedEv.String() != (common.Hash{}).String() {
		iterateEvent(ev.Block, thor.Bytes32(delegationAddedEv), func(log *api.JSONEvent) {
			validatorAddr := thor.BytesToAddress(log.Topics[1][:])

			decoded := hexutil.MustDecode(log.Data)
			vet := big.NewInt(0).SetBytes(decoded[0:32])
			multiplier := big.NewInt(0).SetBytes(decoded[32:64])
			weight := big.NewInt(0).Mul(vet, multiplier)
			weight = weight.Div(weight, big.NewInt(100))

			record, ok := queuedDelegators[validatorAddr]
			if !ok {
				record = &delegationAdded{
					count:  0,
					stake:  0,
					weight: 0,
				}
				queuedDelegators[validatorAddr] = record
			}
			record.count++
			record.stake += vetutil.ScaleToVET(vet)
			record.weight += vetutil.ScaleToVET(weight)
		})
	} else {
		slog.Warn("DelegationAdded event not found in staker ABI, skipping queued delegators count")
	}

	for _, validator := range validators {
		flags := map[string]any{
			"staked_amount":     vetutil.ScaleToVET(validator.Stake),
			"weight":            vetutil.ScaleToVET(validator.Weight),
			"online":            validator.Online,
			"start_block":       validator.StartBlock,
			"completed_periods": validator.CompletedPeriods,
			"total_staked":      vetutil.ScaleToVET(validator.TotalStaked),
			"delegators_staked": vetutil.ScaleToVET(validator.DelegatorsStaked),
			"delegators_weight": vetutil.ScaleToVET(validator.DelegatorsWeight),
			"exit_block":        validator.ExitBlock,
		}

		delegatorsAdded, ok := queuedDelegators[validator.Address]
		if ok {
			flags["queued_delegators_count"] = delegatorsAdded.count
			flags["queued_delegators_stake"] = delegatorsAdded.stake
			flags["queued_delegators_weight"] = delegatorsAdded.weight
		}

		if validator.Status == builtin.StakerStatusQueued {
			flags["queue_position"] = queueOrder[validator.Address]
		}

		p := influxdb2.NewPoint(
			"individual_validators",
			map[string]string{
				"chain_tag":             ev.ChainTag,
				"staker":                validator.Address.String(),
				"endorsor":              validator.Endorsor.String(),
				"status":                statusToString(validator.Status),
				"signalled_exit":        strconv.FormatBool(validator.ExitBlock != math.MaxUint32),
				"staking_period_length": strconv.FormatUint(uint64(validator.Period), 10),
			},
			flags,
			ev.Timestamp,
		)

		if err := ev.WriteAPI.WritePoint(context.Background(), p); err != nil {
			return err
		}
	}

	return nil
}
