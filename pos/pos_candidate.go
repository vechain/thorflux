package pos

import (
	"github.com/vechain/thor/v2/thor"
	"math/big"
)

type Candidate struct {
	Master   thor.Address
	Endorsor thor.Address
	Stake    big.Int
	Weight   big.Int
	Status   big.Int
}

type Placement struct {
	Start  *big.Rat
	End    *big.Rat
	Addr   thor.Address
	Hash   thor.Bytes32
	Weight big.Int
}
