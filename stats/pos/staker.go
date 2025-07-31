package pos

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	lru "github.com/hashicorp/golang-lru"
	"log/slog"
	"math/big"
	"sync"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
)

type Staker struct {
	staker      *builtin.Staker
	client      *thorclient.Client
	epochLength uint32
	cache       *lru.Cache
	mu          sync.Mutex // Protects the cache
}

func NewStaker(client *thorclient.Client) (*Staker, error) {
	staker, err := builtin.NewStaker(client)
	if err != nil {
		return nil, err
	}
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
	cache, err := lru.New(100) // Cache size of 1000 entries
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	return &Staker{staker: staker, client: client, epochLength: epochLength, cache: cache}, nil
}

func (s *Staker) NextValidator(
	validators map[thor.Address]*builtin.Validator,
	block *api.JSONExpandedBlock,
	seed []byte,
) (*thor.Address, error) {
	proposers := make(map[thor.Address]*validation.Validation)
	for id, v := range validators {
		// scheduler doesn't need any other fields
		proposers[id] = &validation.Validation{
			Online: v.Online,
			Weight: v.Weight,
		}
	}

	sched, err := pos.NewScheduler(block.Signer, proposers, block.Number, block.Timestamp, seed)
	if err != nil {
		return nil, err
	}
	for id := range validators {
		if sched.IsScheduled(block.Timestamp+10, id) {
			return &id, nil
		}
	}
	slog.Warn("No expected validator found for current block", "block", block.ID, "seed", fmt.Sprintf("%x", seed))
	return nil, fmt.Errorf("no expected validator found for current block %s", block.ID)
}

//go:embed compiled/GetValidators.abi
var contractABI string

//go:embed compiled/GetValidators.bin
var bytecode string

// FetchAll retrieves all queued and active validators from the staker contract.
// Using a hacky getAll in a simulation. It means the method takes 35ms
// It takes 500ms if we iterate each validator on the client side
// The validators are ordered by their position in the active and queued groups. Ie FIFO.
// See `GetValidators.sol` for more details.
func (s *Staker) FetchAll(id thor.Bytes32) ([]*builtin.Validator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.cache.Get(id)
	if ok {
		return existing.([]*builtin.Validator), nil
	}
	abi, err := ethabi.JSON(bytes.NewReader([]byte(contractABI)))
	if err != nil {
		return nil, err
	}
	method, ok := abi.Methods["getAll"]
	if !ok {
		return nil, errors.New("method not found")
	}
	to := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")
	res, err := s.staker.Raw().Client().InspectClauses(&api.BatchCallData{
		Clauses: api.Clauses{
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

	validators := make([]*builtin.Validator, 0)
	masters := out[0].([]common.Address)
	endorsors := out[1].([]common.Address)
	stakes := out[2].([]*big.Int)
	weights := out[3].([]*big.Int)
	statuses := out[4].([]uint8)
	onlines := out[5].([]bool)
	stakingPeriods := out[6].([]uint32)
	startBlocks := out[7].([]uint32)
	exitBlocks := out[8].([]uint32)
	for i := range masters {
		v := &builtin.Validator{
			Address:    (thor.Address)(masters[i]),
			Endorsor:   (thor.Address)(endorsors[i]),
			Stake:      stakes[i],
			Weight:     weights[i],
			Status:     builtin.StakerStatus(statuses[i]),
			Online:     onlines[i],
			Period:     stakingPeriods[i],
			StartBlock: startBlocks[i],
			ExitBlock:  exitBlocks[i],
		}
		validators = append(validators, v)
	}

	s.cache.Add(id, validators)

	return validators, nil
}
