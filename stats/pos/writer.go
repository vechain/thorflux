package pos

import (
	"context"
	"log/slog"
	"math/big"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thorflux/types"
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

	resultChan := make(chan writeResult, 3)

	// Launch concurrent goroutines
	go func() {
		err := s.writeEpochStats(event)
		resultChan <- writeResult{"writeEpochStats", err}
	}()

	go func() {
		err := s.appendHayabusaEpochGasStats(event)
		resultChan <- writeResult{"appendHayabusaEpochGasStats", err}
	}()

	go func() {
		err := s.appendStakerStats(event)
		resultChan <- writeResult{"appendStakerStats", err}
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

func (s *Staker) writeEpochStats(event *types.Event) error {
	block := event.Block
	epoch := block.Number / 180
	blockInEpoch := block.Number % 180

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
		"total_stake":     big.NewInt(0).Add(totalStakedVet, totalQueuedVet).Int64(),
		"active_stake":    totalStakedVet.Int64(),
		"active_weight":   totalWeightVet.Int64(),
		"queued_stake":    totalQueuedVet.Int64(),
		"queued_weight":   totalQueuedWeight.Int64(),
		"circulating_vet": totalCirculatingVet.Int64(),
		"epoch":           strconv.FormatUint(uint64(epoch), 10),
	}

	if event.DPOSActive {
		var candidates map[thor.Bytes32]*builtin.Validator
		if blockInEpoch == 0 || len(candidates) == 0 {
			candidates, err = s.GetValidators(block)
			if err != nil {
				slog.Error("Error while fetching validators", "error", err)
			}
		}

		expectedValidator := &thor.Address{}
		if len(candidates) > 0 {
			expectedValidator, err = s.NextValidator(block, event.Seed)
			if err != nil {
				slog.Error("Cannot extract expected validator", "error", err)
			}
		}

		onlineValidators := 0
		offlineValidators := 0
		for _, candidate := range candidates {
			probabilityValue := big.NewInt(0).Mul(candidate.Weight, big.NewInt(100))
			candidateProbability[candidate.Master.String()] = big.NewInt(0).Div(probabilityValue, totalWeightVet).Int64()
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
	epoch := block.Number / 180
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
			"vtho_issued":         vthoIssued.Int64(),
			"vtho_burned":         vthoBurned.Int64(),
			"issued_burned_ratio": issuedBurnedRatio,
			"validators_share":    validatorsShare.Int64(),
			"delegators_share":    delegatorsShare.Int64(),
			"epoch":               strconv.FormatUint(uint64(epoch), 10),
		},
		event.Timestamp,
	)

	return event.WriteAPI.WritePoint(context.Background(), heatmapPoint)
}

func (s *Staker) appendStakerStats(ev *types.Event) error {
	stakerStats := NewStakerStats()

	if err := stakerStats.CollectActiveStakers(s, ev.Block, ev.DPOSActive); err != nil {
		slog.Error("Failed to collect active stakers", "error", err)
	}

	txs := ev.Block.Transactions
	for _, tx := range txs {
		for _, output := range tx.Outputs {
			for _, event := range output.Events {
				stakerStats.processEvent(event)
			}
		}
	}

	for _, staker := range stakerStats.AddStaker {
		p := influxdb2.NewPoint(
			"queued_stakers",
			map[string]string{
				"chain_tag": ev.ChainTag,
				"staker":    staker.Master.String(),
			},
			map[string]any{
				"period":        staker.Period,
				"auto_renew":    staker.AutoRenew,
				"staked_amount": staker.Stake,
			},
			ev.Timestamp,
		)

		if err := ev.WriteAPI.WritePoint(context.Background(), p); err != nil {
			return err
		}
	}

	for _, staker := range stakerStats.StakersStatus {
		p := influxdb2.NewPoint(
			"stakers_status",
			map[string]string{
				"chain_tag": ev.ChainTag,
				"staker":    staker.Master.String(),
			},
			map[string]any{
				"auto_renew":    staker.AutoRenew,
				"status":        int(staker.Status),
				"staked_amount": staker.Stake.Uint64(),
			},
			ev.Timestamp,
		)

		if err := ev.WriteAPI.WritePoint(context.Background(), p); err != nil {
			return err
		}
	}

	return nil
}
