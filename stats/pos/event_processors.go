package pos

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thorflux/vetutil"
	"log/slog"
	"math/big"
	"strconv"
	"time"
)

func (s *Staker) ProcessEvents(
	revision thor.Bytes32,
	events map[thor.Bytes32][]*api.JSONEvent,
	eventABIs map[thor.Bytes32]abi.Event,
	timestamp time.Time,
) ([]*write.Point, error) {

	points := make([]*write.Point, 0)

	for _, logs := range events {
		for _, log := range logs {
			eventABI, ok := eventABIs[log.Topics[0]]
			if !ok {
				continue
			}
			var processor func(thor.Bytes32, *api.JSONEvent, abi.Event, time.Time) (*write.Point, error)

			switch eventABI.Name {
			case "ValidationQueued":
				processor = processValidationQueued
			case "ValidationWithdrawn":
				processor = processValidationWithdrawn
			case "ValidationSignaledExit":
				processor = processValidationSignaledExit
			case "StakeIncreased":
				processor = processStakeIncreased
			case "StakeDecreased":
				processor = processStakeDecreased
			case "DelegationAdded":
				processor = s.processDelegationAdded
			case "DelegationWithdrawn":
				processor = s.processDelegationWithdrawn
			case "DelegationSignaledExit":
				processor = s.processDelegationSignaledExit
			default:
				slog.Warn("unknown event type", "event", eventABI.Name)
				continue
			}
			point, err := processor(revision, log, eventABI, timestamp)
			if err != nil {
				slog.Error("failed to process event", "event", eventABI.Name, "error", err)
				continue
			}
			points = append(points, point)
		}

	}

	return points, nil
}

func processValidationQueued(_ thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	validator := thor.BytesToAddress(event.Topics[1][:]) // indexed
	endorsor := thor.BytesToAddress(event.Topics[2][:])  // indexed

	out, err := abi.Inputs.UnpackValues(hexutil.MustDecode(event.Data))
	if err != nil {
		slog.Error("failed to unpack event data", "event", abi.Name, "error", err)
		return nil, err
	}
	period, ok := out[0].(uint32)
	if !ok {
		slog.Error("failed to cast period", "event", abi.Name, "value", out[0])
		return nil, err
	}
	stake, ok := out[1].(*big.Int)
	if !ok {
		slog.Error("failed to cast stake", "event", abi.Name, "value", out[1])
		return nil, err
	}

	return write.NewPoint(
		"validation_queued",
		map[string]string{
			"validator": validator.String(),
			"endorsor":  endorsor.String(),
		},
		map[string]interface{}{
			"period": period,
			"stake":  stake.String(),
		},
		timestamp,
	), nil
}

func processValidationWithdrawn(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	validator := thor.BytesToAddress(event.Topics[1][:]) // indexed

	out, err := abi.Inputs.UnpackValues(hexutil.MustDecode(event.Data))
	if err != nil {
		slog.Error("failed to unpack event data", "event", abi.Name, "error", err)
		return nil, err
	}
	stake, ok := out[0].(*big.Int)
	if !ok {
		slog.Error("failed to cast stake", "event", abi.Name, "value", out[0])
		return nil, err
	}

	return write.NewPoint(
		"validation_withdrawn",
		map[string]string{
			"validator": validator.String(),
		},
		map[string]interface{}{
			"stake": vetutil.ScaleToVET(stake),
		},
		timestamp,
	), nil
}

func processValidationSignaledExit(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	validator := thor.BytesToAddress(event.Topics[1][:]) // indexed

	return write.NewPoint(
		"validation_signaled_exit",
		map[string]string{
			"validator": validator.String(),
		},
		map[string]interface{}{
			"null": true,
		},
		timestamp,
	), nil
}

func processStakeIncreased(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	validator := thor.BytesToAddress(event.Topics[1][:]) // indexed

	out, err := abi.Inputs.UnpackValues(hexutil.MustDecode(event.Data))
	if err != nil {
		slog.Error("failed to unpack event data", "event", abi.Name, "error", err)
		return nil, err
	}
	added, ok := out[0].(*big.Int)
	if !ok {
		slog.Error("failed to cast added stake", "event", abi.Name, "value", out[0])
		return nil, err
	}

	return write.NewPoint(
		"stake_increased",
		map[string]string{
			"validator": validator.String(),
		},
		map[string]interface{}{
			"added": added.String(),
		},
		timestamp,
	), nil
}

func processStakeDecreased(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	validator := thor.BytesToAddress(event.Topics[1][:]) // indexed

	out, err := abi.Inputs.UnpackValues(hexutil.MustDecode(event.Data))
	if err != nil {
		slog.Error("failed to unpack event data", "event", abi.Name, "error", err)
		return nil, err
	}
	removed, ok := out[0].(*big.Int)
	if !ok {
		slog.Error("failed to cast removed stake", "event", abi.Name, "value", out[0])
		return nil, err
	}

	return write.NewPoint(
		"stake_decreased",
		map[string]string{
			"validator": validator.String(),
		},
		map[string]interface{}{
			"removed": removed.String(),
		},
		timestamp,
	), nil
}

func (s *Staker) processDelegationAdded(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	validator := thor.BytesToAddress(event.Topics[1][:])      // indexed
	delegationID := new(big.Int).SetBytes(event.Topics[2][:]) // indexed
	delegation, err := s.staker.Revision(rev.String()).GetDelegation(delegationID)
	if err != nil {
		slog.Error("failed to get delegation", "delegation_id", delegationID, "error", err)
		return nil, err
	}

	out, err := abi.Inputs.UnpackValues(hexutil.MustDecode(event.Data))
	if err != nil {
		slog.Error("failed to unpack event data", "event", abi.Name, "error", err)
		return nil, err
	}
	stake, ok := out[0].(*big.Int)
	if !ok {
		slog.Error("failed to cast stake", "event", abi.Name, "value", out[0])
		return nil, err
	}
	multiplier, ok := out[1].(uint8)
	if !ok {
		slog.Error("failed to cast multiplier", "event", abi.Name, "value", out[1])
		return nil, err
	}
	weight := new(big.Int).SetUint64(uint64(multiplier))
	weight = weight.Mul(weight, stake)
	weight = weight.Div(weight, big.NewInt(100)) // assuming multiplier is in percentage

	return write.NewPoint(
		"delegation_added",
		map[string]string{
			"validator":     validator.String(),
			"delegation_id": delegationID.String(),
		},
		map[string]interface{}{
			"stake":        vetutil.ScaleToVET(stake),
			"multiplier":   multiplier,
			"weight":       vetutil.ScaleToVET(weight),
			"start_period": delegation.StartPeriod,
		},
		timestamp,
	), nil
}

func (s *Staker) processDelegationWithdrawn(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	// delegationID is indexed, so we can extract it from the event topics
	staker := s.staker.Revision(rev.String())
	delegationID := new(big.Int).SetBytes(event.Topics[1][:]) // indexed
	delegation, err := staker.GetDelegation(delegationID)
	if err != nil {
		slog.Error("failed to get delegation", "delegation_id", delegationID, "error", err)
		return nil, err
	}
	validation, err := staker.Get(delegation.Validator)
	if err != nil {
		slog.Error("failed to get validation", "validator", delegation.Validator, "error", err)
		return nil, err
	}
	validatorComplete, err := staker.GetCompletedPeriods(delegation.Validator)
	if err != nil {
		slog.Error("failed to get validation", "validator", delegation.Validator, "error", err)
		return nil, err
	}
	validationCurrent := *validatorComplete
	if validation.Status == builtin.StakerStatusActive {
		validationCurrent += 1
	}

	out, err := abi.Inputs.UnpackValues(hexutil.MustDecode(event.Data))
	if err != nil {
		slog.Error("failed to unpack event data", "event", abi.Name, "error", err)
		return nil, err
	}
	stake, ok := out[0].(*big.Int)
	if !ok {
		slog.Error("failed to cast stake", "event", abi.Name, "value", out[0])
		return nil, err
	}

	weight := new(big.Int).SetUint64(uint64(delegation.Multiplier))
	weight = weight.Mul(weight, stake)
	weight = weight.Div(weight, big.NewInt(100)) // assuming multiplier is in percentage

	return write.NewPoint(
		"delegation_withdrawn",
		map[string]string{
			"validator":     delegation.Validator.String(),
			"delegation_id": delegationID.String(),
		},
		map[string]interface{}{
			"stake":      vetutil.ScaleToVET(stake),
			"multiplier": delegation.Multiplier,
			"weight":     vetutil.ScaleToVET(weight),
			"started":    strconv.FormatBool(validationCurrent > delegation.StartPeriod),
			"block":      tx.NewBlockRefFromID(rev).Number(),
		},
		timestamp,
	), nil
}

func (s *Staker) processDelegationSignaledExit(rev thor.Bytes32, event *api.JSONEvent, abi abi.Event, timestamp time.Time) (*write.Point, error) {
	// delegationID is indexed, so we can extract it from the event topics
	delegationID := new(big.Int).SetBytes(event.Topics[1][:]) // indexed
	delegation, err := s.staker.Revision(rev.String()).GetDelegation(delegationID)
	if err != nil {
		slog.Error("failed to get delegation", "delegation_id", delegationID, "error", err)
		return nil, err
	}
	validation, err := s.staker.Revision(rev.String()).Get(delegation.Validator)
	if err != nil {
		slog.Error("failed to get validation", "validator", delegation.Validator, "error", err)
		return nil, err
	}

	weight := new(big.Int).SetUint64(uint64(delegation.Multiplier))
	weight = weight.Mul(weight, delegation.Stake)
	weight = weight.Div(weight, big.NewInt(100)) // assuming multiplier is in percentage

	return write.NewPoint(
		"delegation_signaled_exit",
		map[string]string{
			"validator":     delegation.Validator.String(),
			"delegation_id": delegationID.String(),
		},
		map[string]interface{}{
			"exit_period":              delegation.EndPeriod,
			"stake":                    vetutil.ScaleToVET(delegation.Stake),
			"multiplier":               delegation.Multiplier,
			"weight":                   vetutil.ScaleToVET(weight),
			"block":                    tx.NewBlockRefFromID(rev).Number(),
			"validator_start_block":    validation.StartBlock,
			"validator_staking_period": validation.Period,
		},
		timestamp,
	), nil
}
