package types

import "github.com/vechain/thor/v2/thor"

// AuthorityNode represents a VeChain authority node with complete information
type AuthorityNode struct {
	Master   thor.Address `json:"master"`
	Endorsor thor.Address `json:"endorsor"`
	Active   bool         `json:"active"`
}

// AuthorityNodeList manages a list of authority nodes for slot calculations
type AuthorityNodeList []AuthorityNode

// SetNodes updates the authority node list with a new set of nodes
func (al *AuthorityNodeList) SetNodes(nodes []AuthorityNode) {
	*al = AuthorityNodeList(nodes)
}

// GetActiveNodes returns only the active authority nodes
func (al *AuthorityNodeList) GetActiveNodes() []AuthorityNode {
	active := make([]AuthorityNode, 0)
	for _, node := range *al {
		if node.Active {
			active = append(active, node)
		}
	}
	return active
}

// GetActiveCount returns the number of active authority nodes
func (al *AuthorityNodeList) GetActiveCount() int {
	count := 0
	for _, node := range *al {
		if node.Active {
			count++
		}
	}
	return count
}
