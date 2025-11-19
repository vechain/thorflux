package pos

import (
	_ "embed"

	"log/slog"
	"math/big"
	"strconv"
	"time"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/api"
	builtin2 "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/types"
	"github.com/vechain/thorflux/vetutil"
)

func (s *Staker) Write(event *types.Event) []*write.Point {
	if !event.HayabusaStatus.Forked {
		return nil
	}
	points := make([]*write.Point, 0)

	var err error
	if event.ParentStaker == nil {
		slog.Warn("No parent staker information available", "block", event.Block.Number)
		event.ParentStaker, err = FetchValidations(event.Block.ParentID, s.client)
		if err != nil {
			slog.Error("Failed to fetch parent staker information", "error", err)
		}
	}
	if event.Staker == nil {
		slog.Warn("No staker information available", "block", event.Block.Number)
		event.Staker, err = FetchValidations(event.Block.ID, s.client)
		if err != nil {
			slog.Error("Failed to fetch staker information", "error", err)
			return nil
		}
	}

	singleValidatorPoints := s.createSingleValidatorStats(event, event.Staker)
	points = append(points, singleValidatorPoints...)

	overviewPoints := s.createValidatorOverview(event, event.Staker)
	points = append(points, overviewPoints...)

	energyPoints, err := s.createEnergyStats(event, event.Staker)
	if err != nil {
		slog.Error("Failed to write energy stats", "error", err)
	} else {
		points = append(points, energyPoints...)
	}

	blockPoints, err := s.createBlockPoints(event, event.Staker)
	if err != nil {
		slog.Error("Failed to create block points", "error", err)
	} else {
		points = append(points, blockPoints...)
	}

	missedSlotsPoints, err := s.createSlotPoints(event, event.Staker)
	if err != nil {
		slog.Error("Failed to create missed slots points", "error", err)
	}
	points = append(points, missedSlotsPoints...)
	validPoints := make([]*write.Point, 0, len(points))
	for _, point := range points {
		if point == nil {
			continue
		}
		fields := point.FieldList()
		if len(fields) == 0 {
			continue
		}
		validPoints = append(validPoints, point)
	}

	return validPoints
}

func (s *Staker) createBlockPoints(event *types.Event, _ *types.StakerInformation) ([]*write.Point, error) {
	abi := s.staker.Raw().ABI()

	eventAbiByHash := make(map[thor.Bytes32]ethabi.Event)
	for _, e := range abi.Events {
		eventAbiByHash[thor.Bytes32(e.Id())] = e
	}
	eventsByTopic := make(map[thor.Bytes32][]*api.JSONEvent)

	for _, tx := range event.Block.Transactions {
		for _, output := range tx.Outputs {
			for _, log := range output.Events {
				if log.Address != builtin2.Staker.Address {
					continue // Skip logs that are not from the staker contract
				}
				if eventsByTopic[log.Topics[0]] == nil {
					eventsByTopic[log.Topics[0]] = make([]*api.JSONEvent, 0)
				}
				eventsByTopic[log.Topics[0]] = append(eventsByTopic[log.Topics[0]], log)
			}
		}
	}

	points, err := s.ProcessEvents(event.Block.ID, eventsByTopic, eventAbiByHash, event.Timestamp)
	if err != nil {
		slog.Error("Failed to process events", "error", err)
		return nil, err
	}

	flags := make(map[string]interface{})
	for signature, events := range eventsByTopic {
		abiEvent, ok := eventAbiByHash[signature]
		if !ok {
			slog.Warn("Event ABI not found", "event", signature)
			continue
		}
		flags[abiEvent.Name] = len(events)
	}
	if len(flags) > 1 {
		eventTotalsPoint := influxdb2.NewPoint(
			"staker_events",
			event.DefaultTags,
			flags,
			event.Timestamp,
		)
		points = append(points, eventTotalsPoint)
	}

	return points, nil
}

func (s *Staker) createValidatorOverview(event *types.Event, info *types.StakerInformation) []*write.Point {
	block := event.Block
	epoch := block.Number / thor.EpochLength()

	leaderGroup := make(map[thor.Address]*types.Validation)

	onlineValidators := 0
	onlineStake := big.NewInt(0)
	onlineWeight := uint64(0)

	accumulatedStake := uint64(0)
	accumulatedWeight := uint64(0)

	offlineValidators := 0
	offlineStake := big.NewInt(0)
	offlineWeight := uint64(0)
	exitingVET := big.NewInt(0)

	// accumulated stakes and weights. We can use this to compare with contract totals
	for _, v := range info.Validations {
		if v.Status != validation.StatusActive {
			continue
		}
		accumulatedStake += v.LockedVET
		accumulatedStake += vetutil.ScaleToVET(v.DelegatorStake)
		leaderGroup[v.Address] = v
		if v.Online {
			onlineValidators++
			onlineStake.Add(onlineStake, v.TotalLockedStake)
			onlineStake.Add(onlineStake, v.DelegatorStake)
			onlineWeight += v.Weight
		} else {
			offlineValidators++
			offlineStake.Add(offlineStake, v.TotalLockedStake)
			offlineStake.Add(offlineStake, v.DelegatorStake)
			offlineWeight += v.Weight
		}
		if v.ExitBlock != nil {
			exitingVET.Add(exitingVET, v.TotalExitingStake)
		}
		accumulatedWeight += v.Weight
	}

	withdrawnFunds := big.NewInt(0)
	for _, tx := range event.Block.Transactions {
		for _, output := range tx.Outputs {
			for _, transfer := range output.Transfers {
				if transfer.Sender != builtin2.Staker.Address {
					continue // Skip transfers not from the staker contract
				}
				withdrawnFunds.Add(withdrawnFunds, (*big.Int)(transfer.Amount))
			}
		}
	}

	flags := map[string]interface{}{
		"total_stake":               vetutil.ScaleToVET(big.NewInt(0).Add(info.TotalVET, info.QueuedVET)),
		"active_stake":              vetutil.ScaleToVET(info.TotalVET),
		"active_stake_accumulated":  accumulatedStake,
		"active_weight":             vetutil.ScaleToVET(info.TotalWeight),
		"active_weight_accumulated": onlineWeight,
		"accumulated_weight":        accumulatedWeight,
		"queued_stake":              vetutil.ScaleToVET(info.QueuedVET),
		"withdrawn_vet":             vetutil.ScaleToVET(withdrawnFunds),
		"contract_vet":              vetutil.ScaleToVET(info.ContractBalance),
		"cooldown_vet_contract":     info.CooldownVET,
		"withdrawable_vet_contract": info.WithdrawableVET,
		"exiting_vet":               vetutil.ScaleToVET(exitingVET),
		"online_stake":              vetutil.ScaleToVET(onlineStake),
		"offline_stake":             vetutil.ScaleToVET(offlineStake),
		"online_weight":             onlineWeight,
		"offline_weight":            offlineWeight,
		"epoch":                     epoch,
		"block_in_epoch":            block.Number % thor.EpochLength(),
		"active_validators":         len(leaderGroup),
		"online_validators":         onlineValidators,
		"offline_validators":        offlineValidators,
		"block_number":              block.Number,
	}

	if event.HayabusaStatus.Active {
		signer, ok := leaderGroup[event.Block.Signer]
		if ok {
			signerProbability := float64(signer.Weight) * 100
			signerProbability = signerProbability / float64(onlineWeight)
			flags["signer_probability"] = signerProbability
			flags["weight_processed"] = signer.Weight
		}
	}

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"validator_overview",
		event.DefaultTags,
		flags,
		time.Unix(int64(block.Timestamp), 0),
	)

	return []*write.Point{heatmapPoint}
}

func (s *Staker) createEnergyStats(event *types.Event, info *types.StakerInformation) ([]*write.Point, error) {
	if !event.HayabusaStatus.Active {
		return nil, nil
	}

	block := event.Block
	epoch := block.Number / s.epochLength

	totalSupply := info.VTHO.TotalSupply
	totalBurned := info.VTHO.TotalBurned
	parentTotalSupply := event.ParentStaker.VTHO.TotalSupply
	parentTotalBurned := event.ParentStaker.VTHO.TotalBurned

	// Validate data before processing
	if parentTotalSupply == nil || parentTotalSupply.Cmp(big.NewInt(0)) <= 0 ||
		parentTotalBurned == nil || parentTotalBurned.Cmp(big.NewInt(0)) <= 0 {
		return nil, nil
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
		event.DefaultTags,
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

	return []*write.Point{heatmapPoint}, nil
}

func statusToString(status validation.Status) string {
	switch status {
	case validation.StatusQueued:
		return "queued"
	case validation.StatusActive:
		return "active"
	case validation.StatusExit:
		return "exited"
	default:
		return "unknown"
	}
}

func (s *Staker) createSingleValidatorStats(ev *types.Event, info *types.StakerInformation) []*write.Point {
	queueOrder := make(map[thor.Address]int)
	queueCount := 0
	for _, validator := range info.Validations {
		if validator.Status == validation.StatusQueued {
			queueOrder[validator.Address] = queueCount
			queueCount++
		}
	}
	var prevValidators map[thor.Address]*types.Validation
	if ev.ParentStaker == nil {
		prevValidators = make(map[thor.Address]*types.Validation)
	} else {
		prevValidators = ev.ParentStaker.ValidationMap()
	}
	validators := ev.Staker.ValidationMap()

	points := make([]*write.Point, 0)

	// process previous validators that are not in the current set
	// this is useful for tracking exits and status changes
	for addr, validator := range prevValidators {
		_, ok := validators[addr]
		if ok {
			continue
		}
		flags := map[string]any{
			"status_changed": statusToString(validation.StatusExit),
		}
		exitType := "previously_queued"
		exited := validator.ExitBlock != nil
		if exited {
			flags["cooldown_vet"] = validator.LockedVET
			exitType = "previously_active"
		}

		p := influxdb2.NewPoint(
			"individual_validators",
			map[string]string{
				"validator":             addr.String(),
				"endorsor":              validator.Endorser.String(),
				"status":                statusToString(validation.StatusExit),
				"signalled_exit":        strconv.FormatBool(exited),
				"staking_period_length": strconv.FormatUint(uint64(validator.Period), 10),
				"exit_type":             exitType,
			},
			flags,
			ev.Timestamp,
		)
		points = append(points, p)

	}

	// process all current validators, queued and active
	for _, validator := range info.Validations {
		multiplier := uint64(1)
		if validator.DelegatorStake.Sign() > 0 {
			multiplier = uint64(2)
		}
		flags := map[string]any{
			"online":            validator.Online,
			"start_block":       validator.StartBlock,
			"completed_periods": validator.CompletedPeriods,
			"current_block":     ev.Block.Number,

			// combined totals, validator + delegators
			"total_staked":       vetutil.ScaleToVET(validator.TotalLockedStake),
			"total_weight":       validator.Weight,
			"total_queued_vet":   vetutil.ScaleToVET(validator.TotalQueuedStake),
			"total_exiting_vet":  vetutil.ScaleToVET(validator.TotalExitingStake),
			"next_period_weight": vetutil.ScaleToVET(validator.NextPeriodWeight),

			// validator only
			"validator_staked": validator.LockedVET,

			"validator_weight":     validator.LockedVET * multiplier,
			"validator_queued_vet": validator.QueuedVET,

			// delegator only
			"delegators_staked":    vetutil.ScaleToVET(validator.DelegatorStake),
			"delegators_weight":    vetutil.ScaleToVET(validator.DelegatorWeight),
			"delegator_queued_vet": vetutil.ScaleToVET(validator.DelegatorQueuedStake),
		}
		if validator.OfflineBlock != nil {
			flags["offline_block"] = *validator.OfflineBlock
		}
		if validator.ExitBlock != nil {
			flags["exit_block"] = *validator.ExitBlock
		}

		prevEntry, ok := prevValidators[validator.Address]
		if ok {
			if prevEntry.Weight != validator.Weight {
				flags["weight_changed"] = validator.Weight - prevEntry.Weight
			}
			if prevEntry.LockedVET != validator.LockedVET {
				flags["stake_changed"] = validator.LockedVET - prevEntry.LockedVET
			}
			if prevEntry.ExitBlock != nil && validator.ExitBlock != nil {
				if *prevEntry.ExitBlock != *validator.ExitBlock {
					flags["exit_block_changed"] = *validator.ExitBlock
				}
			}
		}
		if prevEntry == nil || prevEntry.Online != validator.Online {
			flags["online_changed"] = strconv.FormatBool(validator.Online)
		}
		if prevEntry == nil || prevEntry.Status != validator.Status {
			flags["status_changed"] = statusToString(validator.Status)
		}
		if validator.Status == validation.StatusQueued {
			flags["queue_position"] = queueOrder[validator.Address]
		}
		p := influxdb2.NewPoint(
			"individual_validators",
			map[string]string{
				"validator":             validator.Address.String(),
				"endorsor":              validator.Endorser.String(),
				"status":                statusToString(validator.Status),
				"signalled_exit":        strconv.FormatBool(validator.ExitBlock != nil),
				"staking_period_length": strconv.FormatUint(uint64(validator.Period), 10),
			},
			flags,
			ev.Timestamp,
		)

		points = append(points, p)
	}

	return points
}

func (s *Staker) createSlotPoints(event *types.Event, info *types.StakerInformation) ([]*write.Point, error) {
	if !event.HayabusaStatus.Active {
		return nil, nil
	}

	points := make([]*write.Point, 0)

	// record missed slots
	missedOnline, missedOffline, err := s.MissedSlots(event.Prev, info.Validations, event.Block, event.Seed)
	if err != nil {
		slog.Error("Failed to get missed slots", "error", err)
		return nil, err
	}
	for _, v := range missedOnline {
		slog.Warn("Missed slot for an online validator", "validator", v.Signer, "block", event.Block.Number)
		point := influxdb2.NewPoint(
			"dpos_missed_slots",
			map[string]string{
				"signer": v.Signer.String(),
			},
			map[string]interface{}{
				"block_number": event.Block.Number,
			},
			event.Timestamp,
		)
		points = append(points, point)
	}
	for _, v := range missedOffline {
		missType := "went-offline"
		if !v.WasOnline {
			missType = "previously-offline"
		}

		point := influxdb2.NewPoint(
			"dpos_offline_missed_slots",
			map[string]string{
				"signer": v.Signer.String(),
				"type":   missType,
			},
			map[string]interface{}{
				"block_number": event.Block.Number,
			},
			event.Timestamp,
		)
		points = append(points, point)
	}

	// record future slots
	future, err := s.FutureSlots(info.Validations, event.Block, event.Seed)
	if err != nil {
		slog.Error("Failed to get future slots", "error", err)
		return nil, err
	}
	for _, f := range future {
		point := influxdb2.NewPoint(
			"dpos_future_slots",
			map[string]string{
				"signer": f.Signer.String(),
				"index":  strconv.FormatUint(uint64(f.Index), 10),
			},
			map[string]interface{}{
				"block_number":   f.Block,
				"block_in_epoch": f.Block % s.epochLength,
			},
			event.Timestamp,
		)
		points = append(points, point)
	}

	return points, nil
}
