package transactions

import (
	"github.com/vechain/thor/v2/api"
	"math/big"

	"github.com/vechain/thor/v2/tx"
)

type txStats struct {
	legacyCount        int
	dynamicFeeCount    int
	clauseCount        int
	vetTransferCount   int
	eventCount         int
	vetTransfersAmount *big.Float
	totalRewards       float64
}

func (s *txStats) processTx(t *api.JSONEmbeddedTx) {
	s.clauseCount += len(t.Clauses)
	// process Rewards
	if t.Reward != nil {
		// Convert Reward to float64 (in Wei) using big.Float for precision.
		rewardFloat, _ := new(big.Float).SetInt((*big.Int)(t.Reward)).Float64()
		s.totalRewards += rewardFloat
	}

	switch t.Type {
	case tx.TypeLegacy:
		s.legacyCount++
	case tx.TypeDynamicFee:
		s.dynamicFeeCount++
	}
}

func (s *txStats) processOutput(o *api.JSONOutput) {
	s.vetTransferCount += len(o.Transfers)
	s.eventCount += len(o.Events)
}

func (s *txStats) processTransf(transf *api.JSONTransfer) {
	s.vetTransfersAmount.Add(s.vetTransfersAmount, new(big.Float).SetInt((*big.Int)(transf.Amount)))
}
