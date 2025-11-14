package pos

import (
	"bytes"
	_ "embed"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/pos"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
	"github.com/vechain/thorflux/vetutil"
)

type Staker struct {
	staker      *builtin.Staker
	client      *thorclient.Client
	epochLength uint32
}

func NewStaker(client *thorclient.Client) *Staker {
	staker, _ := builtin.NewStaker(client)
	// imported function, ok to swallow errors

	return &Staker{
		staker:      staker,
		client:      client,
		epochLength: thor.EpochLength(),
	}
}

func createProposers(validators []*types.Validation) []pos.Proposer {
	proposers := make([]pos.Proposer, 0)
	for _, v := range validators {
		if v.Status == validation.StatusActive {
			proposers = append(proposers, pos.Proposer{
				Address: v.Address,
				Active:  v.Online,
				Weight:  v.Weight,
			})
		}
	}
	return proposers
}

type MissedSlot struct {
	Signer thor.Address
}

func (s *Staker) MissedSlots(
	parent *api.JSONExpandedBlock,
	validators []*types.Validation,
	block *api.JSONExpandedBlock,
	seed []byte,
) ([]MissedSlot, []MissedSlot, error) {
	proposers := createProposers(validators)
	sched, err := pos.NewScheduler(block.Signer, proposers, parent.Number, parent.Timestamp, seed)
	if err != nil {
		return nil, nil, err
	}
	missedOnlineSigners := make([]MissedSlot, 0)
	for i := parent.Timestamp + config.BlockIntervalSeconds; i < block.Timestamp; i += config.BlockIntervalSeconds {
		for _, master := range proposers {
			if sched.IsScheduled(i, master.Address) {
				missedOnlineSigners = append(missedOnlineSigners, MissedSlot{
					Signer: master.Address,
				})
			}
		}
	}

	// go through offline validators, forcing them online one by one
	missedOfflineSigners := make([]MissedSlot, 0)
	for _, val := range validators {
		if val.Status != validation.StatusActive {
			continue
		}
		// we already went through online validators
		if val.OfflineBlock != nil {
			continue
		}

		sched, err := pos.NewScheduler(val.Address, proposers, parent.Number, parent.Timestamp, seed)
		if err != nil {
			return nil, nil, err
		}

		// NOTE: We do not check for the skipped slots for offline validators
		if sched.IsScheduled(block.Timestamp, val.Address) &&
			block.Signer != val.Address {
			// if an offline validator could be scheduled for this block
			// but the signer is different
			missedOfflineSigners = append(missedOfflineSigners, MissedSlot{
				Signer: val.Address,
			})
		}

	}
	return missedOnlineSigners, missedOfflineSigners, nil
}

type FutureSlot struct {
	Block  uint32
	Signer thor.Address
}

func (s *Staker) FutureSlots(validators []*types.Validation, block *api.JSONExpandedBlock, seed []byte) ([]FutureSlot, error) {
	// max amount of blocks that we can predict
	// eg epoch length = 180, block number = 177, then we can predict, 178, 179. 180 is a new epoch
	blockInEpoch := block.Number % s.epochLength
	predictableSlots := s.epochLength - blockInEpoch - 1
	slots := make([]FutureSlot, 0)

	proposers := createProposers(validators)

	// Check each future timestamp to find who is scheduled
	for i := range predictableSlots {
		parent := block.Number + i
		parentTimestamp := block.Timestamp + config.BlockIntervalSeconds*uint64(i)
		futureBlockNumber := parent + 1
		futureTimestamp := parentTimestamp + config.BlockIntervalSeconds

		sched, err := pos.NewScheduler(block.Signer, proposers, parent, parentTimestamp, seed)
		if err != nil {
			return nil, fmt.Errorf("failed to create scheduler for block %d: %w", block.Number, err)
		}

		// Check all validators (not just proposers) to find who is scheduled
		found := false
		for _, v := range validators {
			if v.Status == validation.StatusActive {
				if sched.IsScheduled(futureTimestamp, v.Address) {
					slots = append(slots, FutureSlot{
						Block:  futureBlockNumber,
						Signer: v.Address,
					})
					found = true
					break // we only need one signer per block
				}
			}
		}
		if !found {
			// If no validator found for this slot, we can't predict further
			break
		}
	}

	return slots, nil
}

//go:embed compiled/GetValidators.abi
var contractABI string

//go:embed compiled/GetValidators.bin
var bytecode string

// FetchValidations retrieves all queued and active validators from the staker contract.
// Using a hacky getAll in a simulation. It means the method takes 35ms
// It takes 500ms if we iterate each validator on the client side
// The validators are ordered by their position in the active and queued groups. Ie FIFO.
// See `GetValidators.sol` for more details.
func FetchValidations(id thor.Bytes32, client *thorclient.Client) (*types.StakerInformation, error) {
	rawResult, err := fetchStakerInfo(id, client)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToFetchStakerInfo, err)
	}
	result, err := unpackInfo(rawResult)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToUnpackStakerInfo, err)
	}
	return result, nil
}

func fetchStakerInfo(id thor.Bytes32, client *thorclient.Client) ([]*api.CallResult, error) {
	to := thor.MustParseAddress(config.GetValidatorsContractAddress)

	withdrawCounterPosition := thor.BytesToBytes32([]byte("withdrawable-stake"))
	cooldownCounterPosition := thor.BytesToBytes32([]byte("cooldown-stake"))

	withdrawableCallData, err := stakerStorageABI.Inputs.Pack(withdrawCounterPosition)
	if err != nil {
		return nil, fmt.Errorf("failed to pack withdrawable stake call data: %w", err)
	}
	stakerStorageCallData, err := stakerStorageABI.Inputs.Pack(cooldownCounterPosition)
	if err != nil {
		return nil, fmt.Errorf("failed to pack cooldown stake call data: %w", err)
	}
	withdrawableCallData = append(stakerStorageABI.Id(), withdrawableCallData...)
	stakerStorageCallData = append(stakerStorageABI.Id(), stakerStorageCallData...)

	clauses := api.Clauses{
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
		{
			To:   &to,
			Data: hexutil.Encode(withdrawableCallData),
		},
		{
			To:   &to,
			Data: hexutil.Encode(stakerStorageCallData),
		},
	}

	res, err := client.InspectClauses(&api.BatchCallData{
		Clauses: clauses,
	}, thorclient.Revision(id.String()))
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToFetchStakerInfo, err)
	}
	if len(res) != len(clauses) {
		// expect exactly expectedResultsLength results
		return nil, fmt.Errorf(config.ErrUnexpectedResults, len(res), len(clauses))
	}

	for i, r := range res {
		if r.Reverted || r.VMError != "" {
			return nil, fmt.Errorf(config.ErrCallReverted, i, r.VMError)
		}
	}
	return res, nil
}

func unpackInfo(result []*api.CallResult) (*types.StakerInformation, error) {
	validatorsCall := result[1]
	stakerBalanceCall := result[2]
	totalStakeCall := result[3]
	queuedStakeCall := result[4]

	validators, err := unpackValidators(validatorsCall)
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
	// withdrawable stake
	withdrawableBytes, err := hexutil.Decode(result[7].Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeWithdrawableStake, err)
	}
	// cooldown stake
	cooldownBytes, err := hexutil.Decode(result[8].Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeCooldownStake, err)
	}

	return &types.StakerInformation{
		Validations:     validators,
		ContractBalance: new(big.Int).SetBytes(stakerBalanceBytes),
		QueuedVET:       queuedStakeVET,
		TotalVET:        totalStakeVET,
		TotalWeight:     totalStakeWeight,
		VTHO: types.VTHO{
			TotalSupply: new(big.Int).SetBytes(totalSupplyBytes),
			TotalBurned: new(big.Int).SetBytes(totalBurnedBytes),
		},
		CooldownVET:     new(big.Int).SetBytes(cooldownBytes).Uint64(),
		WithdrawableVET: new(big.Int).SetBytes(withdrawableBytes).Uint64(),
	}, nil
}

func unpackValidators(result *api.CallResult) ([]*types.Validation, error) {
	bytes, err := hexutil.Decode(result.Data)
	if err != nil {
		return nil, fmt.Errorf(config.ErrFailedToDecodeResultData, err)
	}
	out, err := getValidatorsABI.Outputs.UnpackValues(bytes)
	if err != nil {
		return nil, err
	}

	current := 0
	next := func() interface{} {
		res := out[current]
		current++
		return res
	}

	validators := make([]*types.Validation, 0)
	masters := next().([]common.Address)
	endorsors := next().([]common.Address)
	statuses := next().([]uint8)
	onlines := next().([]bool)
	offlineBlocks := next().([]uint32)
	stakingPeriodLengths := next().([]uint32)
	startBlocks := next().([]uint32)
	exitBlocks := next().([]uint32)
	completedPeriods := next().([]uint32)
	validatorLockedVETs := next().([]*big.Int)
	validatorLockedWeights := next().([]*big.Int)
	delegatorsStakes := next().([]*big.Int)
	validatorQueuedStakes := next().([]*big.Int)
	totalQueuedStakes := next().([]*big.Int)
	exitingStakes := next().([]*big.Int)
	nextPeriodWeight := next().([]*big.Int)

	for i := range masters {
		totals := &builtin.ValidationTotals{
			TotalLockedStake:  new(big.Int).Add(validatorLockedVETs[i], delegatorsStakes[i]),
			TotalLockedWeight: validatorLockedWeights[i],
			TotalQueuedStake:  totalQueuedStakes[i],
			NextPeriodWeight:  nextPeriodWeight[i],
			TotalExitingStake: exitingStakes[i],
		}

		v := &types.Validation{
			Validation: &validation.Validation{
				Endorser:         (thor.Address)(endorsors[i]),
				Beneficiary:      nil, // Beneficiary is not used in this context
				Period:           stakingPeriodLengths[i],
				CompletedPeriods: completedPeriods[i],
				Status:           statuses[i],
				StartBlock:       startBlocks[i],
				LockedVET:        vetutil.ScaleToVET(validatorLockedVETs[i]),
				// TODO: find the validator exiting VET
				PendingUnlockVET: 0,
				QueuedVET:        vetutil.ScaleToVET(validatorQueuedStakes[i]),
				// TODO: Can we capture this?
				CooldownVET: 0,
				// TODO: Do we care about this?
				WithdrawableVET: 0,
				Weight:          vetutil.ScaleToVET(validatorLockedWeights[i]),
			},
			Address:               (thor.Address)(masters[i]),
			Online:                onlines[i],
			ValidationTotals:      totals,
			DelegatorStake:        delegatorsStakes[i],
			DelegatorWeight:       new(big.Int).Sub(validatorLockedWeights[i], big.NewInt(0).Mul(validatorLockedVETs[i], big.NewInt(2))),
			DelegatorQueuedStake:  new(big.Int).Sub(totalQueuedStakes[i], validatorQueuedStakes[i]),
			DelegatorQueuedWeight: new(big.Int).Sub(big.NewInt(0).Sub(nextPeriodWeight[i], validatorLockedWeights[i]), big.NewInt(0).Mul(validatorQueuedStakes[i], big.NewInt(2))),
		}
		if exitBlocks[i] != uint32(math.MaxUint32) {
			v.ExitBlock = &exitBlocks[i]
		}
		if offlineBlocks[i] != uint32(math.MaxUint32) {
			v.OfflineBlock = &offlineBlocks[i]
		}

		validators = append(validators, v)
	}

	return validators, nil
}

var stakerBalanceABI abi.Method
var getValidatorsABI abi.Method
var totalStakeABI abi.Method
var queuedStakeABI abi.Method
var totalSupplyABI abi.Method
var totalBurnedABI abi.Method
var stakerStorageABI abi.Method

var once sync.Once

func init() {
	var err error
	once.Do(func() {
		var helperABI abi.ABI
		helperABI, err = abi.JSON(bytes.NewReader([]byte(contractABI)))
		if err != nil {
			slog.Error("Failed to parse staker contract ABI", "error", err)
			return
		}
		var ok bool
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
		stakerStorageABI, ok = helperABI.Methods["stakerStorage"]
		if !ok {
			err = fmt.Errorf("stakerStorage method not found in staker contract ABI")
			slog.Error("Failed to find stakerStorage method", "error", err)
			return
		}
	})
	if err != nil {
		panic(err)
	}
}
