package block

import (
	"github.com/darrenvechain/thor-go-sdk/client"
	"github.com/vechain/thorflux/rlp"
)

type Block struct {
	ExpandedBlock *client.ExpandedBlock
	RawHeader     *rlp.Header
}
