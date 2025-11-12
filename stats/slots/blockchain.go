package slots

import (
	"errors"
	"log/slog"
	"math/big"

	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"

	"github.com/vechain/thorflux/stats/contracts/generated/listauthority"
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
	return parseAuthorityNodeResponse(response[1].Data)
}

// parseAuthorityNodeResponse parses the raw contract response into authority node structs
func parseAuthorityNodeResponse(data string) ([]AuthorityNode, error) {
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
