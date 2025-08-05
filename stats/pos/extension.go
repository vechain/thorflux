package pos

import (
	"github.com/vechain/thor/v2/builtin"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
	"math/big"
)

type Extension struct {
	contract *bind.Contract
	revision string
}

func NewExtension(client *thorclient.Client) (*Extension, error) {
	contract, err := bind.NewContract(client, builtin.Extension.RawABI(), &builtin.Extension.Address)
	if err != nil {
		return nil, err
	}
	return &Extension{contract: *contract}, nil
}

func (e *Extension) Revision(rev string) *Extension {
	return &Extension{
		contract: e.contract,
		revision: rev,
	}
}

func (e *Extension) TotalSupply() (*big.Int, error) {
	totalSupply := new(big.Int)
	if err := e.contract.Method("totalSupply").Call().ExecuteInto(&totalSupply); err != nil {
		return nil, err
	}
	return totalSupply, nil
}
