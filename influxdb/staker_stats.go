package influxdb

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/accounts"
)

const (
	validatorQueuedEventName           = "ValidatorQueued"
	validatorUpdatedAutoRenewEventName = "ValidatorUpdatedAutoRenew"
)

type stakerStats struct {
	AddStaker  []addStakerEvent
	ExitStaker []exitStakerEvent
}

type addStakerEvent struct {
	Endorsor  thor.Address
	Master    thor.Address
	Period    uint32
	Stake     *big.Int
	AutoRenew bool
}

type exitStakerEvent struct {
	Endorsor  thor.Address
	Master    thor.Address
	AutoRenew bool
}

func NewStakerStats() *stakerStats {
	return &stakerStats{
		AddStaker:  make([]addStakerEvent, 0),
		ExitStaker: make([]exitStakerEvent, 0),
	}
}

func (s *stakerStats) processEvent(event *blocks.JSONEvent) error {
	if len(event.Topics) == 0 {
		return nil
	}

	parsedABI, err := abi.JSON(strings.NewReader(accounts.StakerAbi))
	if err != nil {
		return err
	}
	addValidatorTopic := parsedABI.Events[validatorQueuedEventName].Id()
	exitValidatorTopic := parsedABI.Events[validatorUpdatedAutoRenewEventName].Id()

	validatorAddedEvent := thor.MustParseBytes32(addValidatorTopic.Hex())
	validatorExitedEvent := thor.MustParseBytes32(exitValidatorTopic.Hex())

	if event.Topics[0] == validatorAddedEvent {
		collectValidatorAddedEvent(s, event)
	}

	if event.Topics[0] == validatorExitedEvent {
		collectValidatorExitedEvent(s, event)
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

func collectValidatorExitedEvent(s *stakerStats, event *blocks.JSONEvent) {
	endorsorAddress := thor.BytesToAddress(event.Topics[1].Bytes())
	masterAddress := thor.BytesToAddress(event.Topics[2].Bytes())

	data := event.Data[2:]
	hexData, _ := hex.DecodeString(data[:64])
	autoRenew := hexData[len(hexData)-1] != 0

	s.ExitStaker = append(s.ExitStaker, exitStakerEvent{
		Endorsor:  endorsorAddress,
		Master:    masterAddress,
		AutoRenew: autoRenew,
	})
}
