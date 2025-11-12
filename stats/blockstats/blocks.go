package blockstats

import (
	"math"
	"math/big"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

func Write(ev *types.Event) []*write.Point {
	flags := make(map[string]any)

	flags["block_id"] = ev.Block.ID.String()
	flags["total_score"] = ev.Block.TotalScore
	flags["pos_active"] = ev.HayabusaStatus.Active
	flags["best_block_number"] = ev.Block.Number
	flags["block_gas_used"] = ev.Block.GasUsed
	flags["block_gas_limit"] = ev.Block.GasLimit
	flags["block_gas_usage"] = float64(ev.Block.GasUsed) * config.GasDivisor / float64(ev.Block.GasLimit)
	flags["storage_size"] = ev.Block.Size
	flags["block_signer"] = ev.Block.Signer.String()
	gap := float64(config.BlockIntervalSeconds)
	if ev.Prev != nil {
		gap = float64(ev.Block.Timestamp - ev.Prev.Timestamp)
		slotsSinceLastBlock := (ev.Block.Timestamp - ev.Prev.Timestamp + uint64(config.BlockIntervalSeconds-1)) / uint64(config.BlockIntervalSeconds)
		missedSlots := slotsSinceLastBlock - 1
		flags["recent_missed_slots"] = missedSlots
	}

	flags["block_mine_gap"] = (gap - float64(config.BlockIntervalSeconds)) / float64(config.BlockIntervalSeconds)

	// NewExtension code to capture block base fee in wei.
	// Since block.Block.BaseFee is a *big.Int, we'll store its string representation.
	if ev.Block.BaseFeePerGas != nil {
		baseFee := (*big.Int)(ev.Block.BaseFeePerGas)

		flags["block_base_fee"] = baseFee.String()
		totalBurnt := new(big.Int).Mul(baseFee, big.NewInt(int64(ev.Block.GasUsed)))
		totalBurntFloat := new(big.Float).SetInt(totalBurnt)
		divisor := new(big.Float).SetFloat64(math.Pow10(config.VETDecimals))
		totalBurntFloat.Quo(totalBurntFloat, divisor)
		totalBurntFinal, _ := totalBurntFloat.Float64()
		flags["block_total_burnt"] = totalBurntFinal

		totalTip := big.NewInt(0)
		for _, transaction := range ev.Block.Transactions {
			totalTip.Add(totalTip, (*big.Int)(transaction.Reward))
		}
		flags["block_total_total_tip"] = totalTip
	} else {
		flags["block_base_fee"] = "0"
	}

	tags := make(map[string]string)
	tags["signer"] = ev.Block.Signer.String()

	p := influxdb2.NewPoint(config.BlockStatsMeasurement, tags, flags, ev.Timestamp)
	return []*write.Point{p}
}
