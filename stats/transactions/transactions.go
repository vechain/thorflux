package transactions

import (
	"math"
	"math/big"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

func Write(event *types.Event) []*write.Point {
	txs := event.Block.Transactions

	priorityFeeStat := priorityFeeStats{}
	txStat := txStats{vetTransfersAmount: &big.Float{}}
	coefStat := coefStats{Total: len(txs), coefCount: map[float64]int{}}

	for _, t := range txs {
		txStat.processTx(t)
		coefStat.processTx(t)
		priorityFeeStat.processTx(t)

		for _, o := range t.Outputs {
			txStat.processOutput(o)

			for _, transf := range o.Transfers {
				txStat.processTransf(transf)
			}
		}
	}

	coefStat.finalizeCalc()

	flags := make(map[string]any)

	flags["total_txs"] = len(txs)
	flags["total_clauses"] = txStat.clauseCount
	flags["vet_transfers"] = txStat.vetTransferCount
	flags["vet_transfers_amount"] = txStat.vetTransfersAmount
	flags["validator_rewards"] = txStat.totalRewards / math.Pow10(config.VETDecimals)

	flags["coef_average"] = coefStat.Average
	flags["coef_max"] = coefStat.Max
	flags["coef_min"] = coefStat.Min
	flags["coef_mode"] = coefStat.Mode
	flags["coef_median"] = coefStat.Median

	// If we have at least one valid transaction for candlestick, add the candlestick fields.
	if priorityFeeStat.candlestickCount > 0 {
		flags["priority_fee_open"] = priorityFeeStat.openFee
		flags["priority_fee_close"] = priorityFeeStat.closeFee
		flags["priority_fee_high"] = priorityFeeStat.highFee
		flags["priority_fee_low"] = priorityFeeStat.lowFee
		flags["candlestick_tx_count"] = priorityFeeStat.candlestickCount
	}
	flags["legacy_txs"] = txStat.legacyCount
	flags["dyn_fee_txs"] = txStat.dynamicFeeCount

	p := influxdb2.NewPoint(config.TransactionsMeasurement, event.DefaultTags, flags, event.Timestamp)
	return []*write.Point{p}
}
