package influxdb

import (
	"github.com/darrenvechain/thor-go-sdk/client"
	"github.com/darrenvechain/thor-go-sdk/transaction"
	"github.com/ethereum/go-ethereum/common"
)

var b3trAddress = common.HexToAddress("0x5ef79995FE8a89e0812330E4378eB2660ceDe699")
var b3trGovernorAddress = common.HexToAddress("0x1c65C25fABe2fc1bCb82f253fA0C916a322f777C")
var emissionsAddress = common.HexToAddress("0xDf94739bd169C84fe6478D8420Bb807F1f47b135")
var galaxyMemberAddress = common.HexToAddress("0x93B8cD34A7Fc4f53271b9011161F7A2B5fEA9D1F")
var treasuryAddress = common.HexToAddress("0xD5903BCc66e439c753e525F8AF2FeC7be2429593")
var vot3Address = common.HexToAddress("0x76Ca782B59C74d088C7D2Cce2f211BC00836c602")
var x2eAppsAddress = common.HexToAddress("0x8392B7CCc763dB03b47afcD8E8f5e24F9cf0554D")
var x2eRewardsAddress = common.HexToAddress("0x6Bee7DDab6c99d5B2Af0554EaEA484CE18F52631")
var xAllocationPoolAddress = common.HexToAddress("0x4191776F05f4bE4848d3f4d587345078B439C7d3")
var xAllocationVotingAddress = common.HexToAddress("0x89A00Bb0947a30FF95BEeF77a66AEdE3842Fe5B7")

var allAddresses = []common.Address{
	b3trAddress,
	b3trGovernorAddress,
	emissionsAddress,
	galaxyMemberAddress,
	treasuryAddress,
	vot3Address,
	x2eAppsAddress,
	x2eRewardsAddress,
	xAllocationPoolAddress,
	xAllocationVotingAddress,
}

// isB3trClause returns true if the clause `to` contains a b3tr address, or if the output.Events contains a b3tr address.
func isB3trClause(clause transaction.Clause, output client.Output, reverted bool) bool {
	for _, addr := range allAddresses {
		to := clause.To()
		if to != nil && addr == *to {
			return true
		}
	}

	if reverted {
		return false
	}

	for _, event := range output.Events {
		for _, addr := range allAddresses {
			if addr == event.Address {
				return true
			}
		}
	}

	return false
}

func isB3trTransaction(tx client.BlockTransaction) bool {
	for i, clause := range tx.Clauses {
		output := client.Output{}
		if !tx.Reverted {
			output = tx.Outputs[i]
		}
		if isB3trClause(clause, output, tx.Reverted) {
			return true
		}
	}
	return false
}

func b3trStats(block *client.ExpandedBlock) (uint64, uint64, uint64) {
	var b3trTxs uint64
	var b3trClauses uint64
	var b3trGas uint64
	for _, t := range block.Transactions {
		if isB3trTransaction(t) {
			b3trTxs++
			b3trGas += t.GasUsed

			// TODO: Fine tune this to count the number of b3tr clauses in a transaction
			if t.Reverted {
				b3trClauses += uint64(len(t.Clauses))
			} else {
				for i, clause := range t.Clauses {
					if isB3trClause(clause, t.Outputs[i], t.Reverted) {
						b3trClauses++
					}
				}
			}
		}
	}

	return b3trTxs, b3trClauses, b3trGas
}
