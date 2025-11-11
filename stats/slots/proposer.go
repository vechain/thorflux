package slots

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/vechain/thor/v2/thor"
)

// FutureProposer represents a future block proposer
type FutureProposer struct {
	AuthorityNode
	Position int `json:"position"`
}

// ProposerCalculator handles future proposer calculations
type ProposerCalculator struct{}

// NewProposerCalculator creates a new proposer calculator
func NewProposerCalculator() *ProposerCalculator {
	return &ProposerCalculator{}
}

// NextBlockProposers calculates the next N proposers for a given block
func (pc *ProposerCalculator) NextBlockProposers(
	nodes []AuthorityNode,
	seed []byte,
	blockNumber uint32,
	count int,
) ([]FutureProposer, error) {

	// Filter to only active authority nodes
	activeNodes := make([]AuthorityNode, 0)
	for _, node := range nodes {
		if node.Active {
			activeNodes = append(activeNodes, node)
		}
	}

	if len(activeNodes) == 0 {
		return []FutureProposer{}, nil
	}

	// Shuffle the active authority nodes using the same algorithm as authority package
	shuffled := pc.shuffleAuthorityNodes(activeNodes, seed, blockNumber)

	// Take the first 'count' proposers
	maxCount := count
	if len(shuffled) < maxCount {
		maxCount = len(shuffled)
	}

	futureProposers := make([]FutureProposer, maxCount)
	for i := 0; i < maxCount; i++ {
		futureProposers[i] = FutureProposer{
			AuthorityNode: shuffled[i],
			Position:      i + 1,
		}
	}

	return futureProposers, nil
}

// shuffleAuthorityNodes shuffles authority nodes using VeChain's proposer selection algorithm
func (pc *ProposerCalculator) shuffleAuthorityNodes(nodes []AuthorityNode, seed []byte, blockNumber uint32) []AuthorityNode {
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], blockNumber)

	var list []struct {
		AuthorityNode
		addr thor.Address
		hash thor.Bytes32
	}

	for _, node := range nodes {
		list = append(list, struct {
			AuthorityNode
			addr thor.Address
			hash thor.Bytes32
		}{
			node,
			node.Master,
			thor.Blake2b(seed, num[:], node.Master.Bytes()),
		})
	}

	// Sort by hash value
	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].hash.Bytes(), list[j].hash.Bytes()) < 0
	})

	// Extract addresses in sorted order
	shuffled := make([]AuthorityNode, 0, len(list))
	for _, item := range list {
		shuffled = append(shuffled, item.AuthorityNode)
	}

	return shuffled
}
