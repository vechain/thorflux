package slots

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"slices"
	"sort"

	"github.com/vechain/thor/v2/thor"
)

// FutureProposer represents a future block proposer
type FutureProposer struct {
	AuthorityNode
	Position int `json:"position"`
}

// PosNode represents a PoS validator node with weight information
type PosNode struct {
	Master   thor.Address `json:"master"`
	Endorsor thor.Address `json:"endorsor"`
	Active   bool         `json:"active"`
	Weight   uint64       `json:"weight"`
}

// ToAuthorityNode converts PosNode to AuthorityNode (for interface compatibility)
func (p PosNode) ToAuthorityNode() AuthorityNode {
	return AuthorityNode{
		Master:   p.Master,
		Endorsor: p.Endorsor,
		Active:   p.Active,
	}
}

// NextBlockProposers calculates the next N proposers for a given block (PoA mode)
func NextBlockProposers(
	nodes []AuthorityNode,
	seed []byte,
	blockNumber uint32,
	count int,
) ([]FutureProposer, error) {
	return NextBlockProposersPoA(nodes, seed, blockNumber, count)
}

// NextBlockProposersPoA calculates the next N proposers for PoA consensus
func NextBlockProposersPoA(
	nodes []AuthorityNode,
	seed []byte,
	blockNumber uint32,
	count int,
) ([]FutureProposer, error) {
	// Shuffle the active authority nodes using the same algorithm as authority package
	shuffled := shuffleAuthorityNodes(nodes, seed, blockNumber)

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

// NextBlockProposersPoS calculates the next N proposers for PoS consensus (Hayabusa mode)
func NextBlockProposersPoS(
	nodes []PosNode,
	seed []byte,
	blockNumber uint32,
	count int,
) ([]FutureProposer, error) {
	// Shuffle using PoS weighted algorithm
	shuffled := shufflePosNodes(nodes, seed, blockNumber)

	// Take the first 'count' proposers
	maxCount := count
	if len(shuffled) < maxCount {
		maxCount = len(shuffled)
	}

	futureProposers := make([]FutureProposer, maxCount)
	for i := 0; i < maxCount; i++ {
		futureProposers[i] = FutureProposer{
			AuthorityNode: shuffled[i].ToAuthorityNode(),
			Position:      i + 1,
		}
	}

	return futureProposers, nil
}

// shuffleAuthorityNodes shuffles authority nodes using VeChain's proposer selection algorithm
func shuffleAuthorityNodes(nodes []AuthorityNode, seed []byte, blockNumber uint32) []AuthorityNode {
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

// shufflePosNodes shuffles PoS nodes using weighted random sampling with exponential distribution
func shufflePosNodes(nodes []PosNode, seed []byte, blockNumber uint32) []PosNode {
	if len(nodes) == 0 {
		return nodes
	}

	var num [4]byte
	binary.BigEndian.PutUint32(num[:], blockNumber)

	// Generate a seed for the deterministic pseudo-random generator
	hashedSeed := thor.Blake2b(seed, num[:])
	src := rand.NewChaCha8(hashedSeed)
	pseudoRND := rand.New(src) //#nosec G404

	// Create entries with scores for weighted random sampling
	type posEntry struct {
		node  PosNode
		score float64
	}

	entries := make([]posEntry, 0, len(nodes))

	// Calculate priority scores for each validator based on their weight
	// using the exponential distribution method for weighted random sampling
	for _, node := range nodes {
		if !node.Active {
			continue
		}

		// IMPORTANT: Every validator should be allocated with the deterministic
		// random number sequence from the same source
		random := pseudoRND.Float64()
		if random == 0 {
			random = 1e-10 // prevent ln(0)
		}

		if node.Weight == 0 {
			continue // Skip nodes with zero weight
		}

		// Score calculation using exponential distribution: -ln(random)/weight
		// https://en.wikipedia.org/wiki/Reservoir_sampling#Algorithm_A-Res
		score := -math.Log(random) / float64(node.Weight)

		entries = append(entries, posEntry{
			node:  node,
			score: score,
		})
	}

	// Sort validators by priority score in ascending order (lowest score first)
	slices.SortStableFunc(entries, func(a, b posEntry) int {
		switch {
		case a.score < b.score:
			return -1
		case a.score > b.score:
			return 1
		default:
			return 0
		}
	})

	// Extract nodes in sorted order
	shuffled := make([]PosNode, 0, len(entries))
	for _, entry := range entries {
		shuffled = append(shuffled, entry.node)
	}

	return shuffled
}
