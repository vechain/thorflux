package influxdb

import (
	"github.com/vechain/thor/v2/api/blocks"
	"math/big"
)

type priorityFeeStats struct {
	candlestickCount int
	openFee          float64
	highFee          float64
	lowFee           float64
	closeFee         float64
}

func (s *priorityFeeStats) processTx(t *blocks.JSONEmbeddedTx) {
	// Process candlestick values from MaxPriorityFeePerGas.
	if t.MaxPriorityFeePerGas != nil {
		// Convert fee from Wei to Gwei (divide by 1e9)
		feeWei, _ := (*big.Int)(t.MaxPriorityFeePerGas).Float64()
		feeGwei := feeWei / 1e9

		// Set the open fee for the first valid transaction.
		if s.candlestickCount == 0 {
			s.openFee = feeGwei
			s.highFee = feeGwei
			s.lowFee = feeGwei
		} else {
			if feeGwei > s.highFee {
				s.highFee = feeGwei
			}
			if feeGwei < s.lowFee {
				s.lowFee = feeGwei
			}
		}
		// The close fee will be the fee of the current transaction.
		s.closeFee = feeGwei
		s.candlestickCount++
	}
}
