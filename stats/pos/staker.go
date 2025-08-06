package pos

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync/atomic"

	"log/slog"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	lru "github.com/hashicorp/golang-lru"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
)

type Validation struct {
	*builtin.Validator
	CompletedPeriods uint32   // Number of completed staking periods
	TotalStaked      *big.Int // Total staked amount in the validator
	DelegatorsStaked *big.Int // Total staked amount by delegators
	DelegatorsWeight *big.Int // Total weight of delegators
}

type StakerInformation struct {
	Validations     []*Validation
	ContractBalance *big.Int // Balance of the staker contract
	QueuedVET       *big.Int // Total VET queued for staking
	QueuedWeight    *big.Int // Total weight of queued validators
	TotalVET        *big.Int // Total VET staked in the network
	TotalWeight     *big.Int // Total weight of all validators
	TotalSupplyVTHO *big.Int // Total supply of VTHO in the network
	TotalBurnedVTHO *big.Int // Total VTHO burned in the network
}

type Staker struct {
	staker      *builtin.Staker
	client      *thorclient.Client
	epochLength uint32
	cache       *lru.Cache
	mu          sync.Mutex // Protects the cache

	prevVTHOSupply atomic.Pointer[big.Int]
	prevVTHOBurned atomic.Pointer[big.Int] // Previous VTHO burned for calculating changes
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

type MissedSlot struct {
	Slot   uint64
	Signer thor.Address
}

func (s *Staker) MissedSlots(
	validators map[thor.Address]*builtin.Validator,
	block *api.JSONExpandedBlock,
	seed []byte,
) ([]MissedSlot, error) {
	proposers := make(map[thor.Address]*validation.Validation)
	for id, v := range validators {
		if v.Status != builtin.StakerStatusActive {
			continue
		}
		// scheduler doesn't need any other fields
		proposers[id] = &validation.Validation{
			Online: v.Online,
			Weight: v.Weight,
		}
	}
	parent, err := s.client.Block(block.ParentID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent block %s: %w", block.ParentID, err)
	}
	sched, err := pos.NewScheduler(block.Signer, proposers, parent.Number, parent.Timestamp, seed)
	if err != nil {
		return nil, err
	}
	missedSigners := make([]MissedSlot, 0)
	for i := parent.Timestamp + thor.BlockInterval; i < block.Timestamp; i += thor.BlockInterval {
		for master := range proposers {
			if sched.IsScheduled(i, master) {
				missedSigners = append(missedSigners, MissedSlot{
					Slot:   i,
					Signer: master,
				})
			}
		}
	}
	return missedSigners, nil
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
func (s *Staker) FetchAll(id thor.Bytes32) (*StakerInformation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.cache.Get(id)
	if ok {
		return existing.(*StakerInformation), nil
	}
	if err := s.initABI(); err != nil {
		return nil, fmt.Errorf("failed to initialize helper ABI: %w", err)
	}
	rawResult, err := s.fetchStakerInfo(id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch staker info: %w", err)
	}
	result, err := s.unpackInfo(rawResult)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack staker info: %w", err)
	}
	s.cache.Add(id, result)
	return result, nil
}

func (s *Staker) ValidatorMap(id thor.Bytes32) (map[thor.Address]*builtin.Validator, error) {
	info, err := s.FetchAll(id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch staker info: %w", err)
	}

	validators := make(map[thor.Address]*builtin.Validator, len(info.Validations))
	for _, v := range info.Validations {
		validators[v.Address] = v.Validator
	}
	return validators, nil
}

func (s *Staker) fetchStakerInfo(id thor.Bytes32) ([]*api.CallResult, error) {
	to := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")
	res, err := s.staker.Raw().Client().InspectClauses(&api.BatchCallData{
		Clauses: api.Clauses{
			{
				Data: "0x" + bytecode,
			},
			{
				To:   &to,
				Data: hexutil.Encode(getValidatorsABI.Id()),
			},
			{
				To:   &to,
				Data: hexutil.Encode(stakerBalanceABI.Id()),
			},
			{
				To:   &to,
				Data: hexutil.Encode(totalStakeABI.Id()),
			},
			{
				To:   &to,
				Data: hexutil.Encode(queuedStakeABI.Id()),
			},
			{
				To:   &to,
				Data: hexutil.Encode(totalSupplyABI.Id()),
			},
			{
				To:   &to,
				Data: hexutil.Encode(totalBurnedABI.Id()),
			},
		},
	}, thorclient.Revision(id.String()))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch staker info: %w", err)
	}
	if len(res) != 7 {
		// expect exactly 7 results
		return nil, fmt.Errorf("unexpected number of results: %d, expected at least 5", len(res))
	}

	for i, r := range res {
		if r.Reverted || r.VMError != "" {
			return nil, fmt.Errorf("call %d reverted or had VM error: %s", i, r.VMError)
		}
	}
	return res, nil
}

func (s *Staker) unpackInfo(result []*api.CallResult) (*StakerInformation, error) {
	validatorsCall := result[1]
	stakerBalanceCall := result[2]
	totalStakeCall := result[3]
	queuedStakeCall := result[4]

	validators, err := s.unpackValidators(validatorsCall)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack validators: %w", err)
	}

	// totalStakeABI returns 2 big.Ints, first is VET, second is weight
	totalStakeBytes, err := hexutil.Decode(totalStakeCall.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode total stake data: %w", err)
	}
	totalStakeVET := new(big.Int).SetBytes(totalStakeBytes[:32])
	totalStakeWeight := new(big.Int).SetBytes(totalStakeBytes[32:64])

	// queuedStakeABI returns 2 big.Ints, first is VET, second is weight
	queuedStakeBytes, err := hexutil.Decode(queuedStakeCall.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode queued stake data: %w", err)
	}
	queuedStakeVET := new(big.Int).SetBytes(queuedStakeBytes[:32])
	queuedStakeWeight := new(big.Int).SetBytes(queuedStakeBytes[32:64])

	// staker contract balance
	stakerBalanceBytes, err := hexutil.Decode(stakerBalanceCall.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode staker balance data: %w", err)
	}
	// vtho total supply
	totalSupplyBytes, err := hexutil.Decode(result[5].Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode total supply data: %w", err)
	}
	// total burned
	totalBurnedBytes, err := hexutil.Decode(result[6].Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode total burned data: %w", err)
	}

	return &StakerInformation{
		Validations:     validators,
		ContractBalance: new(big.Int).SetBytes(stakerBalanceBytes),
		QueuedVET:       queuedStakeVET,
		QueuedWeight:    queuedStakeWeight,
		TotalVET:        totalStakeVET,
		TotalWeight:     totalStakeWeight,
		TotalSupplyVTHO: new(big.Int).SetBytes(totalSupplyBytes),
		TotalBurnedVTHO: new(big.Int).SetBytes(totalBurnedBytes),
	}, nil
}

func (s *Staker) unpackValidators(result *api.CallResult) ([]*Validation, error) {
	bytes, err := hexutil.Decode(result.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode result data: %w", err)
	}
	out, err := getValidatorsABI.Outputs.UnpackValues(bytes)
	if err != nil {
		return nil, err
	}

	validators := make([]*Validation, 0)
	masters := out[0].([]common.Address)
	endorsors := out[1].([]common.Address)
	stakes := out[2].([]*big.Int)
	weights := out[3].([]*big.Int)
	statuses := out[4].([]uint8)
	onlines := out[5].([]bool)
	stakingPeriods := out[6].([]uint32)
	startBlocks := out[7].([]uint32)
	exitBlocks := out[8].([]uint32)
	completedPeriods := out[9].([]uint32)
	delegatorsStaked := out[10].([]*big.Int)
	delegatorsWeight := out[11].([]*big.Int)
	totalStaked := out[2].([]*big.Int)

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

		validators = append(validators, &Validation{
			Validator:        v,
			CompletedPeriods: completedPeriods[i],
			TotalStaked:      new(big.Int).Set(totalStaked[i]),
			DelegatorsStaked: new(big.Int).Set(delegatorsStaked[i]),
			DelegatorsWeight: new(big.Int).Set(delegatorsWeight[i]),
		})
	}

	return validators, nil
}

var stakerBalanceABI abi.Method
var getValidatorsABI abi.Method
var totalStakeABI abi.Method
var queuedStakeABI abi.Method
var totalSupplyABI abi.Method
var totalBurnedABI abi.Method

var once sync.Once

func (s *Staker) initABI() error {
	var err error
	var ok bool
	once.Do(func() {
		var helperABI abi.ABI
		helperABI, err = abi.JSON(bytes.NewReader([]byte(contractABI)))
		if err != nil {
			slog.Error("Failed to parse staker contract ABI", "error", err)
			return
		}
		stakerBalanceABI, ok = helperABI.Methods["stakerBalance"]
		if !ok {
			err = fmt.Errorf("stakerBalance method not found in staker contract ABI")
			slog.Error("Failed to find stakerBalance method", "error", err)
			return
		}
		getValidatorsABI, ok = helperABI.Methods["getValidators"]
		if !ok {
			err = fmt.Errorf("getValidatorsABI method not found in staker contract ABI")
			slog.Error("Failed to find getValidatorsABI method", "error", err)
			return
		}
		totalStakeABI, ok = helperABI.Methods["totalStake"]
		if !ok {
			err = fmt.Errorf("totalStakeABI method not found in staker contract ABI")
			slog.Error("Failed to find totalStakeABI method", "error", err)
			return
		}
		queuedStakeABI, ok = helperABI.Methods["queuedStake"]
		if !ok {
			err = fmt.Errorf("queuedStakeABI method not found in staker contract ABI")
			slog.Error("Failed to find queuedStakeABI method", "error", err)
			return
		}
		totalSupplyABI, ok = helperABI.Methods["totalSupply"]
		if !ok {
			err = fmt.Errorf("totalSupply method not found in staker contract ABI")
			slog.Error("Failed to find totalSupply method", "error", err)
			return
		}
		totalBurnedABI, ok = helperABI.Methods["totalBurned"]
		if !ok {
			err = fmt.Errorf("totalBurned method not found in staker contract ABI")
			slog.Error("Failed to find totalBurned method", "error", err)
			return
		}
	})
	return err
}

func (s *Staker) setPrevTotals(id thor.Bytes32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	to := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")
	res, err := s.staker.Raw().Client().InspectClauses(&api.BatchCallData{
		Clauses: api.Clauses{
			{
				Data: "0x" + bytecode,
			},
			{
				To:   &to,
				Data: hexutil.Encode(totalSupplyABI.Id()),
			},
			{
				To:   &to,
				Data: hexutil.Encode(totalBurnedABI.Id()),
			},
		},
	}, thorclient.Revision(id.String()))

	if err != nil {
		slog.Error("Failed to fetch previous totals", "error", err)
		return fmt.Errorf("failed to fetch previous totals: %w", err)
	}

	if len(res) != 3 {
		slog.Error("Unexpected number of results", "count", len(res))
		return fmt.Errorf("unexpected number of results: %d, expected 3", len(res))
	}

	totalSupplyBytes, err := hexutil.Decode(res[1].Data)
	if err != nil {
		slog.Error("Failed to decode total supply data", "error", err)
		return fmt.Errorf("failed to decode total supply data: %w", err)
	}
	totalBurnedBytes, err := hexutil.Decode(res[2].Data)
	if err != nil {
		slog.Error("Failed to decode total burned data", "error", err)
		return fmt.Errorf("failed to decode total burned data: %w", err)
	}

	s.prevVTHOSupply.Store(new(big.Int).SetBytes(totalSupplyBytes))
	s.prevVTHOBurned.Store(new(big.Int).SetBytes(totalBurnedBytes))
	return nil
}
