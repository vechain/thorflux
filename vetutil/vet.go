package vetutil

import "math/big"

var (
	VET = big.NewInt(1e18) // 1 VET in wei
)

func ScaleToVET(wei *big.Int) uint64 {
	if wei == nil {
		return 0
	}
	bigInt := new(big.Int).Div(wei, VET)
	return bigInt.Uint64()
}
