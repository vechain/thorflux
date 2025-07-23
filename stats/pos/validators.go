package pos

import (
	"bytes"
	_ "embed"
	"errors"
	"log/slog"
	"math/big"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thor/v2/tx"
)

type validators struct {
	entries        map[thor.Bytes32]*builtin.Validator
	mu             sync.Mutex
	previousUpdate thor.Bytes32
	staker         *builtin.Staker
	epochLength    uint32
}

func newValidatorCache(staker *builtin.Staker, client *thorclient.Client) (*validators, error) {
	epochLength := uint32(thor.CheckpointInterval)
	key := thor.BytesToBytes32([]byte("epoch-length"))
	storage, err := client.AccountStorage(staker.Raw().Address(), &key)
	if err != nil {
		return nil, err
	}
	bytes32, err := thor.ParseBytes32(storage.Value)
	if err != nil {
		return nil, err
	}
	if !bytes32.IsZero() {
		big := new(big.Int).SetBytes(bytes32.Bytes())
		epochLength = uint32(big.Uint64())
	}

	return &validators{
		staker:         staker,
		entries:        make(map[thor.Bytes32]*builtin.Validator),
		previousUpdate: thor.Bytes32{},
		epochLength:    epochLength,
	}, nil
}

// Get retrieves the active validators for the given block.
// It checks if the validators are already cached and if not, fetches them from the staker contract.
// It the new block is a new epoch, it fetches the refreshed list of validators.
func (v *validators) Get(block *blocks.JSONExpandedBlock, forceUpdate bool) (map[thor.Bytes32]*builtin.Validator, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	prevBlock := tx.NewBlockRefFromID(v.previousUpdate).Number()
	if prevBlock > block.Number {
		return v.Fetch(block.ID)
	}

	notInitialised := len(v.entries) == 0 || v.previousUpdate.IsZero()
	newEpoch := prevBlock/v.epochLength != block.Number/v.epochLength

	if forceUpdate || notInitialised || newEpoch {
		entries, err := v.Fetch(block.ID)
		if err != nil {
			return nil, err
		}
		v.entries = entries
		v.previousUpdate = block.ID
		return v.entries, nil
	}

	return v.entries, nil
}

//go:embed compiled/GetValidators.abi
var contractABI string

//go:embed compiled/GetValidators.bin
var bytecode string

// Fetch retrieves all active validators from the staker contract.
// Using a hacky getAll in a simulation. It means the method takes 35ms
// It takes 500ms if we iterate each validator on the client side
func (v *validators) Fetch(id thor.Bytes32) (map[thor.Bytes32]*builtin.Validator, error) {
	slog.Info("fetching validators", "block", tx.NewBlockRefFromID(id).Number())
	abi, err := ethabi.JSON(bytes.NewReader([]byte(contractABI)))
	if err != nil {
		return nil, err
	}
	method, ok := abi.Methods["getAll"]
	if !ok {
		return nil, errors.New("method not found")
	}
	to := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")
	res, err := v.staker.Raw().Client().InspectClauses(&accounts.BatchCallData{
		Clauses: accounts.Clauses{
			{
				Data: "0x" + bytecode,
			},
			{
				To:   &to,
				Data: hexutil.Encode(method.Id()),
			},
		},
	}, thorclient.Revision(id.String()))
	if err != nil {
		return nil, err
	}

	bytes, err := hexutil.Decode(res[1].Data)
	if err != nil {
		return nil, err
	}
	out, err := method.Outputs.UnpackValues(bytes)
	if err != nil {
		return nil, err
	}

	validators := make(map[thor.Bytes32]*builtin.Validator)
	ids := out[0].([][32]uint8)
	masters := out[1].([]common.Address)
	endorsors := out[2].([]common.Address)
	stakes := out[3].([]*big.Int)
	weights := out[4].([]*big.Int)
	statuses := out[5].([]uint8)
	autoRenews := out[6].([]bool)
	onlines := out[7].([]bool)
	stakingPeriods := out[8].([]uint32)
	startBlocks := out[9].([]uint32)
	exitBlocks := out[10].([]uint32)
	for i, id := range ids {
		v := &builtin.Validator{
			Master:     (*thor.Address)(&masters[i]),
			Endorsor:   (*thor.Address)(&endorsors[i]),
			Stake:      stakes[i],
			Weight:     weights[i],
			Status:     builtin.StakerStatus(statuses[i]),
			AutoRenew:  autoRenews[i],
			Online:     onlines[i],
			Period:     stakingPeriods[i],
			StartBlock: startBlocks[i],
			ExitBlock:  exitBlocks[i],
		}
		id := thor.Bytes32(id)

		validators[id] = v
	}

	return validators, nil
}
