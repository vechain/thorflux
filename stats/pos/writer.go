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
	stakers, err := s.FetchAll(event.Block.ID)
	if err != nil {
		slog.Error("Failed to fetch all stakers", "error", err)
		return err
	}
	slog.Info("Fetched all stakers", "block", event.Block.Number, "count", len(stakers), "duration", time.Since(start))

	resultChan := make(chan writeResult, 4)

	// Launch concurrent goroutines
	go func() {
		err := s.writeEpochStats(event, stakers)
		resultChan <- writeResult{"writeEpochStats", err}
	}()

	go func() {
		err := s.appendHayabusaEpochGasStats(event)
		resultChan <- writeResult{"appendHayabusaEpochGasStats", err}
	}()

	go func() {
		err := s.writeSingleValidatorStats(event, stakers)
		resultChan <- writeResult{"writeSingleValidatorStats", err}
	}()

	go func() {
		err := s.writeEventStats(event)
		resultChan <- writeResult{"writeEventStats", err}
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

func (s *Staker) writeEventStats(event *types.Event) error {
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

func (s *Staker) writeEpochStats(event *types.Event, validators []*builtin.Validator) error {
	block := event.Block
	epoch := block.Number / s.epochLength

	staker := s.staker.Revision(block.ID.String())
	ext, err := NewExtension(s.client)
	if err != nil {
		slog.Error("Failed to create extension instance", "error", err)
		return err
	}
	ext = ext.Revision(block.ID.String())

	// Make API calls concurrently
	type stakeResult struct {
		totalStakedVet *big.Int
		totalWeightVet *big.Int
		err            error
	}
	type queuedResult struct {
		totalQueuedVet    *big.Int
		totalQueuedWeight *big.Int
		err               error
	}
	type supplyResult struct {
		totalCirculatingVet *big.Int
		err                 error
	}

	stakeChan := make(chan stakeResult, 1)
	queuedChan := make(chan queuedResult, 1)
	supplyChan := make(chan supplyResult, 1)

	// Launch concurrent goroutines
	go func() {
		totalStakedVet, totalWeightVet, err := staker.TotalStake()
		stakeChan <- stakeResult{totalStakedVet, totalWeightVet, err}
	}()

	go func() {
		totalQueuedVet, totalQueuedWeight, err := staker.QueuedStake()
		queuedChan <- queuedResult{totalQueuedVet, totalQueuedWeight, err}
	}()

	go func() {
		totalCirculatingVet, err := ext.TotalSupply()
		supplyChan <- supplyResult{totalCirculatingVet, err}
	}()
	// Wait for all results
	stakeRes := <-stakeChan
	queuedRes := <-queuedChan
	supplyRes := <-supplyChan

	// Handle errors
	if stakeRes.err != nil {
		slog.Error("Failed to fetch total stake for hayabusa", "error", stakeRes.err)
		return stakeRes.err
	}
	if queuedRes.err != nil {
		slog.Error("Failed to fetch active stake for hayabusa", "error", queuedRes.err)
		return queuedRes.err
	}
	if supplyRes.err != nil {
		slog.Error("Failed to fetch total circulating VET", "error", supplyRes.err)
		return supplyRes.err
	}

	// Extract values from results
	totalStakedVet := stakeRes.totalStakedVet
	totalWeightVet := stakeRes.totalWeightVet
	totalQueuedVet := queuedRes.totalQueuedVet
	totalQueuedWeight := queuedRes.totalQueuedWeight
	totalCirculatingVet := supplyRes.totalCirculatingVet
	totalCirculatingVet.Div(totalCirculatingVet, big.NewInt(1e3))
	candidateProbability := make(map[string]interface{})

	flags := map[string]interface{}{
		"total_stake":     vetutil.ScaleToVET(big.NewInt(0).Add(totalStakedVet, totalQueuedVet)),
		"active_stake":    vetutil.ScaleToVET(totalStakedVet),
		"active_weight":   vetutil.ScaleToVET(totalWeightVet),
		"queued_stake":    vetutil.ScaleToVET(totalQueuedVet),
		"queued_weight":   vetutil.ScaleToVET(totalQueuedWeight),
		"circulating_vet": vetutil.ScaleToVET(totalCirculatingVet),
		"epoch":           strconv.FormatUint(uint64(epoch), 10),
	}

	candidates := make(map[thor.Address]*builtin.Validator)
	onlineValidators := 0
	for _, v := range validators {
		if v.Status == builtin.StakerStatusActive {
			candidates[v.Address] = v
		}
		if v.Online {
			onlineValidators++
		}
	}

	flags["active_validators"] = len(candidates)
	flags["online_validators"] = onlineValidators

	if event.DPOSActive {
		expectedValidator := &thor.Address{}
		if len(candidates) > 0 {
			expectedValidator, err = s.NextValidator(candidates, event.Block, event.Seed)
			if err != nil {
				slog.Error("Cannot extract expected validator", "error", err)
			}
		}

		onlineValidators := 0
		offlineValidators := 0
		for _, candidate := range candidates {
			probabilityValue := big.NewInt(0).Mul(candidate.Weight, big.NewInt(100))
			candidateProbability[candidate.Address.String()] = big.NewInt(0).Div(probabilityValue, totalWeightVet).Int64()
			if candidate.Online {
				onlineValidators += 1
			} else {
				offlineValidators += 1
			}
		}
		flags["online_validators"] = onlineValidators
		flags["offline_validators"] = offlineValidators
		flags["next_validator"] = expectedValidator.String()
	}

	if len(candidateProbability) > 0 {
		heatmapPointProbability := influxdb2.NewPoint(
			"hayabusa_probability",
			map[string]string{
				"chain_tag": event.ChainTag,
			},
			candidateProbability,
			time.Unix(int64(block.Timestamp), 0),
		)

		if err := event.WriteAPI.WritePoint(context.Background(), heatmapPointProbability); err != nil {
			slog.Error("Failed to write heatmap point", "error", err)
		}
	}

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"hayabusa_validators",
		map[string]string{
			"chain_tag": event.ChainTag,
		},
		flags,
		time.Unix(int64(block.Timestamp), 0),
	)

	return event.WriteAPI.WritePoint(context.Background(), heatmapPoint)
}

func (s *Staker) appendHayabusaEpochGasStats(event *types.Event) error {
	if !event.DPOSActive {
		return nil
	}

	block := event.Block
	epoch := block.Number / s.epochLength
	energy, err := builtin.NewEnergy(s.client)
	if err != nil {
		slog.Error("Failed to create energy instance", "error", err)
		return err
	}

	// Create revision instances once
	currentEnergyRevision := energy.Revision(block.ID.String())
	parentEnergyRevision := energy.Revision(block.ParentID.String())

	// Make API calls concurrently
	type supplyResult struct {
		value *big.Int
		err   error
	}
	type burnedResult struct {
		value *big.Int
		err   error
	}

	totalSupplyChan := make(chan supplyResult, 1)
	parentTotalSupplyChan := make(chan supplyResult, 1)
	totalBurnedChan := make(chan burnedResult, 1)
	parentTotalBurnedChan := make(chan burnedResult, 1)

	// Launch concurrent goroutines
	go func() {
		value, err := currentEnergyRevision.TotalSupply()
		totalSupplyChan <- supplyResult{value, err}
	}()

	go func() {
		value, err := parentEnergyRevision.TotalSupply()
		parentTotalSupplyChan <- supplyResult{value, err}
	}()

	go func() {
		value, err := currentEnergyRevision.TotalBurned()
		totalBurnedChan <- burnedResult{value, err}
	}()

	go func() {
		value, err := parentEnergyRevision.TotalBurned()
		parentTotalBurnedChan <- burnedResult{value, err}
	}()

	// Wait for all results
	totalSupplyRes := <-totalSupplyChan
	parentTotalSupplyRes := <-parentTotalSupplyChan
	totalBurnedRes := <-totalBurnedChan
	parentTotalBurnedRes := <-parentTotalBurnedChan

	// Handle errors
	if totalSupplyRes.err != nil {
		slog.Error("Failed to fetch energy total supply", "error", totalSupplyRes.err)
		return totalSupplyRes.err
	}
	if parentTotalSupplyRes.err != nil {
		slog.Error("Failed to fetch parent energy total supply", "error", parentTotalSupplyRes.err)
		return parentTotalSupplyRes.err
	}
	if totalBurnedRes.err != nil {
		slog.Error("Failed to fetch energy total burned", "error", totalBurnedRes.err)
		return totalBurnedRes.err
	}
	if parentTotalBurnedRes.err != nil {
		slog.Error("Failed to fetch parent energy total burned", "error", parentTotalBurnedRes.err)
		return parentTotalBurnedRes.err
	}

	// Extract values from results
	totalSupply := totalSupplyRes.value
	parentTotalSupply := parentTotalSupplyRes.value
	totalBurned := totalBurnedRes.value
	parentTotalBurned := parentTotalBurnedRes.value

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

	issuedBurnedRatio, _ := new(big.Rat).Quo(new(big.Rat).SetInt(big.NewInt(0).Abs(vthoIssued)), new(big.Rat).SetInt(vthoBurnedDivider)).Float64()

	validatorsShare := big.NewInt(0).Mul(vthoIssued, big.NewInt(3))
	validatorsShare = validatorsShare.Div(validatorsShare, big.NewInt(10))

	delegatorsShare := big.NewInt(0).Mul(vthoIssued, big.NewInt(7))
	delegatorsShare = delegatorsShare.Div(delegatorsShare, big.NewInt(10))

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

func (s *Staker) writeSingleValidatorStats(ev *types.Event, validators []*builtin.Validator) error {
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
			"period":        validator.Period,
			"staked_amount": vetutil.ScaleToVET(validator.Stake),
			"weight":        vetutil.ScaleToVET(validator.Weight),
			"online":        validator.Online,
			"start_block":   validator.StartBlock,
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
				"chain_tag":      ev.ChainTag,
				"staker":         validator.Address.String(),
				"endorsor":       validator.Endorsor.String(),
				"status":         statusToString(validator.Status),
				"signalled_exit": strconv.FormatBool(validator.ExitBlock != math.MaxUint32),
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
