package types

import (
	"math/big"
	"time"

	tapi "github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin/staker/validation"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient/builtin"
)

type Event struct {
	Block          *tapi.JSONExpandedBlock
	Seed           []byte
	HayabusaStatus HayabusaStatus
	Prev           *tapi.JSONExpandedBlock
	ChainTag       string
	DefaultTags    map[string]string
	Timestamp      time.Time
	Staker         *StakerInformation
	ParentStaker   *StakerInformation
}

type Validation struct {
	*validation.Validation
	*builtin.ValidationTotals
	Address               thor.Address // Address of the validator stake contract
	Online                bool         // Whether the validator is online
	DelegatorStake        *big.Int     // Total stake of delegators for this validator
	DelegatorWeight       *big.Int     // Total weight of delegators for this validator
	DelegatorQueuedStake  *big.Int     // Total queued stake of delegators for this validator
	DelegatorQueuedWeight *big.Int     // Total queued weight of delegators for this validator
}

type VTHO struct {
	TotalSupply *big.Int // Total supply of VTHO
	TotalBurned *big.Int // Total burned VTHO
}

type StakerInformation struct {
	Validations     []*Validation
	ContractBalance *big.Int // Balance of the staker contract
	QueuedVET       *big.Int // Total VET queued for staking
	TotalVET        *big.Int // Total VET staked in the network
	TotalWeight     *big.Int // Total weight of all validators
	VTHO            VTHO     // VTHO information
	IssuanceVTHO    *big.Int // Total VTHO issued in the network
	CooldownVET     uint64   // Total VET in cooldown
	WithdrawableVET uint64   // Total VET withdrawable
}

func (s *StakerInformation) ValidationMap() map[thor.Address]*Validation {
	validationMap := make(map[thor.Address]*Validation)
	for _, v := range s.Validations {
		validationMap[v.Address] = v
	}
	return validationMap
}

func (s *StakerInformation) ActiveMap() map[thor.Address]*Validation {
	activeMap := make(map[thor.Address]*Validation)
	for _, v := range s.Validations {
		if v.Status == validation.StatusActive {
			activeMap[v.Address] = v
		}
	}
	return activeMap
}

type HayabusaStatus struct {
	Active bool
	Forked bool
}
