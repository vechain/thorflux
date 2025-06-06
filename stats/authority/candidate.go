package authority

import "github.com/vechain/thor/v2/thor"

type Candidate struct {
	Master    thor.Address
	Endorsor  thor.Address
	Indentity []byte
	Active    bool
}
