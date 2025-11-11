package slots

import (
	"github.com/vechain/thor/v2/api"
	"github.com/vechain/thor/v2/builtin"
	"log/slog"

	"github.com/vechain/thor/v2/thor"
)

// AuthorityNode represents a VeChain authority node with complete information
type AuthorityNode struct {
	Master   thor.Address `json:"master"`
	Endorsor thor.Address `json:"endorsor"`
	Active   bool         `json:"active"`
}

// AuthorityNodeList manages a list of authority nodes for slot calculations
type AuthorityNodeList struct {
	nodes    []AuthorityNode
	revision thor.Bytes32
}

// NewAuthorityNodeList creates a new authority node list
func NewAuthorityNodeList() *AuthorityNodeList {
	return &AuthorityNodeList{
		nodes: make([]AuthorityNode, 0),
	}
}

// SetNodes updates the authority node list with a new set of nodes
func (al *AuthorityNodeList) SetNodes(nodes []AuthorityNode, revision thor.Bytes32) {
	al.nodes = nodes
	al.revision = revision

	activeCount := 0
	for _, node := range nodes {
		if node.Active {
			activeCount++
		}
	}

	slog.Debug("Authority node list updated",
		"revision", revision.String(),
		"total_nodes", len(nodes),
		"active_nodes", activeCount)
}

// GetActiveNodes returns only the active authority nodes
func (al *AuthorityNodeList) GetActiveNodes() []AuthorityNode {
	active := make([]AuthorityNode, 0)
	for _, node := range al.nodes {
		if node.Active {
			active = append(active, node)
		}
	}
	return active
}

// GetActiveCount returns the number of active authority nodes
func (al *AuthorityNodeList) GetActiveCount() int {
	count := 0
	for _, node := range al.nodes {
		if node.Active {
			count++
		}
	}
	return count
}

// ShouldRefresh returns true if the authority node list should be refreshed
func (al *AuthorityNodeList) ShouldRefresh(block *api.JSONExpandedBlock) bool {
	if len(al.nodes) == 0 {
		return true
	}
	candidateMap := make(map[thor.Address]bool)
	for _, candidate := range al.nodes {
		candidateMap[candidate.Endorsor] = true
		candidateMap[candidate.Master] = true
	}

	for _, r := range block.Transactions {
		for _, o := range r.Outputs {
			for _, ev := range o.Events {
				if ev.Address == builtin.Authority.Address {
					return true
				}
			}
			for _, t := range o.Transfers {
				if _, ok := candidateMap[t.Sender]; ok {
					return true
				}
			}
		}
	}

	return false
}
