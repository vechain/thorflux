package block

import (
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/block"
)

type Block struct {
	ExpandedBlock *blocks.JSONExpandedBlock
	RawHeader     *block.Header
	ForkDetected  bool
}
