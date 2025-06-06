package blockstats

import (
	"context"
	"math"
	"math/big"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thorflux/types"
)

func Write(ev *types.Event) error {

	flags := make(map[string]any)

	flags["pos_active"] = ev.DPOSActive
	flags["best_block_number"] = ev.Block.Number
	flags["block_gas_used"] = ev.Block.GasUsed
	flags["block_gas_limit"] = ev.Block.GasLimit
	flags["block_gas_usage"] = float64(ev.Block.GasUsed) * 100 / float64(ev.Block.GasLimit)
	flags["storage_size"] = ev.Block.Size
	gap := float64(10)
	if ev.Prev != nil {
		gap = float64(ev.Block.Timestamp - ev.Prev.Timestamp)
		slotsSinceLastBlock := (ev.Block.Timestamp - ev.Prev.Timestamp + 9) / 10
		missedSlots := slotsSinceLastBlock - 1
		flags["recent_missed_slots"] = missedSlots
	}

	flags["block_mine_gap"] = (gap - 10) / 10

	genesisBlockTimestamp := ev.Genesis.Timestamp
	slots := ((ev.Block.Timestamp - genesisBlockTimestamp) / 10) + 1
	flags["slots_per_block"] = slots
	flags["blocks_slots_percentage"] = (float64(ev.Block.Number) / float64(slots)) * 100

	// NewExtension code to capture block base fee in wei.
	// Since block.Block.BaseFee is a *big.Int, we'll store its string representation.
	if ev.Block.BaseFeePerGas != nil {
		baseFee := (*big.Int)(ev.Block.BaseFeePerGas)

		flags["block_base_fee"] = baseFee.String()
		totalBurnt := big.NewInt(0).Mul(baseFee, big.NewInt((int64)(len(ev.Block.Transactions))))
		totalBurntFloat := new(big.Float).SetInt(totalBurnt)
		divisor := new(big.Float).SetFloat64(math.Pow10(18))
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

	p := influxdb2.NewPoint("block_stats", ev.DefaultTags, flags, ev.Timestamp)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return ev.WriteAPI.WritePoint(ctx, p)
}
