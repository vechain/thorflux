package influxdb

import (
	"math/big"

	"github.com/vechain/thor/v2/api/blocks"
)

type txStats struct {
	clauseCount        int
	vetTransferCount   int
	eventCount         int
	vetTransfersAmount *big.Float
	totalRewards       float64
}

func (s *txStats) processTx(t *blocks.JSONEmbeddedTx) {
	s.clauseCount += len(t.Clauses)
	// process Rewards
	if t.Reward != nil {
		// Convert Reward to float64 (in Wei) using big.Float for precision.
		rewardFloat, _ := new(big.Float).SetInt((*big.Int)(t.Reward)).Float64()
		s.totalRewards += rewardFloat
	}
}

func (s *txStats) processOutput(o *blocks.JSONOutput) {
	s.vetTransferCount += len(o.Transfers)
	s.eventCount += len(o.Events)
}

func (s *txStats) processTransf(transf *blocks.JSONTransfer) {
	s.vetTransfersAmount.Add(s.vetTransfersAmount, new(big.Float).SetInt((*big.Int)(transf.Amount)))
}
