package accounts

import (
	"fmt"
	"github.com/vechain/thor/v2/abi"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/thor"
)

var tokenTransferEvent = thor.MustParseBytes32("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

var StakerAbi = `[
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "delegationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "uint256",
        "name": "stake",
        "type": "uint256"
      },
      {
        "indexed": false,
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      },
      {
        "indexed": false,
        "internalType": "uint8",
        "name": "multiplier",
        "type": "uint8"
      }
    ],
    "name": "DelegationAdded",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "delegationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      }
    ],
    "name": "DelegationUpdatedAutoRenew",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "delegationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "uint256",
        "name": "stake",
        "type": "uint256"
      }
    ],
    "name": "DelegationWithdrawn",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "address",
        "name": "endorsor",
        "type": "address"
      },
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "uint256",
        "name": "removed",
        "type": "uint256"
      }
    ],
    "name": "StakeDecreased",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "address",
        "name": "endorsor",
        "type": "address"
      },
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "uint256",
        "name": "added",
        "type": "uint256"
      }
    ],
    "name": "StakeIncreased",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "address",
        "name": "endorsor",
        "type": "address"
      },
      {
        "indexed": true,
        "internalType": "address",
        "name": "master",
        "type": "address"
      },
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "uint32",
        "name": "period",
        "type": "uint32"
      },
      {
        "indexed": false,
        "internalType": "uint256",
        "name": "stake",
        "type": "uint256"
      },
      {
        "indexed": false,
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      }
    ],
    "name": "ValidatorQueued",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "address",
        "name": "endorsor",
        "type": "address"
      },
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      }
    ],
    "name": "ValidatorUpdatedAutoRenew",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "internalType": "address",
        "name": "endorsor",
        "type": "address"
      },
      {
        "indexed": true,
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "indexed": false,
        "internalType": "uint256",
        "name": "stake",
        "type": "uint256"
      }
    ],
    "name": "ValidatorWithdrawn",
    "type": "event"
  },
  {
    "stateMutability": "nonpayable",
    "type": "fallback"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      },
      {
        "internalType": "uint8",
        "name": "multiplier",
        "type": "uint8"
      }
    ],
    "name": "addDelegation",
    "outputs": [
      {
        "internalType": "bytes32",
        "name": "",
        "type": "bytes32"
      }
    ],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "address",
        "name": "master",
        "type": "address"
      },
      {
        "internalType": "uint32",
        "name": "period",
        "type": "uint32"
      },
      {
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      }
    ],
    "name": "addValidator",
    "outputs": [],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "id",
        "type": "bytes32"
      },
      {
        "internalType": "uint256",
        "name": "amount",
        "type": "uint256"
      }
    ],
    "name": "decreaseStake",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "firstActive",
    "outputs": [
      {
        "internalType": "bytes32",
        "name": "",
        "type": "bytes32"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "firstQueued",
    "outputs": [
      {
        "internalType": "bytes32",
        "name": "",
        "type": "bytes32"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "id",
        "type": "bytes32"
      }
    ],
    "name": "get",
    "outputs": [
      {
        "internalType": "address",
        "name": "",
        "type": "address"
      },
      {
        "internalType": "address",
        "name": "",
        "type": "address"
      },
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      },
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      },
      {
        "internalType": "uint8",
        "name": "",
        "type": "uint8"
      },
      {
        "internalType": "bool",
        "name": "",
        "type": "bool"
      },
      {
        "internalType": "bool",
        "name": "",
        "type": "bool"
      },
      {
        "internalType": "uint32",
        "name": "",
        "type": "uint32"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      }
    ],
    "name": "getCompletedPeriods",
    "outputs": [
      {
        "internalType": "uint32",
        "name": "",
        "type": "uint32"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "delegationID",
        "type": "bytes32"
      }
    ],
    "name": "getDelegation",
    "outputs": [
      {
        "internalType": "bytes32",
        "name": "",
        "type": "bytes32"
      },
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      },
      {
        "internalType": "uint32",
        "name": "",
        "type": "uint32"
      },
      {
        "internalType": "uint32",
        "name": "",
        "type": "uint32"
      },
      {
        "internalType": "uint8",
        "name": "",
        "type": "uint8"
      },
      {
        "internalType": "bool",
        "name": "",
        "type": "bool"
      },
      {
        "internalType": "bool",
        "name": "",
        "type": "bool"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      },
      {
        "internalType": "uint32",
        "name": "stakingPeriod",
        "type": "uint32"
      }
    ],
    "name": "getRewards",
    "outputs": [
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "id",
        "type": "bytes32"
      }
    ],
    "name": "getWithdraw",
    "outputs": [
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "validationID",
        "type": "bytes32"
      }
    ],
    "name": "increaseStake",
    "outputs": [],
    "stateMutability": "payable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "prev",
        "type": "bytes32"
      }
    ],
    "name": "next",
    "outputs": [
      {
        "internalType": "bytes32",
        "name": "",
        "type": "bytes32"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "queuedStake",
    "outputs": [
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      },
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [],
    "name": "totalStake",
    "outputs": [
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      },
      {
        "internalType": "uint256",
        "name": "",
        "type": "uint256"
      }
    ],
    "stateMutability": "view",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "id",
        "type": "bytes32"
      },
      {
        "internalType": "bool",
        "name": "autoRenew",
        "type": "bool"
      }
    ],
    "name": "updateAutoRenew",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "delegationID",
        "type": "bytes32"
      },
      {
        "internalType": "bool",
        "name": "active",
        "type": "bool"
      }
    ],
    "name": "updateDelegationAutoRenew",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "id",
        "type": "bytes32"
      }
    ],
    "name": "withdraw",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "inputs": [
      {
        "internalType": "bytes32",
        "name": "delegationID",
        "type": "bytes32"
      }
    ],
    "name": "withdrawDelegation",
    "outputs": [],
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "stateMutability": "payable",
    "type": "receive"
  }
]`

var ExtensionAbi = `
[
  {
    "constant": true,
    "inputs": [],
    "name": "totalSupply",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "data",
        "type": "bytes"
      }
    ],
    "name": "blake2b256",
    "outputs": [
      {
        "name": "",
        "type": "bytes32"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "num",
        "type": "uint256"
      }
    ],
    "name": "blockTime",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "num",
        "type": "uint256"
      }
    ],
    "name": "blockSigner",
    "outputs": [
      {
        "name": "",
        "type": "address"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "num",
        "type": "uint256"
      }
    ],
    "name": "blockTotalScore",
    "outputs": [
      {
        "name": "",
        "type": "uint64"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "txGasPayer",
    "outputs": [
      {
        "name": "",
        "type": "address"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "txExpiration",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "txID",
    "outputs": [
      {
        "name": "",
        "type": "bytes32"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "txProvedWork",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "num",
        "type": "uint256"
      }
    ],
    "name": "blockID",
    "outputs": [
      {
        "name": "",
        "type": "bytes32"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "txBlockRef",
    "outputs": [
      {
        "name": "",
        "type": "bytes8"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  }
]`

var EnergyAbi = `[
  {
    "constant": true,
    "inputs": [],
    "name": "name",
    "outputs": [
      {
        "name": "",
        "type": "string"
      }
    ],
    "payable": false,
    "stateMutability": "pure",
    "type": "function"
  },
  {
    "constant": false,
    "inputs": [
      {
        "name": "_spender",
        "type": "address"
      },
      {
        "name": "_value",
        "type": "uint256"
      }
    ],
    "name": "approve",
    "outputs": [
      {
        "name": "success",
        "type": "bool"
      }
    ],
    "payable": false,
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "totalSupply",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": false,
    "inputs": [
      {
        "name": "_from",
        "type": "address"
      },
      {
        "name": "_to",
        "type": "address"
      },
      {
        "name": "_amount",
        "type": "uint256"
      }
    ],
    "name": "transferFrom",
    "outputs": [
      {
        "name": "success",
        "type": "bool"
      }
    ],
    "payable": false,
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "decimals",
    "outputs": [
      {
        "name": "",
        "type": "uint8"
      }
    ],
    "payable": false,
    "stateMutability": "pure",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "_owner",
        "type": "address"
      }
    ],
    "name": "balanceOf",
    "outputs": [
      {
        "name": "balance",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "symbol",
    "outputs": [
      {
        "name": "",
        "type": "string"
      }
    ],
    "payable": false,
    "stateMutability": "pure",
    "type": "function"
  },
  {
    "constant": false,
    "inputs": [
      {
        "name": "_to",
        "type": "address"
      },
      {
        "name": "_amount",
        "type": "uint256"
      }
    ],
    "name": "transfer",
    "outputs": [
      {
        "name": "success",
        "type": "bool"
      }
    ],
    "payable": false,
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "constant": false,
    "inputs": [
      {
        "name": "_from",
        "type": "address"
      },
      {
        "name": "_to",
        "type": "address"
      },
      {
        "name": "_amount",
        "type": "uint256"
      }
    ],
    "name": "move",
    "outputs": [
      {
        "name": "success",
        "type": "bool"
      }
    ],
    "payable": false,
    "stateMutability": "nonpayable",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [],
    "name": "totalBurned",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "constant": true,
    "inputs": [
      {
        "name": "_owner",
        "type": "address"
      },
      {
        "name": "_spender",
        "type": "address"
      }
    ],
    "name": "allowance",
    "outputs": [
      {
        "name": "remaining",
        "type": "uint256"
      }
    ],
    "payable": false,
    "stateMutability": "view",
    "type": "function"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "name": "_from",
        "type": "address"
      },
      {
        "indexed": true,
        "name": "_to",
        "type": "address"
      },
      {
        "indexed": false,
        "name": "_value",
        "type": "uint256"
      }
    ],
    "name": "Transfer",
    "type": "event"
  },
  {
    "anonymous": false,
    "inputs": [
      {
        "indexed": true,
        "name": "_owner",
        "type": "address"
      },
      {
        "indexed": true,
        "name": "_spender",
        "type": "address"
      },
      {
        "indexed": false,
        "name": "_value",
        "type": "uint256"
      }
    ],
    "name": "Approval",
    "type": "event"
  }
]`

var StakerContract = thor.MustParseAddress("0x00000000000000000000000000005374616b6572")
var ExtensionContract = thor.MustParseAddress("0x0000000000000000000000457874656e73696f6e")
var EnergyContract = thor.MustParseAddress("0x0000000000000000000000000000456e65726779")
var Caller = thor.MustParseAddress("0x00000000000000000000000000005374616b6572")

type ContractInfo struct {
	Name  string
	Count int
	// 'account', 'contract', 'erc721', 'erc20'
	Type string
}

type CallData struct {
	BlockRef   string   `json:"blockRef"`
	Caller     string   `json:"caller"`
	Clauses    []Clause `json:"clauses"`
	Expiration uint64   `json:"expiration"`
	Gas        uint64   `json:"gas"`
	GasPayer   string   `json:"gasPayer"`
	GasPrice   string   `json:"gasPrice"` // Using string for large numbers
	ProvedWork string   `json:"provedWork"`
}

type Clause struct {
	To    string `json:"to"`    // Pointer to string because 'to' can be null
	Value string `json:"value"` // Using string for hex values
	Data  string `json:"data"`
}

type Stats struct {
	KnownAccountTxs     map[string]ContractInfo
	ClauseCount         int
	TxCount             int
	KnownNftCount       int
	KnownErc20Count     int
	TotalTokenTransfers int
}

func updateContractInfo(knownContractTXs map[string]ContractInfo, clause *blocks.JSONClause, name string, contractType string) {
	existing, ok := knownContractTXs[clause.To.String()]
	if !ok {
		knownContractTXs[clause.To.String()] = ContractInfo{
			Name:  name,
			Count: 1,
			Type:  contractType,
		}
	} else {
		existing.Count++
		knownContractTXs[clause.To.String()] = existing
	}
}

func EncodeCallData(functionAbi string, functionName string, args ...any) ([]byte, error) {
	parsedAbi, err := abi.New([]byte(functionAbi))
	if err != nil {
		return nil, err
	}
	methodABI, ok := parsedAbi.MethodByName(functionName)
	if !ok {
		return nil, fmt.Errorf("method not found")
	}
	data, err := methodABI.EncodeInput(args...)
	return data, err
}

func GetStats(block *blocks.JSONExpandedBlock) *Stats {
	knownContractTXs := make(map[string]ContractInfo)
	knownErc20Count := 0
	knownNftCount := 0
	txCount := 0
	clauseCount := 0
	tokenContractTransfers := 0

	for _, tx := range block.Transactions {
		knownTx := false
		for _, clause := range tx.Clauses {

			if clause.To == nil {
				continue
			}

			if knownContracts[*clause.To] != "" {
				knownTx = true
				clauseCount++
				updateContractInfo(knownContractTXs, clause, knownContracts[*clause.To], "contract")
			}

			if nftContracts[*clause.To] != "" {
				clauseCount++
				knownNftCount++
				knownTx = true
				updateContractInfo(knownContractTXs, clause, nftContracts[*clause.To], "erc721")
			}

			if fungibleTokens[*clause.To] != "" {
				clauseCount++
				knownErc20Count++
				knownTx = true
				updateContractInfo(knownContractTXs, clause, fungibleTokens[*clause.To], "erc20")
			}

			if knownAddresses[*clause.To] != "" {
				clauseCount++
				knownTx = true
				updateContractInfo(knownContractTXs, clause, knownAddresses[*clause.To], "account")
			}
		}
		if knownTx {
			txCount++
		}

		for _, output := range tx.Outputs {
			for _, event := range output.Events {
				if len(event.Topics) > 0 && event.Topics[0] == tokenTransferEvent {
					tokenContractTransfers++
				}
			}
		}
	}

	stats := &Stats{
		KnownAccountTxs:     knownContractTXs,
		KnownNftCount:       knownNftCount,
		KnownErc20Count:     knownErc20Count,
		TxCount:             txCount,
		ClauseCount:         clauseCount,
		TotalTokenTransfers: tokenContractTransfers,
	}

	return stats
}

var knownContracts = map[thor.Address]string{
	thor.MustParseAddress("0xb81e9c5f9644dec9e5e3cac86b4461a222072302"): "VeChain Node",
	thor.MustParseAddress("0xe28ce32d637eb93cbda105f87fbb829e9ef8540b"): "VeChain Auction",
	thor.MustParseAddress("0xdbaec4165a6cff07901c41d3561eefdcfbc20cb6"): "Steering Committee Vote",
	thor.MustParseAddress("0x0000000000000000000000417574686f72697479"): "VeChain Authority Node",
	thor.MustParseAddress("0xa6416a72f816d3a69f33d0814700545c8e3fe4be"): "VeVote Contract",
	thor.MustParseAddress("0xf57a7cdee288ecc97dd90b56778acb724a1a1d59"): "KaY - Not Part of VeChain",
	thor.MustParseAddress("0xba6b65f7a48636b3e533205d9070598b4faf6a0c"): "DNV",
	thor.MustParseAddress("0x20eb29a2f76021c4cb5bbd4ee359a6172ee2e30a"): "Supply@Me",
	thor.MustParseAddress("0xf72614024c9273320b2f82dda3932785df6b9208"): "Aretaeio Hospital",
	thor.MustParseAddress("0x54f14e2e4a204a8c1b734c1b73d6d7cb96894a61"): "ToolChain Partners",
	thor.MustParseAddress("0x91ace4b91fc65ee930724deb580dfe80c135713e"): "ToolChain Partners",
	thor.MustParseAddress("0xc3c118c6fa5479244b9f0da0b0ba8f9afa8dc33c"): "ToolChain Partners",
	thor.MustParseAddress("0xeddc51042586b66cf8fb75e971636c76ce2e9c35"): "ToolChain Partners",
	thor.MustParseAddress("0x7995bdbc94ab8bd33f77457416214a4abe0b8631"): "ToolChain Partners",
	thor.MustParseAddress("0xd9a43482cb6af1b7236cde4f7ec3201ac2a13d79"): "ToolChain Partners",
	thor.MustParseAddress("0xf41aab18649523a18de81042bd30da07b37b1ec1"): "ToolChain Partners",
	thor.MustParseAddress("0x11d27c2307c108990d3874b5b3dac3209bc7eee4"): "ToolChain Partners",
	thor.MustParseAddress("0x96fa20b2162cae29b3ccb7984d3b23459200722a"): "ToolChain Partners",
	thor.MustParseAddress("0x7977f06e9f1d7f5bf0d73cd8de24bd16ddea2cf2"): "ToolChain Partners",
	thor.MustParseAddress("0x0eca11f7035fb0cdfa059796add7f6552560b968"): "YONEX",
	thor.MustParseAddress("0xe5e83b13c4b4042bae5809b1c5d1bed4bb3836dc"): "ToolChain Partners",
	thor.MustParseAddress("0x65e8d64bcb5b5e235626958ed116d4a9d7aea081"): "ToolChain Partners",
	thor.MustParseAddress("0x8f23de2ad8d4fc4955539fd6cd160eb25946237e"): "ToolChain Partners",
	thor.MustParseAddress("0xcf4645cc606c48bfe63a3b4987628f1b2760db08"): "ToolChain Partners",
	thor.MustParseAddress("0x9398648b907399bd0fcb5a8d0822e8daa66378ef"): "Yongpu Coffee",
	thor.MustParseAddress("0xea665486ba7d2d1904e9ea8694860e825d4beaf0"): "ToolChain Partners",
	thor.MustParseAddress("0xfee823ac958e34973d124218f8ddbe65a651a08b"): "NSF International",
	thor.MustParseAddress("0x66f36d228a5201419dff9895dcfb8bf45c3cf262"): "San Marino Green Pass NFT",
	thor.MustParseAddress("0xdcaa96e264eb8514b130e1a97072b41c875bec7b"): "San Marino Green Pass Data Upload",
	thor.MustParseAddress("0x8e1bf526c0e40e8abe6a34129a1f68c2d489ac96"): "Inner Mongolia Traceability Platform",
	thor.MustParseAddress("0xecc159751f9aed21399d5e3ce72bc9d4fccb9ccc"): "MyStory",
	thor.MustParseAddress("0xbe7a61b0405fdfbaae28c1355cd53c8affc1c4b0"): "Walmart China",
	thor.MustParseAddress("0x1a2f8fc8e821f46d6962bb0a4e06349a3ad4cf33"): "Walmart China 2",
	thor.MustParseAddress("0x1Cc13a24b1F73288cc7791C2c8Fd428357405226"): "MyCare",
	thor.MustParseAddress("0x1a048cff120f3ebff9bb66459effa34445c8e87e"): "KnowSeafood",
	thor.MustParseAddress("0xa9f3c1bd52c3a506cecbb8cbca562ef26c833175"): "Yuhongtai Foods",
	thor.MustParseAddress("0x505b95e128e403634fe6090472485341905fc0f9"): "Yunnan Pu`er Tea",
	thor.MustParseAddress("0xbb763cea82127548c465f6ad83a297f292e5c2fb"): "Reebonz",
	thor.MustParseAddress("0xfbc5c4e371164e3f1dc5d1760a98f3d227ba7e3b"): "Reebonz",
	thor.MustParseAddress("0x9ee753d070c1fd42d715e951bd8d5441e6c7d052"): "Reebonz",
	thor.MustParseAddress("0xc89dcd4b36b5182f974c556408681cd035be18e4"): "FoodGates",
	thor.MustParseAddress("0xbdccecf078f27cc9bf7a18b4cc2c25068a616fb4"): "Shanghai Gas",
	thor.MustParseAddress("0x9bcb81a9eadd1457ee9729365f9a77d190670ab2"): "Shanghai Gas",
	thor.MustParseAddress("0x576da7124c7bb65a692d95848276367e5a844d95"): "Router02",
	thor.MustParseAddress("0xbdc2edaea65b51053ffce8bc0721753c7895e12f"): "VeRocket",
	thor.MustParseAddress("0x29a996b0ebb7a77023d091c9f2ca34646bea6ede"): "VeRocket",
	thor.MustParseAddress("0x58108ba70902869f42eb12c5fdbc0cefab0ad13d"): "VeRocket",
	thor.MustParseAddress("0x7A3d485cC586d2c5543b0DF3B93043CFA8Aec6D6"): "VeRocket",
	thor.MustParseAddress("0x1a8abd6d5627eb26ad71c0c7ae5224cdc640faf3"): "VeRocket",
	thor.MustParseAddress("0xfe778e3491ae917e76e85ba8d30426ee1cccba06"): "VeRocket",
	thor.MustParseAddress("0xc8c0b13f1152dbd825ecf67c245291aee215a109"): "VeRocket",
	thor.MustParseAddress("0x10ba14b7afec1f3ab701be127ab436de21cdd055"): "VeRocket",
	thor.MustParseAddress("0x94355b3079a38a265e6b7a825ab6a06495c2d419"): "VeRocket",
	thor.MustParseAddress("0x629965c25e1c5d57fb268b23a79c76520bca6698"): "VeRocket",
	thor.MustParseAddress("0x6b7e1aeff308d56f9a8ba1e57174ef97e6cde06d"): "VeRocket",
	thor.MustParseAddress("0x8f34a3764750feb71264e9a105cf07fc301d70d1"): "VeRocket",
	thor.MustParseAddress("0x72189e536dcb19bc6e1b4918a07b60ef8aca41d8"): "VeRocket",
	thor.MustParseAddress("0xb79d201ec8c187e68dad902cc5a14b54b3a7df40"): "VeRocket",
	thor.MustParseAddress("0x0bb35811213df1c6247041e456639b83dd4e4017"): "VeRocket",
	thor.MustParseAddress("0xc75668ce138dd65f4de37d986a84ebdef71cda02"): "VeRocket",
	thor.MustParseAddress("0x538f8890a383c44e59df4c7263d96ca8048da2c7"): "Vexchange",
	thor.MustParseAddress("0xf9f99f982f3ea9020f0a0afd4d4679dfee1b63cf"): "Vexchange",
	thor.MustParseAddress("0xdc391a5dbb89a3f768c41cfa0e85dcaaf3a91f91"): "Vexchange",
	thor.MustParseAddress("0xdc690f1a5de6108239d2d91cfdaa1d19e7ef7f82"): "Vexchange",
	thor.MustParseAddress("0xa8d1a1c88329320234581e203474fe19b99473d3"): "Vexchange",
	thor.MustParseAddress("0x6d08d19dff533050f93eaaa0a009e2771d3598bc"): "Vexchange",
	thor.MustParseAddress("0x6c0a6e1d922e0e63901301573370b932ae20dadb"): "Vexchange",
	thor.MustParseAddress("0xd86bed355d9d6a4c951e96755dd0c3cf004d6cd0"): "Vexchange",
	thor.MustParseAddress("0xc19cf5dfb71374b920f786078d37b5225cfcf30e"): "Vexchange",
	thor.MustParseAddress("0xf306dfc3c4a276ac4c1795c5896e9f4a967341b6"): "realitems.io",
	thor.MustParseAddress("0xbc90a27cef38c774717bf1dfd13ff9a906920215"): "realitems.io",
	thor.MustParseAddress("0xe860cef926e5e76e0e88fdc762417a582f849c27"): "XPN",
	thor.MustParseAddress("0x06ff1e4b5e15d890e746dbefad3e2162a31c10b7"): "XPN",
	thor.MustParseAddress("0xfc9a4759209e445f96cd17bc79b16b9cf7364799"): "XPN",
	thor.MustParseAddress("0xf0e778bd5c4c2f219a2a5699e3afd2d82d50e271"): "XPN",
	thor.MustParseAddress("0xa7f8b361060222b3aee75f4b457ba0353cf10998"): "E-HCert",
	thor.MustParseAddress("0x040093ab307f5acb4ae3afb0fb31de0ec46d62f9"): "Safehaven",
	thor.MustParseAddress("0x7196c6b28f5edac5d9134e44051635cc572fe07b"): "Safehaven",
	thor.MustParseAddress("0x1f711f78685b4a5b0899d26aebd590163cfcb7eb"): "Hacken Club Membership ",
	thor.MustParseAddress("0xffa34bf5b1d7178bd9a9815c84bc64570d88560c"): "Vulcan",
	thor.MustParseAddress("0xdab45be2d501549d11b5712f4c804d793fae5d0b"): "Vulcan",
	thor.MustParseAddress("0x81d5973a21c2dacf9e2a4abcce807338036a3954"): "Vulcan",
	thor.MustParseAddress("0xb1b9d40758cc3d90f1b2899dfb7a64e5d0235c61"): "Vulcan",
	thor.MustParseAddress("0x27b508dba99a05c7810d4956d74daa71bac0d969"): "VIM",
	thor.MustParseAddress("0x05b866b65f3fbf118d45ca2157c43d888f001dd1"): "VIM",
	thor.MustParseAddress("0x9792bc3fe1d998af4f756d3db7fa017b05024ea9"): "VIM",
	thor.MustParseAddress("0xc6cd73941365c0e22d0eeaa8944aa9d0efe554d6"): "VIM",
	thor.MustParseAddress("0xb3e9ab43306695dd7ab5a1c8a68db877206da298"): "VIM",
	thor.MustParseAddress("0x46a7567f65c278b119ddeabf440f42ba2de949c0"): "VIM",
	thor.MustParseAddress("0x36ead69626aecfa792b1cb8a546d3c1b37ac8ee5"): "VIM",
	thor.MustParseAddress("0x65f7ac9ece8ed59044768d53c0d44e3cb7f6ceff"): "Ubique",
	thor.MustParseAddress("0x4fe1ac0b38339a59682fea4ef970404cf989b09c"): "Indigiledger",
	thor.MustParseAddress("0xe1fc8ecc13dc25db25fa4e7c756acbc87f965e60"): "FoodGates",
	thor.MustParseAddress("0x2dd241b93e435046d7264357d67eb58a9cea5857"): "FoodGates",
	thor.MustParseAddress("0x535ab4a9fce43dc71e9540534733bbeb0f494d5c"): "burntoken.io",
	thor.MustParseAddress("0xc7fd71b05b3060fce96e4b6cdc6ec353fa6f838e"): "Marketplace Community NFT",
	thor.MustParseAddress("0xc3f851f9f78c92573620582bf9002f0c4a114b67"): "Marketplace Community NFT",
	thor.MustParseAddress("0x058d4c951aa24ca012cef3408b259ac1c69d1258"): "Marketplace",
	thor.MustParseAddress("0xe01cb06168f52b40fc60d5ce218346361a75efe7"): "Auction Marketplace",
	thor.MustParseAddress("0xe56861c0bb8012ec955da4e4122895ed2a46d229"): "Offer Marketplace",
	thor.MustParseAddress("0x5e6265680087520dc022d75f4c45f9ccd712ba97"): "Open Mint Community NFT",
	thor.MustParseAddress("0xc7592f90a6746e5d55e4a1543b6cae6d5b11d258"): "Account Registry Contract",
	thor.MustParseAddress("0x93ae8aab337e58a6978e166f8132f59652ca6c56"): "WorldOfV",
	thor.MustParseAddress("0x9aab6e4e017964ec7c0f092d431c314f0caf6b4b"): "WorldOfV",
	thor.MustParseAddress("0x2a7bc6e39bcf51f5c55e7fc779e6b4da30be30c3"): "WorldOfV",
	thor.MustParseAddress("0x73f32592df5c0da73d56f34669d4ae28ae1afd9e"): "WorldOfV",
	thor.MustParseAddress("0xf92b2a2ff63ff09933c0ae797eff594ea3498c81"): "WorldOfV",
	thor.MustParseAddress("0xb14baed957b8e58db10ec5ef37927d83b3bbf297"): "WorldOfV",
	thor.MustParseAddress("0xf19fe0f222e4f2a7587b817042fe58f4f330a009"): "WorldOfV",
	thor.MustParseAddress("0xa723a21419181a9ddee6e3981d5854a05c9e90e1"): "WorldOfV",
	thor.MustParseAddress("0x41a03b04725c20f3902c67ee7416e5df4266df45"): "WorldOfV",
	thor.MustParseAddress("0x167f6cc1e67a615b51b5a2deaba6b9feca7069df"): "WorldOfV",
	thor.MustParseAddress("0xda878be46f4a6ec013340fb985231ed67eb712d3"): "WorldOfV",
	thor.MustParseAddress("0xa4bf5a32d0f1d1655eec3297023fd2136bd760a2"): "WorldOfV",
	thor.MustParseAddress("0x3b9521745ae47418c3c636ec1e76f135cdc961fc"): "WorldOfV",
	thor.MustParseAddress("0xd861be8e33ebd09764bfca242ca6a8c54dcf844a"): "WorldOfV",
	thor.MustParseAddress("0x9c872e8420ec38f404402bea8f8f86d5d2c17782"): "WorldOfV",
	thor.MustParseAddress("0xb617fc2597f0eddfa07a5eb04c4c97006308517e"): "WorldOfV",
	thor.MustParseAddress("0x6a4fc1661e9d4ca8814be52d155e2f6353b2782a"): "WorldOfV",
	thor.MustParseAddress("0x42ac6537c8d4d7c5c8a18984e5ac8d32efd35d96"): "WorldOfV",
	thor.MustParseAddress("0x55ce12bb1af513c44f2135ca0b52f1eec27203de"): "WorldOfV",
	thor.MustParseAddress("0x00fbadb64941319d6cbdeaf7d356d8a73eb4ae5e"): "WorldOfV",
	thor.MustParseAddress("0x09985f776ae2c175106d8febf5360f6b380db582"): "WorldOfV",
	thor.MustParseAddress("0xe4538ddaaf68137a98448552c87f6910f1e3470d"): "WorldOfV",
	thor.MustParseAddress("0x8502a0bc9857a43fe7b5c700044fd6dce05619e4"): "WorldOfV",
	thor.MustParseAddress("0x4167d527340afa546bb88d5d83afb6272e48b40e"): "WorldOfV",
	thor.MustParseAddress("0x4c73b23fd7065becaf9a900cd475c19dee514d6e"): "WorldOfV",
	thor.MustParseAddress("0x1119e8f8d66e89d6b5e625dfdc45bab0f97de6a7"): "WorldOfV",
	thor.MustParseAddress("0x45c1438c4f913fdef947e244f559eede40c31931"): "WorldOfV",
	thor.MustParseAddress("0x4a9e867c1809f7ffdf4dc5aa870faf8be911a805"): "VeHash Staking",
	thor.MustParseAddress("0x8639b5f52f0093789f2e0f5bd2d6b9f58e8b0efb"): "Genesis Staking",
	thor.MustParseAddress("0x6b273fffae3b682ba9e62ada2a052ade9f2fc99c"): "WoV Genesis Cards Staking",
	thor.MustParseAddress("0xfabce34bb0b1174f1e0127d69bb705c60c35e587"): "Special Card Staking",
	thor.MustParseAddress("0x4ea989e8430c8d9c7fdd027f139be9067dc0483e"): "Inka Staking",
	thor.MustParseAddress("0x52df0438edeecd03119d613629e2500cd1986f9f"): "VeKongs 2 Staking",
	thor.MustParseAddress("0x20df04e8f8dacbca7c37f4f3233b6bbeac046bbe"): "VeKongs 2 Staking",
	thor.MustParseAddress("0x0f8f7df03010eb629ac717557c82dca963571b72"): "UW Staking",
	thor.MustParseAddress("0xe8a6314ae7813ba8c55568551aea223a657cffbe"): "UY Staking",
	thor.MustParseAddress("0x602d006ed9da40144df3a8f3fc707d29cb651ed5"): "VeHash Staking",
	thor.MustParseAddress("0xf4a6040c695b815b70713deff0793db1d33eb6b3"): "VeHash Staking",
	thor.MustParseAddress("0x489f014285a0c45e5c82013d5c3f3b3e1274889e"): "Shredderz",
	thor.MustParseAddress("0x916a1b8bda7239d0336727bc821bb0a362179dac"): "Shredderz",
	thor.MustParseAddress("0x8af71cd53ad800d792e29a8c4b64e24070eb9307"): "Ratverse",
	thor.MustParseAddress("0x9899a11ecc88b88117cd5195fe9db774b011de1d"): "Inka Staking",
	thor.MustParseAddress("0x2459a3649b5240ae8d851e42e64e694321e975b9"): "Psycho Beast Staking",
	thor.MustParseAddress("0x9394ba5521201c334583701716d61ff54880084c"): "Psycho Beast Staking",
	thor.MustParseAddress("0xc9e7060bc92959d9d2ac6b2121e4268a6b5ee651"): "Shamanic Oracles Staking",
	thor.MustParseAddress("0xf5e2d450881d8c3d466dd4ff4de8838c275bbc3c"): "Domination Staking",
	thor.MustParseAddress("0x8bb4a3ec153c0aa1ed1c51788b1d49dd68c79b30"): "Corgi Staking",
	thor.MustParseAddress("0xdd567517b958b6501a6388b4fa8cd2fd72ab72c4"): "VeKongs Staking",
	thor.MustParseAddress("0xe827213802fcaf4b776fa0adbde8da7fdd5f4b91"): "Nemesis Staking",
	thor.MustParseAddress("0xe804784f0344def17ace6c7569f57b48be27813a"): "VeCorgi Staking",
	thor.MustParseAddress("0x08c73b33115cafda73371a23a98ee354598a4abe"): "Dohrnii Staking",
	thor.MustParseAddress("0x732c69e4cb74279e1a9a6f31764d2c4668e1cba1"): "Dohrnii Staking",
	thor.MustParseAddress("0xcd88063e5bdc4416370557987fc7d15baa447b1d"): "Dohrnii Staking",
	thor.MustParseAddress("0xa2bae9d627A29aE6914c7D18afCcb27664d1b436"): "Dohrnii Staking",
	thor.MustParseAddress("0xe92fddd633008c1bca6e738725d2190cd46df4a1"): "VPunks VIP-181",
	thor.MustParseAddress("0x31c71f4cd01fddd940a46dafd72d3291f52040a4"): "VPunks NFT Auction",
	thor.MustParseAddress("0xdf71fd02fa65767b2b61a6346d7998e25987731a"): "VPunks VPU Staking",
	thor.MustParseAddress("0x3473c5282057d7beda96c1ce0fe708e890764009"): "ExoWorlds",
	thor.MustParseAddress("0xb2f12edde215e39186cc7653aeb551c8cf1f77e3"): "ExoWorlds",
	thor.MustParseAddress("0xf02c9669d502ec0c0bf88c4e68f44dfe25a0114b"): "ExoWorlds",
	thor.MustParseAddress("0xf99ea55a2bee3ee862d7cc61ad9e04fcb5991d5e"): "Black VeMarket - Marketplace",
	thor.MustParseAddress("0x87a4e53d6e65cdfada102818e9eab70d1230391f"): "Black VeMarket - Art NFTs",
	thor.MustParseAddress("0x6368f744862abc4d70d0714180d6d1902a86aa9b"): "Singapura",
	thor.MustParseAddress("0xf0ce85c23e39b74fa73424e36fcaad55420a36b9"): "Singapura",
	thor.MustParseAddress("0x766fa24fee03d6c20124280c23613e75c5601ee4"): "Singapura",
	thor.MustParseAddress("0x6c693abe7183e4f1c93c89721ce2c5bb06408eab"): "Warbands Staking",
	thor.MustParseAddress("0x3824f4288279089b22712afd60cb1d48e5b2c8cb"): "VeBudz Staking",
	thor.MustParseAddress("0xd56340abb721b7c89c6ca3835efc490dfd66f9ae"): "VeShawties NFTs",
	thor.MustParseAddress("0xc22d8ca65bb9ee4a8b64406f3b0405cc1ebeec4e"): "Singapura",
	thor.MustParseAddress("0x3a07dec3477c7e1df69c671df95471eefcf86175"): "Tribes NFTs",
	thor.MustParseAddress("0xa5e2ee50cb49ea4d0a3a520c15aa4cffaf5ea026"): "Gangster Gorillaz NFT",
	thor.MustParseAddress("0x2fd3d1e1a3f1e072c89d67301a86a5ba850ccd4e"): "Venonymous NFT",
	thor.MustParseAddress("0x34109fc2a649965eecd953d31802c67dcc183d57"): "UNION Distribution",
	thor.MustParseAddress("0x77fe6041fa5beb0172c9ab6014b4d8d5099f0a23"): "NO NERDS Tablet NFT",
	thor.MustParseAddress("0xd4310196a56c5193811ae418b8729d82b34abdcc"): "Dragon of Singapura Weapons NFT",
	thor.MustParseAddress("0x5a45edc6311017e6b12ebfb32c28a8d36ecf7686"): "Avery Dennison",
	thor.MustParseAddress("0xd948e6cf79ab34b716350db4aee33cf0031cf7a1"): "XGG Black Tea",
	thor.MustParseAddress("0x3805c62f463f34b2f913bb09115aaa9460794d7c"): "WOV Clock Auction Genesis",
	thor.MustParseAddress("0x2980f7a9bec00ee6ffee21e5fbac5e104578bf13"): "Wall of Vame",
	thor.MustParseAddress("0x49ba5e15899142ee2b192769e4abbc2cf13bfd6c"): "Voteos",
	thor.MustParseAddress("0x955b48a46698b2b8330d75dc88ae5b95cd7ff9f4"): "Yggdrasil",
	thor.MustParseAddress("0xd6fdbeb6d0fbc690dabd352cf93b2f8d782a46b5"): "Satoshi Dice",
	thor.MustParseAddress("0x57f4b5f456add260bf5193271f0bc7a5bed35d55"): "Arb Contract",
	thor.MustParseAddress("0xae51d373f105788a78208c7e3ca5167db1d33137"): "Arb Contract",
	thor.MustParseAddress("0xcfa5ec9df32a9c0a508aa8a6244e88d5ccc6b246"): "Arb Contract",
	thor.MustParseAddress("0xd850350d060ab629363386206df6486dcfa6ed68"): "Arb Contract",
	thor.MustParseAddress("0xbfc4649a50b8fc1fb75706eb28f8b6c5b3978012"): "Arb Contract",
	thor.MustParseAddress("0x9aa9f6472a5b415dbb7dd36dfb773e09b1369288"): "Vesitors NFT",
	thor.MustParseAddress("0x47cce813c986b4d982a192bf7d6831f4beaccbc0"): "YEET Crusaders NFT",
	thor.MustParseAddress("0xa2c82ad2841c23a49fc2ba1a23927d1fe835c7f9"): "Vales NFT",
	thor.MustParseAddress("0xd6b93818ac38c936f51538e5e7832d7127b79622"): "Metatun NFT",
	thor.MustParseAddress("0x910607db19dce5651da4e68950705d6e49bc01a5"): "INKA Empire NFT",
	thor.MustParseAddress("0x9d3837c3188f58ed579f98cfe922dccef25d6e95"): "INKA Empire Conquest NFT",
	thor.MustParseAddress("0x0b6f1e2220e7498111db0e56d972f93dd035da32"): "Gods of Olympus NFT",
	thor.MustParseAddress("0x4035dee4581deb866dc18c97696a5b78f393bcfe"): "Inka Boss Battling",
	thor.MustParseAddress("0xf639b215679d2411e0e2b25191dcc0ac38f1d798"): "Gods Boss Battling",
	thor.MustParseAddress("0x54a343c40a6ee31b27ca98e4c814d5bd02065b20"): "Gates of Hell1",
	thor.MustParseAddress("0xf606d79a1e962b1291d1dcf1f7226ecfbd8c63fc"): "Gates of Hell2",
	thor.MustParseAddress("0xbdf2b45bd428bba31c46b8d8d1f50615ee0e1416"): "3DAbles",
	thor.MustParseAddress("0x319f08fd7c97fe0010b1f6171debc8dce3738a0e"): "3DAbles",
	thor.MustParseAddress("0x3564c224dcbc63779c50dca573b2bccef72a985b"): "3DAbles",
	thor.MustParseAddress("0xafd5ff1c20d41a5f04eb3a82aa055ee5ecf9e331"): "3DAbles",
	thor.MustParseAddress("0x512547eEb6ceBf76210eA1e23588084AD3312C6e"): "3DAbles",
	thor.MustParseAddress("0x1f1d4b35302f9e0837b8ee34e3968023fde0122c"): "Paper Marketplace",
	thor.MustParseAddress("0x04edc606b0d60e843528422619c6d939be8a2fcf"): "Paper NFT",
	thor.MustParseAddress("0x122209e8f89cf2c1f126c3195419b9e4ee9c81ae"): "Paper Community Canvas 2022",
	thor.MustParseAddress("0x242035f42c59119b9a22d4270506c07fb792e55c"): "VeSea",
	thor.MustParseAddress("0xa76a73bcba9b4822f31e9827aaab7953c95a66ba"): "VeSea",
	thor.MustParseAddress("0xdab185ca52b70e087ec0990ad59c612c3d7aab14"): "VeSea",
	thor.MustParseAddress("0xdafca4a51ea97b3b5f21171a95dabf540894a55a"): "VeSea",
	thor.MustParseAddress("0x09212be7a37a066d4707d9afbe09536656aff89b"): "VeSea",
	thor.MustParseAddress("0x92f3bc40facd12504aeb64c86f06ee904acd37c5"): "VeSea",
	thor.MustParseAddress("0x85b2aae82762cd1232cff62f541c289bd349e3ff"): "VeSea",
	thor.MustParseAddress("0xabb89866a65efd45500ac9fe506179ebfb630c9b"): "VeSea",
	thor.MustParseAddress("0x29af120f3d84d8eb76e518dd43c6e408a579f6bf"): "VeSea",
	thor.MustParseAddress("0x148442103eeadfaf8cffd593db80dcdeadda71c9"): "VeSea",
	thor.MustParseAddress("0x588f2b0d4cbea48deb34c3d401cb995046edda81"): "VeSea",
	thor.MustParseAddress("0x997c61cd02b5f2c8826ebcaf26080c650cabdda2"): "VeSea",
	thor.MustParseAddress("0x9992501f1ef16d4b900e9d316cf468959b8f9bcd"): "VeSea",
	thor.MustParseAddress("0x9932690b32c4c0cd4d86a53eca948d33b1556ae7"): "VeSea",
	thor.MustParseAddress("0xc35d04f8783f85ede2f329eed3c1e8b036223a06"): "VeSea",
	thor.MustParseAddress("0x46db08ca72b205a8f9e746a9e33a30d2f379216b"): "VeSea",
	thor.MustParseAddress("0x6f0d98490d57a0b2b3f44342edb6a5bb30012e1c"): "VeSea",
	thor.MustParseAddress("0x8b55d319b6cae4b9fd0b4517f29cf4a100818e38"): "VeSea",
	thor.MustParseAddress("0xffcc1c4492c3b49825712e9a8909e4fcebfe6c02"): "VeSea",
	thor.MustParseAddress("0xb12d1d640f56173ef3a47e5e1a1fde96ba96ce14"): "VeSea",
	thor.MustParseAddress("0x60deca6baceb6258c8d718f9987acb17176f7f24"): "VeSea",
	thor.MustParseAddress("0x436f0a9b45e85eb2f749aa67d3393c649ef4dff2"): "VeSea",
	thor.MustParseAddress("0x01c10830feef88258e7a1ca998009ac19f7f087e"): "VeSea",
	thor.MustParseAddress("0x6aa982158617d53c37f65d43eb9a156449adfff3"): "VeSea",
	thor.MustParseAddress("0x14c7d5357da8a8ed7a3983bc5ffd249fee63192d"): "VeSea",
	thor.MustParseAddress("0x5452c80cdfd31e175f62f6197e65adaf73ec84df"): "VeSea",
	thor.MustParseAddress("0x88d7e38af5bdb6e65a045c79c9ce70ed06e6569b"): "VeSea",
	thor.MustParseAddress("0x1f173256c08e557d0791bc6ef2ac1b1099f57ed5"): "VeSea",
	thor.MustParseAddress("0xc2de1fbb24d918a68923cfb24cc541aea7a49450"): "VeSea",
	thor.MustParseAddress("0x15e2f18feade6ccb990956050bf0c2990445cace"): "VeSea",
	thor.MustParseAddress("0x207577649f08c87de98e9981712fc9aece07df79"): "VeSea",
	thor.MustParseAddress("0x0403745444204d1a0218cecbfe70b2ea42d654a6"): "VeSea",
	thor.MustParseAddress("0x7f2445324b9aaaede83a0bde18a1d55caea8c18f"): "VeSea",
	thor.MustParseAddress("0xbcfc59dcc2a0977ac1e9b465566ad071e5ec06aa"): "VeSea",
	thor.MustParseAddress("0x313d1fff2664a2df5a12e99c8c57e50efa715d73"): "VeSea",
	thor.MustParseAddress("0x6354b35c510cae41cd45b568087bf767756b3589"): "VeSea",
	thor.MustParseAddress("0x64e8f785c27fb1f55f6ef38787853d3a1d0cde02"): "VeSea",
	thor.MustParseAddress("0x4e4faebf70e7c01bcd39adbfaa247f081819919a"): "VeSea",
	thor.MustParseAddress("0x3427e769ae440ae8e18b77f49cc2d6a39e57f047"): "VeSea",
	thor.MustParseAddress("0x78d4ba28c151501fa3f68927ea96304cab89b6f0"): "VeSea",
	thor.MustParseAddress("0xd393c0dcccae49248862b462404b63a8546a888a"): "VeSea",
	thor.MustParseAddress("0x850a2457975fd411f03a513c6f94cd7d378e7ec1"): "VeSea",
	thor.MustParseAddress("0xfd5e344798ceb51afd910fafae9768e4d093a725"): "VeSea",
	thor.MustParseAddress("0x055faf8495067864bcb8e8e3edadc506d98af5b3"): "VeSea",
	thor.MustParseAddress("0x2571978545672fe7e4cced7409bdd0a57bc3c3d2"): "VeSea",
	thor.MustParseAddress("0x4e9eb6f6e04464eee33ae04bf430e20529482e60"): "VeSea",
	thor.MustParseAddress("0xf60b9aa0ab640c23b6dd6456a15d041a5f3a5f5e"): "VeSea",
	thor.MustParseAddress("0x875d36b9760ffe7ce366d3ff700c1ad98bdee990"): "VeSea",
	thor.MustParseAddress("0x0c447c4311afecf8c14108fa962442444a8d3b06"): "VeSea",
	thor.MustParseAddress("0x1d971ac972f671c19d1be00e4fbf3118d3861851"): "VeSea",
	thor.MustParseAddress("0x3759080b28604fd2851c215da71539bd8d5242ef"): "VeSea",
	thor.MustParseAddress("0x3fdf191152684b417f4a55264158c2eab97a81b3"): "VeSea",
	thor.MustParseAddress("0xf647e7b4fe7e0dc7ceddd038c6c004cc53163ca9"): "VeSea",
	thor.MustParseAddress("0x499be5332bfba0761650ae55b8d9c8443458f219"): "VeSea",
	thor.MustParseAddress("0x862b1cb1c75ca2e2529110a9d43564bd5cd83828"): "VeSea",
	thor.MustParseAddress("0xf4d82631be350c37d92ee816c2bd4d5adf9e6493"): "VeSea",
	thor.MustParseAddress("0xcb831e98a3ae13b4a124ef8d0088edfee3de0c89"): "VeSea",
	thor.MustParseAddress("0xb757fc0906f08714315d2abd4b4f077521a21e34"): "VeSea",
	thor.MustParseAddress("0xa19e999fce74ec6e9d8ce1380b4692e63e6c7cb1"): "VeSea",
	thor.MustParseAddress("0x8d831724414739846045e7bc07522058ff5f67d8"): "VeSea",
	thor.MustParseAddress("0xbcbf39013da096c97f0dc913f7ac1cdc42b9a721"): "VeSea",
	thor.MustParseAddress("0x017d182c60e4f3c469156208b9f30a7fb80db214"): "VeSea",
	thor.MustParseAddress("0xc17d84d2d19b45653abefed0b9678fcdbfc1b0b0"): "VeSea",
	thor.MustParseAddress("0x428f6e43adc7649fe79f3a4341f0780cab059ffb"): "VeSea",
	thor.MustParseAddress("0x2e53f17aa7dcbd00ec3eb80388f509faf84edafa"): "VeSea",
	thor.MustParseAddress("0x56b57cc14e10aae2769a9fb660d0d0c0d41a6aca"): "VeSea",
	thor.MustParseAddress("0x0ce0c940d11fbdd73561901dbcdef84e73a511b9"): "VeSea",
	thor.MustParseAddress("0x4acacfeaaaba51c488d429106184591856356b52"): "VeSea",
	thor.MustParseAddress("0x13e13a662bf2a085bbab01f9b1f5c3319f434ed2"): "VeSea",
	thor.MustParseAddress("0xcb99479e30136d86f9d8a8e9a79a4ecc75e36066"): "VeSea",
	thor.MustParseAddress("0x4eb966763294a77be85d1a1a56b7e15f59f45dbe"): "VeSea",
	thor.MustParseAddress("0x4d4664aed6f645fb3defbbd668b2a4842c029187"): "VeSea",
	thor.MustParseAddress("0xc766ddd21f14862ef426f15bfb28573fdad8bc51"): "VeSea",
	thor.MustParseAddress("0xe7af95411f611fbaf39bc91c17ca6179661a032e"): "VeSea",
	thor.MustParseAddress("0x7b927025cd0e645c28924e2726ecc7372615df46"): "VeSea",
	thor.MustParseAddress("0xd9c10931402e9619135481969e62925520bcceeb"): "VeSea",
	thor.MustParseAddress("0x8c41b27504c2ea059312c55122d07149a3363c31"): "VeSea",
	thor.MustParseAddress("0xe4be710b7553602a37126bd2bade15df18c957ff"): "VeSea",
	thor.MustParseAddress("0xb68f43cf91a2c9fa3f8ab369cb2fb23511eb7fb7"): "VeSea",
	thor.MustParseAddress("0x8c810f79900d2b69f7043c7ff447f2eb3084606a"): "VeSea",
	thor.MustParseAddress("0xfb3b2f8b4f8aae9e7a24ba0bcbb6a49d344f2ef3"): "VeSea",
	thor.MustParseAddress("0x6fd65c8ecafebbb505ab74f2e27025058bddc75d"): "VeSea",
	thor.MustParseAddress("0x2f0586faa4b51a678cf5d0f27ce414f3f6d08517"): "VeSea",
	thor.MustParseAddress("0xdce5a78fe9cbba559c73a83ee40891b8a09516c2"): "VeSea",
	thor.MustParseAddress("0x2c59dfa1d8d9a1f17855d1db0d071662aebe16be"): "VeSea",
	thor.MustParseAddress("0x24520e0943b57ce3134238917309b903b181832a"): "VeSea",
	thor.MustParseAddress("0x7d9fd924b15efe9e82093d51af9bcd875ad57428"): "VeSea",
	thor.MustParseAddress("0xb1e19aeaa5da5aba4b5591e548b5b6505c08909e"): "VeSea",
	thor.MustParseAddress("0xafe1d7d4ac69c2f31dfde6dd31f3df955ddec2a3"): "Gresini Card",
	thor.MustParseAddress("0x83f158bbc757ce2ff61ff5ff119eca7ad687a306"): "vtho.exchange",
}

var fungibleTokens = map[thor.Address]string{
	thor.MustParseAddress("0x0000000000000000000000000000456e65726779"): "VeThor",
	thor.MustParseAddress("0x89827f7bb951fd8a56f8ef13c5bfee38522f2e1f"): "Plair",
	thor.MustParseAddress("0x5db3c8a942333f6468176a870db36eef120a34dc"): "Safe Haven Token",
	thor.MustParseAddress("0xf8e1faa0367298b55f57ed17f7a2ff3f5f1d1628"): "Eight Hours Token",
	thor.MustParseAddress("0x1b8ec6c2a45cca481da6f243df0d7a5744afc1f8"): "Decent.bet Token",
	thor.MustParseAddress("0xa94a33f776073423e163088a5078feac31373990"): "TicTalk Token",
	thor.MustParseAddress("0x0ce6661b4ba86a0ea7ca2bd86a0de87b0b860f14"): "OceanEx Token",
	thor.MustParseAddress("0x540768b909782c430cc321192e6c2322f77494ec"): "SneakerCoin",
	thor.MustParseAddress("0x46209d5e5a49c1d403f4ee3a0a88c3a27e29e58d"): "JUR",
	thor.MustParseAddress("0xf9fc8681bec2c9f35d0dd2461d035e62d643659b"): "Aqua Diamond Token",
	thor.MustParseAddress("0xae4c53b120cba91a44832f875107cbc8fbee185c"): "Yeet Coin",
	thor.MustParseAddress("0xacc280010b2ee0efc770bce34774376656d8ce14"): "HackenAI",
	thor.MustParseAddress("0x1b44a9718e12031530604137f854160759677192"): "Madini",
	thor.MustParseAddress("0x67fd63f6068962937ec81ab3ae3bf9871e524fc9"): "VEED",
	thor.MustParseAddress("0xb0821559723db89e0bd14fee81e13a3aae007e65"): "VPunks Token",
	thor.MustParseAddress("0x99763494a7b545f983ee9fe02a3b5441c7ef1396"): "Mad Viking Games",
	thor.MustParseAddress("0x170f4ba8e7acf6510f55db26047c83d13498af8a"): "WorldOfV",
	thor.MustParseAddress("0x28c61940bdcf5a67158d00657e8c3989e112eb38"): "GEMS",
	thor.MustParseAddress("0x0bd802635eb9ceb3fcbe60470d2857b86841aab6"): "Vexchange",
	thor.MustParseAddress("0x4e17357053da4b473e2daa2c65c2c949545724b8"): "VeUSD",
	thor.MustParseAddress("0x45429a2255e7248e57fce99e7239aed3f84b7a53"): "Veiled VET",
	thor.MustParseAddress("0x8e57aadf0992afcc41f7843656c6c7129f738f7b"): "Dohrnii",
	thor.MustParseAddress("0x34109fc2a649965eecd953d31802c67dcc183d57"): "UNION",
	thor.MustParseAddress("0xb9c146507b77500a5cedfcf468da57ba46143e06"): "VeStacks",
	thor.MustParseAddress("0x2f10726b240d7efb08671f4d5f0a442db6f29416"): "Paper Token",
	thor.MustParseAddress("0x107a0b0faeb58c1fdef97f37f50e319833ad1b94"): "Dragon Coin",
	thor.MustParseAddress("0x23368c20c16f64ecbb30164a08666867be22f216"): "VeSea",
	thor.MustParseAddress("0xf01069227b814f425bad4ba70ca30580f2297ae8"): "BananaCoin",
	thor.MustParseAddress("0xff3bc357600885aaa97506ea6e24fb21aba88fbd"): "GOLD Coin",
	thor.MustParseAddress("0xe5bb68318120828fd1159bf73d0e3a823043efc8"): "LEGACY TOKEN",
	thor.MustParseAddress("0xa4f95b1f1c9f4cf984b0a003c4303e8ea86302f6"): "VFoxToken",
	thor.MustParseAddress("0xd5bd1b64cc9dafbfd58abd1d24a51f745ba64712"): "FreeCoffeeWithSunny",
	thor.MustParseAddress("0x02de9e580b51907a471d78ccfb2e8abe4c6b7515"): "MyVeChain",
	thor.MustParseAddress("0xc3fd50a056dc4025875fa164ced1524c93053f29"): "MVA Token",
	thor.MustParseAddress("0xb27a1fb87935b85cdaa2e16468247278c74c5ec7"): "Squirtle Squad",
	thor.MustParseAddress("0x8fcddbb322b18d8bdaec9243e9f4c6eb8901e566"): "ThreeDAbleToken",
	thor.MustParseAddress("0x99ae6b435d37995befb749670c1fb7c377fbb6d1"): "LION",
	thor.MustParseAddress("0x9af004570f2a301d99f2ce4554e564951ee48e3c"): "Sh*tCoin",
	thor.MustParseAddress("0x7ae288b7224ad8740b2d4fc2b2c8a2392caea3c6"): "Black Ve Coin",
	thor.MustParseAddress("0x094042f9719cd6736fa3bd45b605b1b2a23abdec"): "Vyvo US Dollar",
	thor.MustParseAddress("0x65c542ad413dd406d7ae5e47f61fbda027ce7983"): "VSC",
	thor.MustParseAddress("0x3cb62f48dbdc4f7627f37f027811565d292a1001"): "Rainbow",
	thor.MustParseAddress("0x64a8dea68772d478240dd6d3080a8e7f288a720f"): "MILK",
	thor.MustParseAddress("0xb0d5b68a96fab5f3047f6de6f9377a460db7e528"): "PLUG",
	thor.MustParseAddress("0x47921404147046177b8038720ac2d0b2776ee5bf"): "EXO Token",
	thor.MustParseAddress("0xab644843eeab886a5ed3ea22066c6ee5190cfb81"): "Uno",
	thor.MustParseAddress("0x9fbf641fd303bfb294fa9d5393962806644825b4"): "GCRED Token",
	thor.MustParseAddress("0x70a647c84ac1f492efd302e1af6d1ab8d20223a0"): "Vemecoin",
	thor.MustParseAddress("0x6e0b217380b45fd9992bafa91c08a92455ec647a"): "BANGZ",
	thor.MustParseAddress("0x8b8ada6679963e39cb8edd9198decc367790187d"): "Amphi",
	thor.MustParseAddress("0x2c28d59e1424f878cb655d74c297fcb685c22be6"): "Cup of Joe",
	thor.MustParseAddress("0x4a4bd03b67d6aae921b4bb54835079e91d81a3a9"): "PEPE",
	thor.MustParseAddress("0x6924252d44bb2f7592285d3014b1eb291c044f03"): "VienerDog",
	thor.MustParseAddress("0x6a4aa92f8c45242be02c54b433c63b5f525ec658"): "BigVEnergy",
	thor.MustParseAddress("0x4b85757bcf693f742003f2d5529cdc1672392f16"): "Slayers Guild",
	thor.MustParseAddress("0x2f2220139e46bcc98273ecca2ded7bf56373b6cf"): "cool",
	thor.MustParseAddress("0x5ef79995fe8a89e0812330e4378eb2660cede699"): "B3TR",
	thor.MustParseAddress("0x76ca782b59c74d088c7d2cce2f211bc00836c602"): "VOT3",
	thor.MustParseAddress("0x867bca2f3f187bb7bfb900ebcd3155746537a9a9"): "Hawk Tuah",
	thor.MustParseAddress("0x420dfe6b7bc605ce61e9839c8c0e745870a6cde0"): "veB3TR",
	thor.MustParseAddress("0x27404060ea6939ff9e3598a3d0409ff11c9c6247"): "Ratverse Coin",
	thor.MustParseAddress("0x8ce14e9906c64f8e17fa27eb51d3db1df3da2c16"): "LLAMACOIN",
	thor.MustParseAddress("0x84b0caf6436aace4e21d10f126963fdd53ac31ea"): "Sassafras",
}

var knownAddresses = map[thor.Address]string{
	thor.MustParseAddress("0xa4adafaef9ec07bc4dc6de146934c7119341ee25"): "Binance",
	thor.MustParseAddress("0xd0d9cd5aa98efcaeee2e065ddb8538fa977bc8eb"): "Binance Cold",
	thor.MustParseAddress("0x1263c741069eda8056534661256079d485e111eb"): "Binance Warm",
	thor.MustParseAddress("0x44bc93a8d3cefa5a6721723a2f8d2e4f7d480ba0"): "Binance (In/Out Wallet)",
	thor.MustParseAddress("0xd7dd13a54755cb68859eec0cac24144aafb8c881"): "Huobi",
	thor.MustParseAddress("0xfe64e37dfc7d64743d9351260fa99073c840452b"): "Binance US",
	thor.MustParseAddress("0xb73554767983dc5aaeac2b948e407f57e8e9dea1"): "Bittrex",
	thor.MustParseAddress("0xcaca08a5053604bb9e9715ed78102dbb392f21ee"): "Bittrex",
	thor.MustParseAddress("0xe13322e57366a4dff3a3a32b33355ff2bd2c4dbd"): "Bitvavo",
	thor.MustParseAddress("0x6c61974835b4b8fcde83f74e7e5abc470662b3bc"): "Bitvavo",
	thor.MustParseAddress("0xfa4b22b75ae0900e88b640175ae0cd1896ec251a"): "HitBTC",
	thor.MustParseAddress("0x48728dcafa1afaeb79c6d7249b6b4a3868ce5c12"): "OceanEx",
	thor.MustParseAddress("0x15bccf377f1a9bbd0cd8e24d031c9451326f29a0"): "OceanEx",
	thor.MustParseAddress("0xd96ae915d6e28640c373640fd57fad8022c53965"): "OceanEx Custodian",
	thor.MustParseAddress("0x8e9e08eed34cf829158fab863f99c0225d31e123"): "OceanEx",
	thor.MustParseAddress("0x8979cdda17e1afd32c73b65145484abe03f46725"): "OceanEx",
	thor.MustParseAddress("0xdd8a9cca3876343f666a81833d2f3a3863a11159"): "OceanEx",
	thor.MustParseAddress("0x254afc2490d83b1a56fe621cd708f89456472d87"): "OceanEx",
	thor.MustParseAddress("0x9d30a969297cb008e2d777135155e89a35b5dff4"): "OceanEx",
	thor.MustParseAddress("0x589f83e66272d3d783c06dd6a66cb3b3549e5453"): "OceanEx",
	thor.MustParseAddress("0x0ce0000000000000000000000000000000000000"): "OceanEx OCE Burn",
	thor.MustParseAddress("0x45685fb104772e9b6421202ed2d7309d7a6dc32d"): "OceanEx",
	thor.MustParseAddress("0x4e28e3f74c5974c8d18611d5323ae8a1344c3e73"): "OceanEx",
	thor.MustParseAddress("0xe6f432d44de32f22a0b6c743e448e4421653393e"): "OceanEx",
	thor.MustParseAddress("0xee12ecae8a1fea9d4279640bb87072c9db76198d"): "OceanEx",
	thor.MustParseAddress("0x9037aa63d3860b708a31df9d372709322d6a2911"): "KuCoin",
	thor.MustParseAddress("0xda4d4530d856623dc820427f71e9aa601075f02d"): "KuCoin",
	thor.MustParseAddress("0x832fbebb667acc410b434ecfebcbb841cb7c864c"): "Upbit Cold",
	thor.MustParseAddress("0xea09214d6509aa4681ba469dbccfbc89c525c5b7"): "Upbit",
	thor.MustParseAddress("0x4703582c50fcd1b65fab573bd02c1c53bbe05f92"): "Crypto.com",
	thor.MustParseAddress("0x511513c6e60c347402b57f3b13c3a8e994188cab"): "Crypto.com Cold",
	thor.MustParseAddress("0x1c4b70a3968436b9a0a9cf5205c787eb81bb558c"): "Gate.io Cold",
	thor.MustParseAddress("0x0f53ec6bbd2b6712c07d8880e5c8f08753d0d5d5"): "BigONE",
	thor.MustParseAddress("0x0365289be54c921798533cbe56934e0442bafccf"): "Bithumb",
	thor.MustParseAddress("0x86158838e088da2a80a541fe0ec96ea4800bbc5e"): "Bithumb",
	thor.MustParseAddress("0x003bfdd8117f9388f82a1101a2c6f4745803c350"): "Bithumb",
	thor.MustParseAddress("0x3bd4fd485301490e2482e501522a7f6bd8f16ea5"): "Bithumb",
	thor.MustParseAddress("0xe401984ab34bae9f6c9128e50b57e7988ba815c7"): "Bitfinex",
	thor.MustParseAddress("0x01d1aec89781056ae69ee7381e8e237b5c0b6a64"): "Bitrue",
	thor.MustParseAddress("0x284b9e222c461e32c2fa17053e2ea207041cffa0"): "OceanEx",
	thor.MustParseAddress("0x0d0707963952f2fba59dd06f2b425ace40b492fe"): "Gate.io",
	thor.MustParseAddress("0x9a107a75cff525b033a3e53cadafe3d193b570ec"): "MXC",
	thor.MustParseAddress("0xfa02e5f286f635df9378395f4be54647e73a66a0"): "Lbank",
	thor.MustParseAddress("0xfe3baf051e7957393d4bedd14447851946163a74"): "CoinEx",
	thor.MustParseAddress("0xfbc6013ee8891ddc86d850fb8bac99b4d14c8405"): "Coinsuper",
	thor.MustParseAddress("0xce6b1252b32a34fc4013f096cdf90643fb5d23ba"): "ChangeNOW",
	thor.MustParseAddress("0x2c0971b1dccf819f38dcf2d3b55d7219f2b817d0"): "BitMax",
	thor.MustParseAddress("0x68e29026cf3a6b6e3fce24e9fcc2865f39c884d7"): "LaToken",
	thor.MustParseAddress("0x21d54bcf0142c5a3286a7ec7449ad9c4fd5a68f2"): "RightBTC PLA",
	thor.MustParseAddress("0x6852b4161b8bc237db1810700a22bccae370778c"): "Foundation",
	thor.MustParseAddress("0x137053dfbe6c0a43f915ad2efefefdcc2708e975"): "Foundation",
	thor.MustParseAddress("0x29eca91ce3f715c9ba9e87ec1395dca7d1ce9e9e"): "Investor",
	thor.MustParseAddress("0x94bef24751937163e026c63f6c8d833e60c8bf8c"): "Safe Haven ICO",
	thor.MustParseAddress("0xd021980f6bdd2e62ec1a15d3e606e9106dec9544"): "8Hours ICO",
	thor.MustParseAddress("0xa6386e9d2518773f45a941b856b33976ed71c671"): "8Hours ICO",
	thor.MustParseAddress("0xbd916eddd1fc8a9e496ba6bae4355f09bcc44961"): "VTHO rewards",
	thor.MustParseAddress("0xd07c2ee31e98d71aca35aeb29e8a1062fc084cfc"): "HackenAI",
	thor.MustParseAddress("0x1466e8f38b89086ea0155216ab51dd3d1e8f571a"): "HackenAI",
	thor.MustParseAddress("0x15837e1f91860d5ffdd5f3b93b8c946340111cbe"): "HackenAI",
	thor.MustParseAddress("0x1421cb00f42b838e90234b28d05fd701fe1c71dd"): "HackenAI",
	thor.MustParseAddress("0xf050cc342f573155917bb0839c5d823ec2703746"): "HackenAI",
	thor.MustParseAddress("0x5d7fe18beff1c4f16115cb8cfcd87442a89d9278"): "JUR Reserve",
	thor.MustParseAddress("0xf346f1ab880d5b2cd0333bf69c280a732fa4a1c4"): "JUR Team",
	thor.MustParseAddress("0xc01b26cd4b9525ad1b67a54fad53a8bff91ae01d"): "JUR",
	thor.MustParseAddress("0x17b6254c7324438b469a01ce80b67dd7c4d5eef8"): "Plair ICO",
	thor.MustParseAddress("0x48e8dace6a1976d4912f8b5dcc3f45651c3d4b73"): "Safe Haven Boost",
	thor.MustParseAddress("0x27942b0d71919c4aa81b7ae6ba951150faef5ae6"): "VIP-191 Sponsor",
	thor.MustParseAddress("0xb0c224a96655ba8d51f35f98068f5fc12f930946"): "Coinbase",
	thor.MustParseAddress("0xda894a5dc94d64efd5b518a7bd567740c4617fcc"): "Coinbase",
	thor.MustParseAddress("0x65d0dc1b845a9eb4baabbf28e3c5b4de2e19e51c"): "Coinbase",
	thor.MustParseAddress("0xff5ba88a17b2e16d23ff6647e9052e937acb1406"): "Coinbase",
	thor.MustParseAddress("0xbf6ba25d7d3e28153549196cd6361fca5e40d635"): "Coinbase",
	thor.MustParseAddress("0xf9cb626c6f611ae0255cbb452ae70a9c68fb6d89"): "Coinbase",
	thor.MustParseAddress("0xd1815e7a26609a0c07233582e7309c5ae8b25b6f"): "Coinbase",
}

var nftContracts = map[thor.Address]string{
	thor.MustParseAddress("0xb81e9c5f9644dec9e5e3cac86b4461a222072302"): "Vechain Node Token",
	thor.MustParseAddress("0x01c10830feef88258e7a1ca998009ac19f7f087e"): "VeSkullz",
	thor.MustParseAddress("0x04edc606b0d60e843528422619c6d939be8a2fcf"): "NFT Paper Project",
	thor.MustParseAddress("0x0504448a67074e2977723b5d19a3467c5dbabb82"): "Smuzzies Phantom VeGhosts",
	thor.MustParseAddress("0x055faf8495067864bcb8e8e3edadc506d98af5b3"): "Metaversials Alter-egos",
	thor.MustParseAddress("0x09985f776ae2c175106d8febf5360f6b380db582"): "Psycho Beasts - Nemesis",
	thor.MustParseAddress("0x0b6f1e2220e7498111db0e56d972f93dd035da32"): "The Gods of Olympus",
	thor.MustParseAddress("0x13e13a662bf2a085bbab01f9b1f5c3319f434ed2"): "Kickback Koalas 3D",
	thor.MustParseAddress("0x1427d0d3233e39a9703eecdca251da771e9971a7"): "Ratverse Genesis",
	thor.MustParseAddress("0x148442103eeadfaf8cffd593db80dcdeadda71c9"): "VeKings",
	thor.MustParseAddress("0x14c7d5357da8a8ed7a3983bc5ffd249fee63192d"): "VeNerds",
	thor.MustParseAddress("0x15e2f18feade6ccb990956050bf0c2990445cace"): "VeGnomes",
	thor.MustParseAddress("0x167f6cc1e67a615b51b5a2deaba6b9feca7069df"): "Shredderz",
	thor.MustParseAddress("0x1d971ac972f671c19d1be00e4fbf3118d3861851"): "Forest Nation - Guardians",
	thor.MustParseAddress("0x1f173256c08e557d0791bc6ef2ac1b1099f57ed5"): "veLoot",
	thor.MustParseAddress("0x207577649f08c87de98e9981712fc9aece07df79"): "UniⓋerse - The Expanse",
	thor.MustParseAddress("0x2571978545672fe7e4cced7409bdd0a57bc3c3d2"): "Doppelganger",
	thor.MustParseAddress("0x2e53f17aa7dcbd00ec3eb80388f509faf84edafa"): "Bomber Squad Coins",
	thor.MustParseAddress("0x2f478c2e68e3385e632c625f0ee12d5a3a775e68"): "Union Membership",
	thor.MustParseAddress("0x2fd3d1e1a3f1e072c89d67301a86a5ba850ccd4e"): "Venonymous",
	thor.MustParseAddress("0x313d1fff2664a2df5a12e99c8c57e50efa715d73"): "Metaversials",
	thor.MustParseAddress("0x319f08fd7c97fe0010b1f6171debc8dce3738a0e"): "Smuzzies",
	thor.MustParseAddress("0x3427e769ae440ae8e18b77f49cc2d6a39e57f047"): "Frost Giant VeKings",
	thor.MustParseAddress("0x3473c5282057d7beda96c1ce0fe708e890764009"): "ExoWorlds",
	thor.MustParseAddress("0x3759080b28604fd2851c215da71539bd8d5242ef"): "Kickback Koalas",
	thor.MustParseAddress("0x3b9521745ae47418c3c636ec1e76f135cdc961fc"): "VeKongs 2.0",
	thor.MustParseAddress("0x3dbba9ad9e33bd188eee8aa2d5c0e7b9894c6209"): "Vemons",
	thor.MustParseAddress("0x3fdf191152684b417f4a55264158c2eab97a81b3"): "VFox Alliance - ORIGINS",
	thor.MustParseAddress("0x41a03b04725c20f3902c67ee7416e5df4266df45"): "Bored Flamingo Flying Club Ladies",
	thor.MustParseAddress("0x428f6e43adc7649fe79f3a4341f0780cab059ffb"): "Shreddiez",
	thor.MustParseAddress("0x436f0a9b45e85eb2f749aa67d3393c649ef4dff2"): "AstroVets",
	thor.MustParseAddress("0x4523048daa77b61766add0bebf7f83e05f173d8f"): "Bannerboi",
	thor.MustParseAddress("0x46db08ca72b205a8f9e746a9e33a30d2f379216b"): "Vumanoids",
	thor.MustParseAddress("0x4786bfd13641507b4cd8b492c362c13bcf35ee71"): "Ratverse X",
	thor.MustParseAddress("0x47cce813c986b4d982a192bf7d6831f4beaccbc0"): "Yeet Crusaders",
	thor.MustParseAddress("0x499be5332bfba0761650ae55b8d9c8443458f219"): "VVarriors",
	thor.MustParseAddress("0x4a6b084243762dc219480edc5cfa0d88298bb707"): "Vogies",
	thor.MustParseAddress("0x4acacfeaaaba51c488d429106184591856356b52"): "New Pigs Order - Sows",
	thor.MustParseAddress("0x4d4664aed6f645fb3defbbd668b2a4842c029187"): "Goatz Club",
	thor.MustParseAddress("0x4e4faebf70e7c01bcd39adbfaa247f081819919a"): "VeeParrots",
	thor.MustParseAddress("0x4eb966763294a77be85d1a1a56b7e15f59f45dbe"): "Stardust Spectres - Spectres",
	thor.MustParseAddress("0x5452c80cdfd31e175f62f6197e65adaf73ec84df"): "VeNerds Airdrops",
	thor.MustParseAddress("0x55ce12bb1af513c44f2135ca0b52f1eec27203de"): "Mad Ⓥ-Apes - Land Plots",
	thor.MustParseAddress("0x56b57cc14e10aae2769a9fb660d0d0c0d41a6aca"): "Funky Salamanders",
	thor.MustParseAddress("0x588f2b0d4cbea48deb34c3d401cb995046edda81"): "VeGhosts",
	thor.MustParseAddress("0x60deca6baceb6258c8d718f9987acb17176f7f24"): "Universe",
	thor.MustParseAddress("0x64e8f785c27fb1f55f6ef38787853d3a1d0cde02"): "VeAbstract",
	thor.MustParseAddress("0x6a4fc1661e9d4ca8814be52d155e2f6353b2782a"): "DoS Elements",
	thor.MustParseAddress("0x6aa982158617d53c37f65d43eb9a156449adfff3"): "Warbands",
	thor.MustParseAddress("0x6f0d98490d57a0b2b3f44342edb6a5bb30012e1c"): "Undead VeKings",
	thor.MustParseAddress("0x73f32592df5c0da73d56f34669d4ae28ae1afd9e"): "Mad Ⓥ-Apes  Phoenix",
	thor.MustParseAddress("0x77fe6041fa5beb0172c9ab6014b4d8d5099f0a23"): "No Nerds Inc",
	thor.MustParseAddress("0x78d4ba28c151501fa3f68927ea96304cab89b6f0"): "VFoxes",
	thor.MustParseAddress("0x7957c7685879f45db2642d5705b72bc9ad2d0899"): "VFox Alliance - Geckos",
	thor.MustParseAddress("0x7b927025cd0e645c28924e2726ecc7372615df46"): "VVAR DOGS - Passes",
	thor.MustParseAddress("0x7de7c8d9c4cd487b73df58b2a6b9302446f4e116"): "Shamanic Companions",
	thor.MustParseAddress("0x7de983348e6b4bf215a08e4f21ddfe75a39ec9dc"): "Kodama Klub",
	thor.MustParseAddress("0x7f2445324b9aaaede83a0bde18a1d55caea8c18f"): "Guardians",
	thor.MustParseAddress("0x81fc139f676736c96cbcba40b5e5229baec02732"): "Vlippos",
	thor.MustParseAddress("0x850a2457975fd411f03a513c6f94cd7d378e7ec1"): "Ukraine Relief",
	thor.MustParseAddress("0x862b1cb1c75ca2e2529110a9d43564bd5cd83828"): "Mino Mob Elixirs",
	thor.MustParseAddress("0x875d36b9760ffe7ce366d3ff700c1ad98bdee990"): "Phantom VeGhosts",
	thor.MustParseAddress("0x88d7e38af5bdb6e65a045c79c9ce70ed06e6569b"): "New Pigs Order",
	thor.MustParseAddress("0x8b55d319b6cae4b9fd0b4517f29cf4a100818e38"): "Sacrificed VeKings",
	thor.MustParseAddress("0x8c41b27504c2ea059312c55122d07149a3363c31"): "VeKing Raiders",
	thor.MustParseAddress("0x8d831724414739846045e7bc07522058ff5f67d8"): "Stardust Spectres",
	thor.MustParseAddress("0x90dc145867f10ec90d4f4432431896ca8f8be0e3"): "Tamed Teens",
	thor.MustParseAddress("0x910607db19dce5651da4e68950705d6e49bc01a5"): "Inka Empire",
	thor.MustParseAddress("0x97f7c8d476183b69f18f810a18baf3f79994a267"): "VeThugs",
	thor.MustParseAddress("0x9932690b32c4c0cd4d86a53eca948d33b1556ae7"): "VeKongs",
	thor.MustParseAddress("0x9992501f1ef16d4b900e9d316cf468959b8f9bcd"): "Veysarum",
	thor.MustParseAddress("0x9c872e8420ec38f404402bea8f8f86d5d2c17782"): "Mad V-Apes Fusion G2",
	thor.MustParseAddress("0x9d3837c3188f58ed579f98cfe922dccef25d6e95"): "Inka Empire - The Conquest",
	thor.MustParseAddress("0xa2c82ad2841c23a49fc2ba1a23927d1fe835c7f9"): "Vales",
	thor.MustParseAddress("0xa4bf5a32d0f1d1655eec3297023fd2136bd760a2"): "Corgi Gang",
	thor.MustParseAddress("0xa5e2ee50cb49ea4d0a3a520c15aa4cffaf5ea026"): "Gangster Gorillaz",
	thor.MustParseAddress("0xa723a21419181a9ddee6e3981d5854a05c9e90e1"): "Bored Flamingo Flying Club",
	thor.MustParseAddress("0xb12d1d640f56173ef3a47e5e1a1fde96ba96ce14"): "Mad V-Apes Fusion",
	thor.MustParseAddress("0xb14baed957b8e58db10ec5ef37927d83b3bbf297"): "Ukiyoe Warriors",
	thor.MustParseAddress("0xb3317c785176145603e0f6adfe32d8b2e0300633"): "Goatz Club - SuperMilk",
	thor.MustParseAddress("0xb59aea40b5d6946c0b593321318985d0d0bc66c0"): "VeReapers",
	thor.MustParseAddress("0xb757fc0906f08714315d2abd4b4f077521a21e34"): "VVAR DOGS",
	thor.MustParseAddress("0xbb74d3d8305f6a6b49448746de7f1c9effaf0f82"): "Wild Teens",
	thor.MustParseAddress("0xbc0447e063f00a6d43d9bb3c60380a86498d6e64"): "New Pigs Order - Slaughterhouse",
	thor.MustParseAddress("0xbcbf39013da096c97f0dc913f7ac1cdc42b9a721"): "Universal Inventory",
	thor.MustParseAddress("0xbcfc59dcc2a0977ac1e9b465566ad071e5ec06aa"): "VeNature",
	thor.MustParseAddress("0xbdf2b45bd428bba31c46b8d8d1f50615ee0e1416"): "3DAbles",
	thor.MustParseAddress("0xc22d8ca65bb9ee4a8b64406f3b0405cc1ebeec4e"): "DoS Baby Dragons",
	thor.MustParseAddress("0xc2de1fbb24d918a68923cfb24cc541aea7a49450"): "VICTS",
	thor.MustParseAddress("0xc35d04f8783f85ede2f329eed3c1e8b036223a06"): "Galaxy Portraits",
	thor.MustParseAddress("0xc766ddd21f14862ef426f15bfb28573fdad8bc51"): "Mino Mob Multiverse",
	thor.MustParseAddress("0xc9c8964cf25d2c738190f74b8508cdfac8650b9d"): "Bullys",
	thor.MustParseAddress("0xcb99479e30136d86f9d8a8e9a79a4ecc75e36066"): "VeCowboys",
	thor.MustParseAddress("0xd393c0dcccae49248862b462404b63a8546a888a"): "Guardians - Team Leaders",
	thor.MustParseAddress("0xd4310196a56c5193811ae418b8729d82b34abdcc"): "DoS Weapons",
	thor.MustParseAddress("0xd56340abb721b7c89c6ca3835efc490dfd66f9ae"): "Veshawties",
	thor.MustParseAddress("0xd6b546368087d82a230a561c777ca74776a1bb0c"): "DoS Eggs",
	thor.MustParseAddress("0xd6b93818ac38c936f51538e5e7832d7127b79622"): "Metatun",
	thor.MustParseAddress("0xd861be8e33ebd09764bfca242ca6a8c54dcf844a"): "Mad V-Apes Elementals",
	thor.MustParseAddress("0xda878be46f4a6ec013340fb985231ed67eb712d3"): "Shamanic Oracles",
	thor.MustParseAddress("0xdea490a03000f44d8df78991b19e90cf864906b4"): "VVAR DOGS - Portal Davvgs",
	thor.MustParseAddress("0xe1cd98a883c88622cbbd39b23d95490cd540891b"): "Doodle Thugs",
	thor.MustParseAddress("0xe4538ddaaf68137a98448552c87f6910f1e3470d"): "Shark Gang",
	thor.MustParseAddress("0xe4be710b7553602a37126bd2bade15df18c957ff"): "Zilly Zombies",
	thor.MustParseAddress("0xf19fe0f222e4f2a7587b817042fe58f4f330a009"): "Psycho Beasts - Prime",
	thor.MustParseAddress("0xf4d82631be350c37d92ee816c2bd4d5adf9e6493"): "Mino Mob",
	thor.MustParseAddress("0xf60b9aa0ab640c23b6dd6456a15d041a5f3a5f5e"): "Forest Nation - Keepers",
	thor.MustParseAddress("0xf92b2a2ff63ff09933c0ae797eff594ea3498c81"): "Ukiyoe Yōkai",
	thor.MustParseAddress("0xffcc1c4492c3b49825712e9a8909e4fcebfe6c02"): "Mad Ⓥ-Apes  Alpha",
	thor.MustParseAddress("0x174871c02c042ae2e23181b9f3bec7975a544cda"): "SWEAT Voucher",
	thor.MustParseAddress("0x2a7bc6e39bcf51f5c55e7fc779e6b4da30be30c3"): "VeHashes",
	thor.MustParseAddress("0x5cc3fedf2f0956b2d080f0bf4b361fa5599edb04"): "MVA Tickets",
	thor.MustParseAddress("0x5d431bc82b67c070639e747c50a13cff15403f18"): "RAW Point",
	thor.MustParseAddress("0x607007c278a6c87f2f08d0846cb053cd80279ed5"): "RAW Voucher",
	thor.MustParseAddress("0x8fd03f3a0f5dd1549f2416dccaf5acceada18292"): "WoV Lottery Tickets",
	thor.MustParseAddress("0xb7392e3da793d8386f990e495f28865bcfb5f9d6"): "Proof of Vote for VeChain Steering Committee 2023",
	thor.MustParseAddress("0xc40bc08af312ca03592a54f96fb34c10bd10cb37"): "SWEAT Point",
	thor.MustParseAddress("0xd64ae647c44bc1d2edde7c65d9605a0024b86c78"): "NonFungiBulls",
	thor.MustParseAddress("0xa435c77f27dc46b1e64cf25e3f8a380a1a615486"): "User Profile",
	thor.MustParseAddress("0x1f711f78685b4a5b0899d26aebd590163cfcb7eb"): "Hacken Club Membership",
	thor.MustParseAddress("0xe7a88bd74e80da5e3cb67af65e4711b992123c44"): "VeGem Rangers",
	thor.MustParseAddress("0x3a07dec3477c7e1df69c671df95471eefcf86175"): "Puraties",
	thor.MustParseAddress("0x6cb68e47080db4e3574f8a50df6717eeb32e0269"): "Gorilla Petz",
	thor.MustParseAddress("0x7ac767cc96a84ee89a32fa9dd9fe5fb406121f1d"): "Doodle Piglets",
	thor.MustParseAddress("0x998c9d999bd6af31089e4d3bc63a7d196f9a27ca"): "Ganja Girls",
	thor.MustParseAddress("0xc0327e7e13df8b578ad57b8a1aed2a4e001addb3"): "Concrete Jungles Plots",
	thor.MustParseAddress("0xd68ea9f36870aa4195cbe992eca0765d13a133fd"): "Concrete Jungles Buildings",
	thor.MustParseAddress("0xd7a8b0cebed38164c463e39f9f433daf963c5cfb"): "Banana Crack",
	thor.MustParseAddress("0xfc74715b3111909e63e0c0afe73ffe7892755917"): "420VeFam VeBudz",
	thor.MustParseAddress("0xe0ab6916048ee208154bd76f1343d84b726fa62a"): "vechain.energy Project",
	thor.MustParseAddress("0x4167d527340afa546bb88d5d83afb6272e48b40e"): "VeBounce",
	thor.MustParseAddress("0x47a2768ee043f1bd7cbc6f24c8f0854167d300e8"): "ExoWorlds",
	thor.MustParseAddress("0x5e6265680087520dc022d75f4c45f9ccd712ba97"): "World of V Phygital",
	thor.MustParseAddress("0x79fc8e2c3f313d240bfeee9143aadf25f128ca50"): "MVA The HiVe",
	thor.MustParseAddress("0x0d0f3e7ce89405f75b99f0bd6b498c00b6b937ce"): "Goatz Club - Supers",
	thor.MustParseAddress("0x8c810f79900d2b69f7043c7ff447f2eb3084606a"): "Non Fungible Book Club",
	thor.MustParseAddress("0x997c61cd02b5f2c8826ebcaf26080c650cabdda2"): "Honorary VeKings",
	thor.MustParseAddress("0xb1e19aeaa5da5aba4b5591e548b5b6505c08909e"): "VVAR DOGS - Uki-Dawgs",
	thor.MustParseAddress("0xb68f43cf91a2c9fa3f8ab369cb2fb23511eb7fb7"): "Tradze Town",
	thor.MustParseAddress("0xe045f9d4654d7ea0230aa7a36dd9d6a2d486f237"): "Crossfit Mediolanvm",
	thor.MustParseAddress("0x2f0586faa4b51a678cf5d0f27ce414f3f6d08517"): "Bored Eagle Fight Club",
	thor.MustParseAddress("0xfb3b2f8b4f8aae9e7a24ba0bcbb6a49d344f2ef3"): "Hive POP 2023",
	thor.MustParseAddress("0x06ff1e4b5e15d890e746dbefad3e2162a31c10b7"): "XP Network Tezos Bridge",
	thor.MustParseAddress("0x6354b35c510cae41cd45b568087bf767756b3589"): "VeRocket",
	thor.MustParseAddress("0x9aab6e4e017964ec7c0f092d431c314f0caf6b4b"): "Genesis Special",
	thor.MustParseAddress("0x4a572706ea28fa6e5dcf6325d114ad1c607ec7ca"): "Crossfit Mediolanvm Membership",
	thor.MustParseAddress("0xb6ad388aee8d88185c52d1a18bb81c28c567a394"): "OG WizardPunks",
	thor.MustParseAddress("0xdce5a78fe9cbba559c73a83ee40891b8a09516c2"): "VeCowboys - Pixel Outlaws",
	thor.MustParseAddress("0x2c59dfa1d8d9a1f17855d1db0d071662aebe16be"): "Block Bones",
	thor.MustParseAddress("0x63ddb855386066302f0c5602621c3e04c46de372"): "Sonetti di Shakespeare",
	thor.MustParseAddress("0xa7c92359b982605c906380a29846df7e4dcc5b1c"): "MVA Elite Tickets",
	thor.MustParseAddress("0x00fbadb64941319d6cbdeaf7d356d8a73eb4ae5e"): "Chakra NFT",
	thor.MustParseAddress("0x0422d505c9060673f82335c511d8aa9ddb1f7173"): "Goatz Club - Smart Goatz",
	thor.MustParseAddress("0x9decfb16f6639907a378c48bd4c57de12527158c"): "The Shts",
	thor.MustParseAddress("0xf0e778bd5c4c2f219a2a5699e3afd2d82d50e271"): "XP Network",
	thor.MustParseAddress("0xf2d4e0ca9bde645445afe9cd004ca691a8a7da92"): "VeTower - Passkey VIP",
	thor.MustParseAddress("0x7633b0e3c21cc6bacf5780cab8b622b7495666a7"): "Llamamons",
	thor.MustParseAddress("0x84677a2fdc77d0fba658c75a41ef62dac67bfcf4"): "Metalands NFT",
	thor.MustParseAddress("0x80fe06bd44f5ebe8f19c39065e2bb1f3bda2806a"): "Graphite Black Card",
	thor.MustParseAddress("0x6e04f400810be5c570c08ea2def43c4d44481063"): "vet.domains",
	thor.MustParseAddress("0x1c8adf6d8e6302d042b1f09bad0c7f65de3660ea"): "vet.domains",
	thor.MustParseAddress("0x93ae8aab337e58a6978e166f8132f59652ca6c56"): "Genesis",
	thor.MustParseAddress("0xafe1d7d4ac69c2f31dfde6dd31f3df955ddec2a3"): "Gresini Racing",
	thor.MustParseAddress("0xfc32a9895c78ce00a1047d602bd81ea8134cc32b"): "veDelegate Wallet",
	thor.MustParseAddress("0xba2834c43670a28175f965be544386d04b602243"): "BRUSHIZER",
}
