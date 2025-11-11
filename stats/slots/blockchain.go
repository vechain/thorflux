package slots

import (
	"errors"
	"github.com/vechain/thorflux/stats/contracts/generated/listauthority"
	"log/slog"
	"math/big"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

// FetchAuthorityNodes fetches all authority nodes from the authority contract at a specific block
func FetchAuthorityNodes(thorClient *thorclient.Client, blockID thor.Bytes32) ([]AuthorityNode, error) {
	gas := uint64(3000000)
	caller := thor.MustParseAddress("0x6d95e6dca01d109882fe1726a2fb9865fa41e7aa")
	gasPayer := thor.MustParseAddress("0xd3ae78222beadb038203be21ed5ce7c9b1bff602")
	authorityContract := thor.MustParseAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")

	// Get the contract bytecode for listing all authority nodes - use the same variable name as authority package
	clauses := api.Clauses{
		{
			To:    nil,
			Value: nil,
			Data:  listauthority.Bytecode.Hex(),
		},
		{
			To:   &authorityContract,
			Data: listauthority.Methods().ListMethod().Selector.Hex(), // Authority contract method selector
		},
	}

	body := &api.BatchCallData{
		Gas:      gas,
		Caller:   &caller,
		GasPayer: &gasPayer,
		Clauses:  clauses,
	}

	response, err := thorClient.InspectClauses(body, thorclient.Revision(blockID.String()))
	if err != nil {
		return nil, err
	}

	if len(response) < 2 {
		slog.Error("Insufficient contract response clauses", "response_length", len(response), "block_id", blockID.String())
		return nil, errors.New("insufficient response clauses from contract call")
	}
	
	// Parse the response to extract authority node data
	slog.Debug("Contract response received", "data_length", len(response[1].Data), "block_id", blockID.String())
	return parseAuthorityNodeResponse(response[1].Data)
}

// parseAuthorityNodeResponse parses the raw contract response into authority node structs
func parseAuthorityNodeResponse(data string) ([]AuthorityNode, error) {
	slog.Debug("Parsing authority node response", "raw_data_length", len(data))

	if len(data) < 2 {
		slog.Error("Invalid response data", "data_length", len(data))
		return nil, errors.New("invalid response data")
	}

	// Remove '0x' prefix
	data = data[2:]

	// Check minimum length for header parsing
	if len(data) < 128 {
		slog.Warn("Response data too short for parsing, returning empty authority nodes list", "data_length", len(data))
		return []AuthorityNode{}, nil
	}

	// Parse the return data structure
	valueType, _ := big.NewInt(0).SetString(data[:64], 16)
	if valueType.Cmp(big.NewInt(32)) != 0 {
		slog.Warn("Wrong type returned by contract, returning empty authority nodes list", "value_type", valueType.String())
		return []AuthorityNode{}, nil
	}
	data = data[64:]

	// Get the number of authority nodes
	amount, _ := big.NewInt(0).SetString(data[:64], 16)
	data = data[64:]

	// Check if we have enough data for all nodes (each node needs 3 * 64 = 192 characters)
	// Updated: removed identity field, so now only 3 fields per node
	expectedLength := amount.Uint64() * 192
	if uint64(len(data)) < expectedLength {
		slog.Warn("Insufficient data for parsing all authority nodes, returning empty list",
			"expected_length", expectedLength,
			"actual_length", len(data),
			"node_count", amount.Uint64())
		return []AuthorityNode{}, nil
	}

	nodes := make([]AuthorityNode, amount.Uint64())
	for index := uint64(0); index < amount.Uint64(); index++ {
		// Ensure we have enough data for this node (3 fields * 64 chars = 192 chars)
		if len(data) < 192 {
			slog.Warn("Insufficient data for parsing authority node", "remaining_data", len(data), "node_index", index)
			// Return what we have parsed so far
			return nodes[:index], nil
		}

		// Parse master address
		master := thor.MustParseAddress(data[24:64])
		data = data[64:]

		// Parse endorsor address
		endorsor := thor.MustParseAddress(data[24:64])
		data = data[64:]

		// Parse active status (identity field removed)
		activeString := data[:64]
		
		// Debug: Print actual hex values to understand the encoding
		if master.String() == "0xf6ccf0c82cf386e37d55ccdd009965f093043a2d" {
			slog.Info("Debug active status for target node", 
				"master", master.String(),
				"activeString", activeString,
				"block", "23224453")
		}
		
		// Standard Ethereum ABI boolean encoding:
		// false = 0x0000000000000000000000000000000000000000000000000000000000000000
		// true  = 0x0000000000000000000000000000000000000000000000000000000000000001
		active := false // Default to false
		if activeString == "0000000000000000000000000000000000000000000000000000000000000001" {
			active = true
		}
		data = data[64:]

		nodes[index] = AuthorityNode{
			Master:   master,
			Endorsor: endorsor,
			Active:   active,
		}
	}

	return nodes, nil
}

// AuthorityListAllBytecode contains the compiled bytecode for the authority list contract
// This is the same bytecode used in the authority package
const AuthorityListAllBytecode = "608060405234801561001057600080fd5b5061068f806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c8063629a4b521461003b5780636f0470aa14610059575b600080fd5b61004361006e565b604051610050919061063f565b60405180910390f35b61006161011d565b60405161005091906105c0565b6040517f8eaa6ac000000000000000000000000000000000000000000000000000000000815260009065506172616d7390638eaa6ac0906100c7907370726f706f7365722d656e646f7273656d656e749060040161063f565b60206040518083038186803b1580156100df57600080fd5b505afa1580156100f3573d6000803e3d6000fd5b505050506040513d601f19601f820116820180604052508101906101179190610587565b90505b90565b6060600061012961006e565b604080516065808252610cc0820190925291925060609190816020015b61014e6104c3565b81526020019060019003908161014657905050905060007f417574686f72697479000000000000000000000000000000000000000000000060b81c68ffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16633df4ddf46040518163ffffffff1660e01b815260040160206040518083038186803b1580156101d957600080fd5b505afa1580156101ed573d6000803e3d6000fd5b505050506040513d601f19601f820116820180604052508101906102119190610514565b905060005b73ffffffffffffffffffffffffffffffffffffffff82161561040d576040517fc2bc2efc0000000000000000000000000000000000000000000000000000000081526000908190819068417574686f726974799063c2bc2efc9061027e90889060040161059f565b60806040518083038186803b15801561029657600080fd5b505afa1580156102aa573d6000803e3d6000fd5b505050506040513d601f19601f820116820180604052508101906102ce9190610536565b93509350935050868373ffffffffffffffffffffffffffffffffffffffff1631101580156102fc5750606584105b1561036e5760405180608001604052808673ffffffffffffffffffffffffffffffffffffffff1681526020018473ffffffffffffffffffffffffffffffffffffffff16815260200183815260200182151581525086858151811061035c57fe5b60209081029190910101526001909301925b6040517fab73e31600000000000000000000000000000000000000000000000000000000815268417574686f726974799063ab73e316906103b390889060040161059f565b60206040518083038186803b1580156103cb57600080fd5b505afa1580156103df573d6000803e3d6000fd5b505050506040513d601f19601f820116820180604052508101906104039190610514565b9450505050610216565b6065811415610422578294505050505061011a565b60608167ffffffffffffffff8111801561043b57600080fd5b5060405190808252806020026020018201604052801561047557816020015b6104626104c3565b81526020019060019003908161045a5790505b50905060005b828110156104b65784818151811061048f57fe5b6020026020010151828281581106104a357fe5b602090810291909101015260010161047b565b50945061011a9350505050565b60408051608081018252600080825260208201819052918101829052606081019190915290565b805173ffffffffffffffffffffffffffffffffffffffff8116811461050e57600080fd5b92915050565b600060208284031215610525578081fd5b61052f83836104ea565b9392505050565b6000806000806080858703121561054b578283fd5b845161055681610648565b935061056586602087016104ea565b925060408501519150606085015161057c81610648565b939692955090935050565b600060208284031215610598578081fd5b5051919050565b73ffffffffffffffffffffffffffffffffffffffff91909116815260200190565b602080825282518282018190526000919060409081850190868401855b82811015610632578151805173ffffffffffffffffffffffffffffffffffffffff90811686528782015116878601528581015186860152606090810151151590850152608090930192908501906001016105dd565b5091979650505050505050565b90815260200190565b801515811461065657600080fd5b5056fea26469706673582212205d562338213e0bd497411ba6aaded36136be9c73a750c3e5efee7dcf69adbb6064736f6c634300060c0033"
