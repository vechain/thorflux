package slots

import (
	"testing"

	"github.com/vechain/thor/v2/thor"
)

func TestProposerCalculator_CalculateFutureProposers(t *testing.T) {
	pc := NewProposerCalculator()

	// Create test authority nodes
	nodes := []AuthorityNode{
		{Master: thor.MustParseAddress("0x1234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xA234567890123456789012345678901234567890"), Active: true},
		{Master: thor.MustParseAddress("0x2234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xB234567890123456789012345678901234567890"), Active: true},
		{Master: thor.MustParseAddress("0x3234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xC234567890123456789012345678901234567890"), Active: false},
		{Master: thor.MustParseAddress("0x4234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xD234567890123456789012345678901234567890"), Active: true},
	}

	seed := []byte("test-seed")
	blockNumber := uint32(100)
	count := 3

	proposers, err := pc.NextBlockProposers(nodes, seed, blockNumber, count)
	if err != nil {
		t.Fatalf("NextBlockProposers failed: %v", err)
	}

	// Should only include active authority nodes (3 active out of 4 total)
	if len(proposers) != 3 {
		t.Errorf("Expected 3 proposers, got %d", len(proposers))
	}

	// Check positions are sequential starting from 1
	for i, proposer := range proposers {
		if proposer.Position != i+1 {
			t.Errorf("Expected position %d, got %d", i+1, proposer.Position)
		}
	}
}

func TestAuthorityNodeList_GetActiveCount(t *testing.T) {
	anl := NewAuthorityNodeList()

	nodes := []AuthorityNode{
		{Master: thor.MustParseAddress("0x1234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xA234567890123456789012345678901234567890"), Active: true},
		{Master: thor.MustParseAddress("0x2234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xB234567890123456789012345678901234567890"), Active: false},
		{Master: thor.MustParseAddress("0x3234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xC234567890123456789012345678901234567890"), Active: true},
	}

	revision := thor.Bytes32{}
	anl.SetNodes(nodes, revision)

	activeCount := anl.GetActiveCount()
	if activeCount != 2 {
		t.Errorf("Expected 2 active authority nodes, got %d", activeCount)
	}
}

func TestAuthorityNodeList_GetActiveNodes(t *testing.T) {
	anl := NewAuthorityNodeList()

	nodes := []AuthorityNode{
		{Master: thor.MustParseAddress("0x1234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xA234567890123456789012345678901234567890"), Active: true},
		{Master: thor.MustParseAddress("0x2234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xB234567890123456789012345678901234567890"), Active: false},
		{Master: thor.MustParseAddress("0x3234567890123456789012345678901234567890"), Endorsor: thor.MustParseAddress("0xC234567890123456789012345678901234567890"), Active: true},
	}

	revision := thor.Bytes32{}
	anl.SetNodes(nodes, revision)

	active := anl.GetActiveNodes()
	if len(active) != 2 {
		t.Errorf("Expected 2 active authority nodes, got %d", len(active))
	}

	for _, node := range active {
		if !node.Active {
			t.Errorf("Found inactive authority node in active list: %s", node.Master.String())
		}
	}
}
