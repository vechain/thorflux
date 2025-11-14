package slots

import (
	"errors"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/stats/contracts/generated/listauthority"
	"github.com/vechain/thorflux/types"
	"log/slog"
)

// FetchAuthorityNodes fetches all authority nodes from the authority contract at a specific block
func FetchAuthorityNodes(thorClient *thorclient.Client, blockID thor.Bytes32) (types.AuthorityNodeList, error) {
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

	nodes, err := listauthority.Methods().ListMethod().Decode(hexutil.MustDecode(response[1].Data))
	if err != nil {
		return nil, err
	}
	var authNodes types.AuthorityNodeList
	for _, node := range nodes {
		authNodes = append(authNodes, types.AuthorityNode{
			Master:   thor.Address(node.NodeMaster),
			Endorsor: thor.Address(node.Endorsor),
			Active:   node.Active,
		})
	}

	return authNodes, nil
}
