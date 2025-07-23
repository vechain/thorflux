package pos

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/v2/api/blocks"
	builtin2 "github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/builtin"
)

const (
	validatorQueuedEventName = "ValidatorQueued"
)

type stakerStats struct {
	AddStaker     []addStakerEvent
	StakersStatus []stakerStatus
}

type addStakerEvent struct {
	Endorsor  thor.Address
	Master    thor.Address
	Period    uint32
	Stake     *big.Int
	AutoRenew bool
}

type stakerStatus struct {
	Endorsor  *thor.Address
	Master    *thor.Address
	Status    builtin.StakerStatus
	AutoRenew bool
	Stake     *big.Int
}

func NewStakerStats() *stakerStats {
	return &stakerStats{
		AddStaker:     make([]addStakerEvent, 0),
		StakersStatus: make([]stakerStatus, 0),
	}
}

func (s *stakerStats) processEvent(event *blocks.JSONEvent) error {
	if len(event.Topics) == 0 {
		return nil
	}

	parsedABI, err := abi.JSON(bytes.NewBuffer(builtin2.Staker.RawABI()))
	if err != nil {
		return err
	}

	addValidatorTopic := parsedABI.Events[validatorQueuedEventName].Id()
	validatorAddedEvent := thor.MustParseBytes32(addValidatorTopic.Hex())

	if event.Topics[0] == validatorAddedEvent {
		collectValidatorAddedEvent(s, event)
	}
	return nil
}

func collectValidatorAddedEvent(s *stakerStats, event *blocks.JSONEvent) {
	endorsorAddress := thor.BytesToAddress(event.Topics[1].Bytes())
	masterAddress := thor.BytesToAddress(event.Topics[2].Bytes())

	data := event.Data[2:]
	hexData, _ := hex.DecodeString(data[:64])
	period := binary.BigEndian.Uint32(hexData[28:32])

	hexData, _ = hex.DecodeString(data[64:128])
	stake := new(big.Int).SetBytes(hexData)

	hexData, _ = hex.DecodeString(data[128:192])
	autoRenew := hexData[len(hexData)-1] != 0

	s.AddStaker = append(s.AddStaker, addStakerEvent{
		Endorsor:  endorsorAddress,
		Master:    masterAddress,
		Period:    period,
		Stake:     stake,
		AutoRenew: autoRenew,
	})
}

func (s *stakerStats) CollectActiveStakers(staker *Staker, block, parent *blocks.JSONExpandedBlock, active bool) error {
	if !active {
		s.StakersStatus = make([]stakerStatus, 0)
		return nil
	}
	candidates, err := staker.GetValidators(block, parent)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		s.StakersStatus = append(s.StakersStatus, stakerStatus{
			Endorsor:  candidate.Endorsor,
			Master:    candidate.Master,
			Status:    candidate.Status,
			Stake:     candidate.Stake,
			AutoRenew: candidate.AutoRenew,
		})
	}
	return nil
}
