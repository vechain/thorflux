package staker

import (
	"bytes"
	_ "embed"
	"errors"
	"github.com/ethereum/go-ethereum/accounts/abi"
)

//go:embed staker_abi.json
var ABI []byte

func GetEvent(name string) (*abi.Event, error) {
	reader := bytes.NewReader(ABI)
	abi, err := abi.JSON(reader)
	if err != nil {
		return nil, err
	}
	event, ok := abi.Events[name]
	if !ok {
		return nil, errors.New("event not found")
	}
	return &event, nil
}
