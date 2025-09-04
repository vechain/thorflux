package pos

import (
	"context"
	_ "embed"

	"log/slog"
	"math/big"
	"strconv"
	"time"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/types"
	"github.com/vechain/thorflux/vetutil"
)

func (s *Staker) Write(event *types.Event) error {
	if !event.HayabusaForked {
		return nil
	}

	stakerInfo, err := s.FetchAll(event.Block.ID)
	if err != nil {
		slog.Error("Failed to fetch all stakers", "error", err)
		return err
	}

	points := make([]*write.Point, 0)

	singleValidatorPoints := s.createSingleValidatorStats(event, stakerInfo)
	points = append(points, singleValidatorPoints...)

	overviewPoints := s.createValidatorOverview(event, stakerInfo)
	points = append(points, overviewPoints...)

	energyPoints, err := s.createEnergyStats(event, stakerInfo)
	if err != nil {
		slog.Error("Failed to write energy stats", "error", err)
	} else {
		points = append(points, energyPoints...)
	}

	blockPoints, err := s.createBlockPoints(event, stakerInfo)
	if err != nil {
		slog.Error("Failed to create block points", "error", err)
	} else {
		points = append(points, blockPoints...)
	}

	missedSlotsPoints, err := s.createSlotPoints(event, stakerInfo)
	if err != nil {
		slog.Error("Failed to create missed slots points", "error", err)
	}
	points = append(points, missedSlotsPoints...)

	for _, point := range points {
		if len(point.FieldList()) == 0 {
			slog.Warn("Skipping point with no fields", "point", point.Name())
		}
	}

	if err := event.WriteAPI.WritePoint(context.Background(), points...); err != nil {
		slog.Error("Failed to write points to InfluxDB", "error", err)
		return err
	}

	return nil
}

func (s *Staker) createBlockPoints(event *types.Event, _ *StakerInformation) ([]*write.Point, error) {
	abi := s.staker.Raw().ABI()

	eventAbiByHash := make(map[thor.Bytes32]ethabi.Event)
	for _, e := range abi.Events {
		eventAbiByHash[thor.Bytes32(e.Id())] = e
	}
	eventsByTopic := make(map[thor.Bytes32][]*api.JSONEvent)

	for _, tx := range event.Block.Transactions {
		for _, output := range tx.Outputs {
			for _, log := range output.Events {
				if log.Address != *s.staker.Raw().Address() {
					continue // Skip logs that are not from the staker contract
				}
				if allEvents, ok := eventsByTopic[log.Topics[0]]; !ok {
					allEvents = make([]*api.JSONEvent, 0)
					eventsByTopic[log.Topics[0]] = allEvents
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
			map[string]string{
				"chain_tag": event.ChainTag,
			},
			flags,
			event.Timestamp,
		)
		points = append(points, eventTotalsPoint)
	}

	return points, nil
}

func (s *Staker) createValidatorOverview(event *types.Event, info *StakerInformation) []*write.Point {
	block := event.Block
	epoch := block.Number / thor.EpochLength()

	leaderGroup := make(map[thor.Address]*Validation)

	onlineValidators := 0
	onlineStake := big.NewInt(0)
	onlineWeight := uint64(0)
	accumulatedStake := uint64(0)

	offlineValidators := 0
	offlineStake := big.NewInt(0)
	offlineWeight := uint64(0)

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
	}

	withdrawnFunds := big.NewInt(0)
	for _, tx := range event.Block.Transactions {
		for _, output := range tx.Outputs {
			for _, transfer := range output.Transfers {
				if transfer.Sender != *s.staker.Raw().Address() {
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
		"queued_stake":              vetutil.ScaleToVET(info.QueuedVET),
		"withdrawn_vet":             vetutil.ScaleToVET(withdrawnFunds),
		"contract_vet":              vetutil.ScaleToVET(info.ContractBalance),
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

	if event.DPOSActive {
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
		map[string]string{
			"chain_tag": event.ChainTag,
			"signer":    event.Block.Signer.String(),
		},
		flags,
		time.Unix(int64(block.Timestamp), 0),
	)

	return []*write.Point{heatmapPoint}
}

func (s *Staker) createEnergyStats(event *types.Event, info *StakerInformation) ([]*write.Point, error) {
	if !event.DPOSActive {
		return nil, nil
	}

	defer func() {
		s.prevVTHOSupply.Store(info.TotalSupplyVTHO)
		s.prevVTHOBurned.Store(info.TotalBurnedVTHO)
	}()

	block := event.Block
	epoch := block.Number / s.epochLength

	if (s.prevVTHOSupply.Load() == nil || s.prevVTHOBurned.Load() == nil) || event.Block.ParentID != event.Prev.ID {
		slog.Info("Fetching previous totals for VTHO supply and burned")
		if err := s.setPrevTotals(event.Block.ParentID); err != nil {
			slog.Error("Failed to set previous totals", "error", err)
			return nil, err
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
		map[string]string{
			"chain_tag": event.ChainTag,
			"signer":    event.Block.Signer.String(),
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

func (s *Staker) createSingleValidatorStats(ev *types.Event, info *StakerInformation) []*write.Point {
	queueOrder := make(map[thor.Address]int)
	queueCount := 0
	for _, validator := range info.Validations {
		if validator.Status == validation.StatusQueued {
			queueOrder[validator.Address] = queueCount
			queueCount++
		}
	}
	prevValidators, err := s.ValidatorMap(ev.Block.ParentID)
	if err != nil {
		slog.Error("Failed to get previous validators", "error", err)
	}
	validators, err := s.ValidatorMap(ev.Block.ID)
	if err != nil {
		slog.Error("Failed to get current validators", "error", err)
	}

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
				"chain_tag":             ev.ChainTag,
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
			"completed_periods": validator.CompleteIterations,
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
			if prevEntry.ExitBlock != prevEntry.ExitBlock {
				flags["exit_block_changed"] = prevEntry.ExitBlock
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
				"chain_tag":             ev.ChainTag,
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

func (s *Staker) createSlotPoints(event *types.Event, info *StakerInformation) ([]*write.Point, error) {
	if !event.DPOSActive {
		return nil, nil
	}

	points := make([]*write.Point, 0)

	// record missed slots
	missedOnline, missedOffline, err := s.MissedSlots(info.Validations, event.Block, event.Seed)
	if err != nil {
		slog.Error("Failed to get missed slots", "error", err)
		return nil, err
	}
	for _, v := range missedOnline {
		slog.Warn("Missed slot for an online validator", "validator", v.Signer, "block", event.Block.Number)
		point := influxdb2.NewPoint(
			"dpos_missed_slots",
			map[string]string{
				"chain_tag": event.ChainTag,
				"signer":    v.Signer.String(),
			},
			map[string]interface{}{
				"block_number": event.Block.Number,
			},
			event.Timestamp,
		)
		points = append(points, point)
	}
	for _, v := range missedOffline {
		point := influxdb2.NewPoint(
			"dpos_offline_missed_slots",
			map[string]string{
				"chain_tag": event.ChainTag,
				"signer":    v.Signer.String(),
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
				"chain_tag": event.ChainTag,
				"signer":    f.Signer.String(),
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
