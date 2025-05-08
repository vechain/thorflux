package pos

import (
	"math/big"

	"github.com/vechain/thor/v2/thor"
)

type Candidate struct {
	Master    thor.Address
	Endorsor  thor.Address
	Stake     big.Int
	Weight    big.Int
	Status    big.Int
	AutoRenew bool
}

type Placement struct {
	Start  *big.Rat
	End    *big.Rat
	Addr   thor.Address
	Hash   thor.Bytes32
	Weight big.Int
}
