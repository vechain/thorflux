package slots

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thorclient/builtin"
	"testing"

	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
)

func TestBlock23224453AuthorityNodes(t *testing.T) {
	// Connect to the VeChain network
	thorURL := "https://mainnet.vechain.org"
	client := thorclient.New(thorURL)

	// The specific block we want to investigate
	blockNumber := uint32(23224453)

	// Get the block to get its ID
	block, err := client.Block(fmt.Sprintf("%d", blockNumber))
	if err != nil {
		t.Fatalf("Failed to get block %d: %v", blockNumber, err)
	}

	fmt.Printf("=== Block %d Analysis ===\n", blockNumber)
	fmt.Printf("Block ID: %s\n", block.ID.String())
	fmt.Printf("Block Signer: %s\n", block.Signer.String())

	// Fetch authority nodes for this block
	nodes, err := FetchAuthorityNodes(client, block.ID)
	if err != nil {
		t.Fatalf("Failed to fetch authority nodes for block %d: %v", blockNumber, err)
	}

	fmt.Printf("\n=== Authority Nodes Status ===\n")
	fmt.Printf("Total nodes: %d\n", len(nodes))

	activeCount := 0
	inactiveCount := 0

	// Addresses we're specifically interested in
	expectedProposer := thor.MustParseAddress("0xf6ccf0c82cf386e37d55ccdd009965f093043a2d")
	actualSigner := thor.MustParseAddress("0x6872a236ab21258e05358ed510c215ca6b70d442")

	var expectedProposerNode *AuthorityNode
	var actualSignerNode *AuthorityNode

	for i, node := range nodes {
		if node.Active {
			activeCount++
		} else {
			inactiveCount++
		}

		// Check for our specific addresses
		if node.Master == expectedProposer {
			expectedProposerNode = &nodes[i]
			fmt.Printf("*** EXPECTED PROPOSER FOUND ***\n")
			fmt.Printf("  Master: %s\n", node.Master.String())
			fmt.Printf("  Endorsor: %s\n", node.Endorsor.String())
			fmt.Printf("  Active: %t\n", node.Active)
		}

		if node.Master == actualSigner {
			actualSignerNode = &nodes[i]
			fmt.Printf("*** ACTUAL SIGNER FOUND ***\n")
			fmt.Printf("  Master: %s\n", node.Master.String())
			fmt.Printf("  Endorsor: %s\n", node.Endorsor.String())
			fmt.Printf("  Active: %t\n", node.Active)
		}

		// Print inactive nodes for debugging
		if !node.Active {
			fmt.Printf("INACTIVE NODE: %s (Endorsor: %s)\n", node.Master.String(), node.Endorsor.String())
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Active nodes: %d\n", activeCount)
	fmt.Printf("Inactive nodes: %d\n", inactiveCount)

	if expectedProposerNode == nil {
		fmt.Printf("ERROR: Expected proposer %s NOT FOUND in authority list!\n", expectedProposer.String())
	} else {
		fmt.Printf("Expected proposer %s status: Active=%t\n", expectedProposer.String(), expectedProposerNode.Active)
	}

	if actualSignerNode == nil {
		fmt.Printf("ERROR: Actual signer %s NOT FOUND in authority list!\n", actualSigner.String())
	} else {
		fmt.Printf("Actual signer %s status: Active=%t\n", actualSigner.String(), actualSignerNode.Active)
	}

	// This should help us understand the discrepancy
	if expectedProposerNode != nil && expectedProposerNode.Active {
		fmt.Printf("\n*** ISSUE FOUND ***\n")
		fmt.Printf("Expected proposer %s is still marked as ACTIVE\n", expectedProposer.String())
		fmt.Printf("This explains why the missed slot is not being reflected in the authority contract state\n")
		fmt.Printf("The authority contract at block %d revision has not yet updated the validator status\n", blockNumber)
	}
}

func TestBlock23224453AuthorityNodesThorclient(t *testing.T) {
	thorURL := "https://mainnet.vechain.org"
	client := thorclient.New(thorURL)

	authority, err := builtin.NewAuthority(client)
	require.NoError(t, err)

	var nodes []AuthorityNode

	first, err := authority.Revision("23224453").First()
	require.NoError(t, err)
	current := first

	fmt.Printf("=== Block 23224453 Builtin Authority Analysis ===\n")

	for {
		node, err := authority.Revision("23224453").Get(current)
		require.NoError(t, err)

		nodes = append(nodes, AuthorityNode{
			Master:   current,
			Endorsor: node.Endorsor,
			Active:   node.Active,
		})

		current, err = authority.Revision("23224453").Next(current)
		require.NoError(t, err)

		if current.String() == (thor.Address{}).String() {
			break
		}
	}

	fmt.Printf("Total authority nodes: %d\n", len(nodes))

	//// Addresses we're specifically interested in
	//expectedProposer := thor.MustParseAddress("0xf6ccf0c82cf386e37d55ccdd009965f093043a2d")
	//actualSigner := thor.MustParseAddress("0x6872a236ab21258e05358ed510c215ca6b70d442")

	activeCount := 0

	block, err := client.Block(fmt.Sprintf("%d", 23224453))
	require.NoError(t, err)

	nodesContract, err := FetchAuthorityNodes(client, block.ID)
	if err != nil {
		t.Fatalf("Failed to fetch authority nodes for block %d: %v", 23224453, err)
	}

	for _, node := range nodes {
		if node.Active {
			activeCount++
		}
		for _, nodeContract := range nodesContract {
			if node.Master == nodeContract.Master && nodeContract.Endorsor == node.Endorsor {
				if node.Active != nodeContract.Active {
					fmt.Printf("*** Disparity found ***\n")
					fmt.Printf(" Builtin Master: %s\n", node.Master.String())
					fmt.Printf(" Builtin Endorsor: %s\n", node.Endorsor.String())
					fmt.Printf(" Builtin Active: %t - Contract Active %t\n", node.Active, nodeContract.Active)
				}
			}
		}
	}

	fmt.Printf("\n=== Builtin Authority Summary ===\n")
	fmt.Printf("Active nodes: %d\n", activeCount)
	fmt.Printf("Inactive nodes: %d\n", len(nodes)-activeCount)

	// Print all nodes for debugging
	fmt.Printf("\n=== All Authority Nodes (builtin) ===\n")
	for i, node := range nodes {
		fmt.Printf("Node %d: Master=%s, Endorsor=%s, Active=%t\n",
			i+1, node.Master.String(), node.Endorsor.String(), node.Active)
	}
}
