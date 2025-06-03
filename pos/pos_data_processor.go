package pos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	thorAccounts "github.com/vechain/thor/v2/api/accounts"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/tx"
	"github.com/vechain/thorflux/accounts"
)

type PoSDataExtractor struct {
	Thor *thorclient.Client
}

func (posData *PoSDataExtractor) FetchAmount(parsedAbi abi.ABI, block *blocks.JSONExpandedBlock, chainTag byte, functionName string, contractAddress thor.Address) (*big.Int, error) {
	stake, err := posData.inspectClause(parsedAbi, block, chainTag, functionName, contractAddress)
	if err != nil {
		return nil, err
	}
	stakeParsed, err := thor.ParseBytes32(stake[0].Data)
	if err != nil {
		return nil, err
	}

	staked := big.NewInt(0).SetBytes(stakeParsed.Bytes())
	staked.Div(staked, big.NewInt(1e15))
	return staked, nil
}

func (posData *PoSDataExtractor) FetchStakeWeight(parsedAbi abi.ABI, block *blocks.JSONExpandedBlock, chainTag byte, functionName string, contractAddress thor.Address) (*big.Int, *big.Int, error) {
	stake, err := posData.inspectClause(parsedAbi, block, chainTag, functionName, contractAddress)
	if err != nil {
		return nil, nil, err
	}

	if len(stake[0].Data) < 130 {
		return big.NewInt(0), big.NewInt(0), nil
	}

	stakeParsed, err := thor.ParseBytes32(stake[0].Data[2:66])
	if err != nil {
		return nil, nil, err
	}
	weightParsed, err := thor.ParseBytes32(stake[0].Data[66:])
	if err != nil {
		return nil, nil, err
	}

	staked := big.NewInt(0).SetBytes(stakeParsed.Bytes())
	stakedVet := big.NewInt(0).Div(staked, big.NewInt(1e18))
	weight := big.NewInt(0).SetBytes(weightParsed.Bytes())
	weightVet := big.NewInt(0).Div(weight, big.NewInt(1e18))

	return stakedVet, weightVet, nil
}

func (posData *PoSDataExtractor) inspectClause(parsedAbi abi.ABI, block *blocks.JSONExpandedBlock, chainTag byte, functionName string, contractAddress thor.Address, args ...any) ([]*thorAccounts.CallResult, error) {
	methodData, err := parsedAbi.Pack(functionName, args...)
	if err != nil {
		return nil, err
	}

	stakeTx := new(tx.Builder).GasPriceCoef(255).
		BlockRef(tx.NewBlockRef(block.Number)).
		Expiration(1000).
		ChainTag(chainTag).
		Gas(10e6).
		Nonce(rand.Uint64()).
		Clause(
			tx.NewClause(&contractAddress).WithData(methodData),
		).Build()

	return posData.Thor.InspectTxClauses(stakeTx, &accounts.Caller, thorclient.Revision(block.ID.String()))
}

func (posData *PoSDataExtractor) ExtractCandidates(block *blocks.JSONExpandedBlock, chainTag byte) ([]*Candidate, error) {
	parsedABI, err := abi.JSON(strings.NewReader(accounts.StakerAbi))
	if err != nil {
		return nil, err
	}
	result, err := posData.inspectClause(parsedABI, block, chainTag, "firstActive", accounts.StakerContract)
	if err != nil {
		return nil, err
	}

	id, err := thor.ParseBytes32(result[0].Data)
	if err != nil {
		return nil, err
	}

	result, err = posData.inspectClause(parsedABI, block, chainTag, "get", accounts.StakerContract, id)
	if err != nil {
		return nil, err
	}

	firstActiveAddress, err := thor.ParseAddress(result[0].Data[26:66])
	if err != nil {
		return nil, err
	}

	candidates := make([]*Candidate, 0)
	candidate, err := posData.getCandidate(result[0].Data, firstActiveAddress)
	candidates = append(candidates, candidate)
	if err != nil {
		return nil, err
	}

	for candidate != nil {
		next, err := posData.inspectClause(parsedABI, block, chainTag, "next", accounts.StakerContract, id)
		if err != nil {
			return nil, err
		}
		nextId, err := thor.ParseBytes32(next[0].Data)
		if err != nil {
			return nil, err
		}
		id = nextId
		nextGet, err := posData.inspectClause(parsedABI, block, chainTag, "get", accounts.StakerContract, nextId)
		if err != nil {
			return nil, err
		}

		nextAddress, err := thor.ParseAddress(nextGet[0].Data[26:66])
		if nextAddress.IsZero() {
			candidate = nil
		} else {
			candidate, err = posData.getCandidate(nextGet[0].Data, nextAddress)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func (posData *PoSDataExtractor) getCandidate(getData string, address thor.Address) (*Candidate, error) {
	endorsor, err := thor.ParseAddress(getData[90:130])
	if err != nil {
		return nil, err
	}

	stakeBytes, err := thor.ParseBytes32(getData[130:194])
	if err != nil {
		return nil, err
	}
	stake := big.NewInt(0).SetBytes(stakeBytes.Bytes())

	weightBytes, err := thor.ParseBytes32(getData[194:258])
	if err != nil {
		return nil, err
	}
	weight := big.NewInt(0).SetBytes(weightBytes.Bytes())

	statusBytes, err := thor.ParseBytes32(getData[258:322])
	if err != nil {
		return nil, err
	}
	status := big.NewInt(0).SetBytes(statusBytes.Bytes())
	autoRenewBytes, err := thor.ParseBytes32(getData[322:386])
	if err != nil {
		return nil, err
	}
	autoRenew := autoRenewBytes.Bytes()[31] != 0
	// online is the 7th returned value from get function (master, endorser, stake, weight, status, autoRenew, online, period),
	// so we are processing 7h position of the returned value from 2 + (6 * 64) to 2 + (7 * 64)
	onlineBytes, err := thor.ParseBytes32(getData[386:450])
	if err != nil {
		return nil, err
	}
	online := onlineBytes.Bytes()[31] != 0
	candidate := Candidate{
		Master:    address,
		Endorsor:  endorsor,
		Stake:     *stake,
		Weight:    *weight,
		Status:    *status,
		AutoRenew: autoRenew,
		Online:    online,
	}
	return &candidate, nil
}

func ExpectedValidator(candidates []*Candidate, currentBlock *blocks.JSONExpandedBlock, seed []byte) (*thor.Address, error) {

	hash := thor.Blake2b(seed, big.NewInt(0).SetUint64(currentBlock.Timestamp+10).Bytes())
	selector := new(big.Rat).SetInt(new(big.Int).SetBytes(hash.Bytes()))
	divisor := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), uint(len(hash)*8)))

	selector.Quo(selector, divisor)

	placements := make([]Placement, 0, len(candidates))
	onlineStake := big.NewInt(0)
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], currentBlock.Number)

	for idx := range candidates {
		entry := candidates[idx]
		onlineStake.Add(onlineStake, &entry.Weight)
		placements = append(placements, Placement{
			Addr:   entry.Master,
			Hash:   thor.Blake2b(seed, num[:], entry.Master.Bytes()),
			Weight: entry.Weight,
		})
	}

	if onlineStake.Cmp(big.NewInt(0)) <= 0 {
		return &thor.Address{}, fmt.Errorf("no online stake in the network")
	}

	sort.Slice(placements, func(i, j int) bool {
		return bytes.Compare(placements[i].Hash.Bytes(), placements[j].Hash.Bytes()) < 0
	})

	prev := big.NewRat(0, 1)
	totalStakeRat := new(big.Rat).SetInt(onlineStake)

	for i := range placements {
		weightRat := new(big.Rat).SetInt(&placements[i].Weight)
		weight := new(big.Rat).Quo(weightRat, totalStakeRat)

		placements[i].Start = new(big.Rat).Set(prev)
		placements[i].End = new(big.Rat).Add(prev, weight)
		prev = placements[i].End
	}

	for i := range placements {
		if selector.Cmp(placements[i].Start) >= 0 && selector.Cmp(placements[i].End) < 0 {
			return &placements[i].Addr, nil
		}
	}
	return &thor.Address{}, nil
}

func (posData *PoSDataExtractor) IsHayabusaFork() bool {
	code, err := posData.Thor.AccountCode(&accounts.StakerContract)
	if err != nil {
		slog.Error("Failed to get contract code", "error", err)
		return false
	}

	if len(code.Code) == 0 || code.Code == "0x" {
		return false
	}

	return true
}

func (posData *PoSDataExtractor) IsHayabusaActive() bool {
	posActiveTimeKey := thor.BytesToBytes32([]byte("hayabusa-energy-growth-stop-time"))
	posActivated, err := posData.Thor.AccountStorage(&accounts.EnergyContract, &posActiveTimeKey)
	if err != nil {
		slog.Error("Failed to get pos activated time", "error", err)
		return false
	}

	if len(posActivated.Value) == 0 || posActivated.Value == "0x" {
		return false
	}

	posTime, err := thor.ParseBytes32(posActivated.Value)
	if err != nil {
		return false
	}

	posTimeParsed := big.NewInt(0).SetBytes(posTime.Bytes())

	return posTimeParsed.Cmp(big.NewInt(0)) > 0
}
