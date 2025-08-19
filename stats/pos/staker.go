package pos

import (
	"bytes"
	_ "embed"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"sync/atomic"

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
	"github.com/vechain/thorflux/config"
)

type Validation struct {
	*builtin.ValidatorStake
	*builtin.ValidatorStatus
	*builtin.ValidatorPeriodDetails
	*builtin.ValidationTotals
	DelegatorStake        *big.Int // Total stake of delegators for this validator
	DelegatorWeight       *big.Int // Total weight of delegators for this validator
	DelegatorQueuedStake  *big.Int // Total queued stake of delegators for this validator
	DelegatorQueuedWeight *big.Int // Total queued weight of delegators for this validator
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
	IssuanceVTHO    *big.Int // Total VTHO issued in the network
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
	epochLength := uint32(config.CheckpointInterval)
	key := thor.BytesToBytes32([]byte(config.EpochLengthStorageKey))
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
	cache, err := lru.New(config.DefaultCacheSize)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToCreateCache, err)
	}

	return &Staker{staker: staker, client: client, epochLength: epochLength, cache: cache}, nil
}

func createProposers(validators []*Validation) map[thor.Address]*validation.Validation {
	proposers := make(map[thor.Address]*validation.Validation)
	for _, v := range validators {
		if v.Status != builtin.StakerStatusActive {
			continue
		}
		// scheduler doesn't need any other fields
		proposers[v.ValidatorStatus.Address] = &validation.Validation{
			Online: v.Online,
			Weight: v.Weight,
		}
	}
	return proposers
}

type MissedSlot struct {
	Signer thor.Address
}

func (s *Staker) MissedSlots(
	validators []*Validation,
	block *api.JSONExpandedBlock,
	seed []byte,
) ([]MissedSlot, error) {
	proposers := createProposers(validators)
	parent, err := s.client.Block(block.ParentID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch parent block %s: %w", block.ParentID, err)
	}

	sched, err := pos.NewScheduler(block.Signer, proposers, parent.Number, parent.Timestamp, seed)
	if err != nil {
		return nil, err
	}
	missedSigners := make([]MissedSlot, 0)
	for i := parent.Timestamp + config.BlockIntervalSeconds; i < block.Timestamp; i += config.BlockIntervalSeconds {
		for master := range proposers {
			if sched.IsScheduled(i, master) {
				missedSigners = append(missedSigners, MissedSlot{
					Signer: master,
				})
			}
		}
	}

	// go through offline validators, forcing them online one by one
	for offlineProposer, value := range proposers {
		// we already went through online validators
		if value.Online {
			continue
		}
		// force validator to become online
		value.Online = true

		sched, err := pos.NewScheduler(block.Signer, proposers, parent.Number, parent.Timestamp, seed)
		if err != nil {
			return nil, err
		}

		// NOTE: We do not check for the skipped slots for offline validators
		if sched.IsScheduled(block.Timestamp, offlineProposer) &&
			block.Signer != offlineProposer {
			// if an offline validator could be scheduled for this block
			// but the signer is different
			missedSigners = append(missedSigners, MissedSlot{
				Signer: offlineProposer,
			})
		}

		// put validator back to offline
		value.Online = false
	}
	return missedSigners, nil
}

type FutureSlot struct {
	Block  uint32
	Signer thor.Address
}

func (s *Staker) FutureSlots(validators []*Validation, block *api.JSONExpandedBlock, seed []byte) ([]FutureSlot, error) {
	// max amount of blocks that we can predict
	// eg epoch length = 180, block number = 177, then we can predict, 178, 179. 180 is a new epoch
	blockInEpoch := block.Number % s.epochLength
	predictableSlots := s.epochLength - blockInEpoch - 1
	slots := make([]FutureSlot, 0)

	proposers := createProposers(validators)

	for i := range predictableSlots {
		parent := block.Number + i
		parentTimestamp := block.Timestamp + uint64(i)*config.BlockIntervalSeconds
		newTimestamp := parentTimestamp + config.BlockIntervalSeconds
		sched, err := pos.NewScheduler(block.Signer, proposers, parent, parentTimestamp, seed)
		if err != nil {
			return nil, fmt.Errorf("failed to create scheduler for block %d: %w", parent, err)
		}
		for master := range proposers {
			if sched.IsScheduled(newTimestamp, master) {
				slots = append(slots, FutureSlot{
					Block:  parent + 1,
					Signer: master,
				})
				break // we only need one signer per block
			}
		}
	}

	return slots, nil
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
		return nil, fmt.Errorf(config.ErrFailedToInitializeABI, err)
	}
	rawResult, err := s.fetchStakerInfo(id)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToFetchStakerInfo, err)
	}
	result, err := s.unpackInfo(rawResult)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToUnpackStakerInfo, err)
	}
	s.cache.Add(id, result)
	return result, nil
}

func (s *Staker) ValidatorMap(id thor.Bytes32) (map[thor.Address]*Validation, error) {
	info, err := s.FetchAll(id)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToFetchStakerInfoFromDB, err)
	}

	validators := make(map[thor.Address]*Validation, len(info.Validations))
	for _, v := range info.Validations {
		validators[(v.ValidatorStake.Address)] = v
	}
	return validators, nil
}

func (s *Staker) fetchStakerInfo(id thor.Bytes32) ([]*api.CallResult, error) {
	to := thor.MustParseAddress(config.GetValidatorsContractAddress)
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
		return nil, fmt.Errorf(config.ErrFailedToFetchStakerInfo, err)
	}
	expectedResultsLength := 7
	if len(res) != expectedResultsLength {
		// expect exactly expectedResultsLength results
		return nil, fmt.Errorf(config.ErrUnexpectedResults, len(res), expectedResultsLength)
	}

	for i, r := range res {
		if r.Reverted || r.VMError != "" {
			return nil, fmt.Errorf(config.ErrCallReverted, i, r.VMError)
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
		return nil, fmt.Errorf(config.ErrFailedToUnpackValidators, err)
	}

	// totalStakeABI returns 2 big.Ints, first is VET, second is weight
	totalStakeBytes, err := hexutil.Decode(totalStakeCall.Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeTotalStake, err)
	}
	totalStakeVET := new(big.Int).SetBytes(totalStakeBytes[:32])
	totalStakeWeight := new(big.Int).SetBytes(totalStakeBytes[32:64])

	// queuedStakeABI returns 2 big.Ints, first is VET, second is weight
	queuedStakeBytes, err := hexutil.Decode(queuedStakeCall.Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeQueuedStake, err)
	}
	queuedStakeVET := new(big.Int).SetBytes(queuedStakeBytes[:32])
	queuedStakeWeight := new(big.Int).SetBytes(queuedStakeBytes[32:64])

	// staker contract balance
	stakerBalanceBytes, err := hexutil.Decode(stakerBalanceCall.Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeStakerBalance, err)
	}
	// vtho total supply
	totalSupplyBytes, err := hexutil.Decode(result[5].Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeTotalSupply, err)
	}
	// total burned
	totalBurnedBytes, err := hexutil.Decode(result[6].Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeTotalBurned, err)
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
		return nil, fmt.Errorf(config.ErrFailedToDecodeResultData, err)
	}
	out, err := getValidatorsABI.Outputs.UnpackValues(bytes)
	if err != nil {
		return nil, err
	}

	validators := make([]*Validation, 0)
	masters := out[0].([]common.Address)
	endorsors := out[1].([]common.Address)
	statuses := out[2].([]uint8)
	onlines := out[3].([]bool)
	stakingPeriodLengths := out[4].([]uint32)
	startBlocks := out[5].([]uint32)
	exitBlocks := out[6].([]uint32)
	completedPeriods := out[7].([]uint32)
	validatorLockedVETs := out[8].([]*big.Int)
	validatorLockedWeights := out[9].([]*big.Int)
	delegatorsStakes := out[10].([]*big.Int)
	validatorQueuedStakes := out[11].([]*big.Int)
	totalQueuedStakes := out[12].([]*big.Int)
	totalQueuedWeights := out[13].([]*big.Int)
	exitingStakes := out[14].([]*big.Int)
	exitingWeights := out[15].([]*big.Int)

	for i := range masters {
		vStake := &builtin.ValidatorStake{
			Address:     (thor.Address)(masters[i]),
			Endorsor:    (thor.Address)(endorsors[i]),
			Stake:       validatorLockedVETs[i],
			Weight:      validatorLockedWeights[i],
			QueuedStake: validatorQueuedStakes[i],
		}
		vStatus := &builtin.ValidatorStatus{
			Address: (thor.Address)(masters[i]),
			Status:  builtin.StakerStatus(statuses[i]),
			Online:  onlines[i],
		}
		vPeriodDetails := &builtin.ValidatorPeriodDetails{
			Address:          (thor.Address)(masters[i]),
			Period:           stakingPeriodLengths[i],
			StartBlock:       startBlocks[i],
			ExitBlock:        exitBlocks[i],
			CompletedPeriods: completedPeriods[i],
		}
		totals := &builtin.ValidationTotals{
			TotalLockedStake:   new(big.Int).Add(vStake.Stake, delegatorsStakes[i]),
			TotalLockedWeight:  validatorLockedWeights[i],
			TotalQueuedStake:   totalQueuedStakes[i],
			TotalQueuedWeight:  totalQueuedWeights[i],
			TotalExitingStake:  exitingStakes[i],
			TotalExitingWeight: exitingWeights[i],
		}

		validators = append(validators, &Validation{
			ValidatorStake:         vStake,
			ValidatorStatus:        vStatus,
			ValidatorPeriodDetails: vPeriodDetails,
			ValidationTotals:       totals,
			DelegatorStake:         delegatorsStakes[i],
			DelegatorWeight:        new(big.Int).Sub(validatorLockedWeights[i], big.NewInt(0).Mul(validatorLockedVETs[i], big.NewInt(2))),
			DelegatorQueuedStake:   new(big.Int).Sub(totalQueuedStakes[i], validatorQueuedStakes[i]),
			DelegatorQueuedWeight:  new(big.Int).Sub(totalQueuedWeights[i], big.NewInt(0).Mul(validatorQueuedStakes[i], big.NewInt(2))),
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

	to := thor.MustParseAddress(config.GetValidatorsContractAddress)
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
		return fmt.Errorf(config.ErrFailedToFetchPreviousTotals, err)
	}

	if len(res) != 3 {
		slog.Error("Unexpected number of results", "count", len(res))
		return fmt.Errorf(config.ErrUnexpectedResults, len(res), 3)
	}

	totalSupplyBytes, err := hexutil.Decode(res[1].Data)
	if err != nil {
		slog.Error("Failed to decode total supply data", "error", err)
		return fmt.Errorf(config.ErrFailedToDecodePreviousTotalSupply, err)
	}
	totalBurnedBytes, err := hexutil.Decode(res[2].Data)
	if err != nil {
		slog.Error("Failed to decode total burned data", "error", err)
		return fmt.Errorf(config.ErrFailedToDecodePreviousTotalBurned, err)
	}

	s.prevVTHOSupply.Store(new(big.Int).SetBytes(totalSupplyBytes))
	s.prevVTHOBurned.Store(new(big.Int).SetBytes(totalBurnedBytes))
	return nil
}
