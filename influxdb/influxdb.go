package influxdb

import (
	"context"
	"log/slog"
	"math/big"
	"sort"
	"sync/atomic"
	"time"

	"github.com/darrenvechain/thor-go-sdk/thorgo"

	"github.com/darrenvechain/thor-go-sdk/client"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

type DB struct {
	thor      *thorgo.Thor
	client    influxdb2.Client
	chainTag  byte
	prevBlock atomic.Value
}

func New(thor *thorgo.Thor, url, token string, chainTag byte) (*DB, error) {
	influx := influxdb2.NewClient(url, token)

	_, err := influx.Ping(context.Background())

	if err != nil {
		slog.Error("failed to ping influxdb", "error", err)
		return nil, err
	}

	return &DB{
		thor:     thor,
		client:   influx,
		chainTag: chainTag,
	}, nil
}

// Latest returns the latest block number stored in the database
func (i *DB) Latest() (uint64, error) {
	queryAPI := i.client.QueryAPI("vechain")
	query := `from(bucket: "vechain")
	  |> range(start: 2015-01-01T00:00:00Z, stop: 2100-01-01T00:00:00Z)
	  |> filter(fn: (r) => r["_measurement"] == "measurement1")
	  |> filter(fn: (r) => r["_field"] == "block_number")
	  |> last()`
	res, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return 0, err
	}
	defer res.Close()

	if res.Next() {
		blockNum, ok := res.Record().Value().(uint64)
		if !ok {
			slog.Warn("failed to cast block number to uint64")
			return 0, nil
		}
		return blockNum, nil
	}

	err = res.Err()
	if err != nil {
		slog.Error("error in result", "error", res.Err())
		return 0, err
	}

	return 0, nil
}

// WriteBlock writes a block to the database
func (i *DB) WriteBlock(block *client.ExpandedBlock) {
	defer i.prevBlock.Store(block)

	if block.Number%1000 == 0 {
		slog.Info("🪣 saving results to bucket", "number", block.Number)
	}

	writeAPI := i.client.WriteAPIBlocking("vechain", "vechain")

	tags := map[string]string{
		"chain_tag": string(i.chainTag),
		"signer":    block.Signer.Hex(),
	}

	flags := map[string]interface{}{}
	i.appendBlockStats(block, flags)
	i.appendTxStats(block, flags)
	i.appendB3trStats(block, flags)
	i.appendSlotStats(block, flags)

	p := influxdb2.NewPoint("measurement1", tags, flags, time.Unix(int64(block.Timestamp), 0))

	if err := writeAPI.WritePoint(context.Background(), p); err != nil {
		slog.Error("Failed to write point", "error", err)
	}
}

type coefStats struct {
	Average float64
	Max     float64
	Min     float64
	Mode    float64
	Median  float64
	Total   int
}

func (i *DB) appendTxStats(block *client.ExpandedBlock, flags map[string]interface{}) (total, success, failed int) {
	txs := block.Transactions
	clauseCount := 0
	vetTransferCount := 0
	vetTransfersAmount := new(big.Float)
	eventCount := 0

	stats := coefStats{
		Total: len(txs),
	}

	coefs := make([]float64, 0, len(txs))
	coefCount := make(map[float64]int)
	sum := 0

	for _, t := range txs {
		clauseCount += len(t.Clauses)
		coef := t.GasPriceCoef
		coefs = append(coefs, float64(coef))
		coefCount[float64(coef)]++
		sum += int(coef)
		for _, o := range t.Outputs {
			vetTransferCount += len(o.Transfers)
			eventCount += len(o.Events)
			for _, tr := range o.Transfers {
				amountFloat := new(big.Float).SetInt(tr.Amount.Int)
				vetTransfersAmount.Add(vetTransfersAmount, amountFloat)
			}
		}
	}

	if len(coefs) > 0 {
		sort.Slice(coefs, func(i, j int) bool { return coefs[i] < coefs[j] })
		stats.Min = coefs[0]
		stats.Max = coefs[len(coefs)-1]
		stats.Median = coefs[len(coefs)/2]
		stats.Average = float64(sum) / float64(len(coefs))

		mode := float64(0)
		maxCount := 0
		for coef, count := range coefCount {
			if count > maxCount {
				mode = float64(coef)
				maxCount = count
			}
		}
		stats.Mode = mode
	}

	vetAmount, _ := vetTransfersAmount.Quo(vetTransfersAmount, big.NewFloat(1e18)).Float64()

	flags["total_txs"] = stats.Total
	flags["total_clauses"] = clauseCount
	flags["coef_average"] = stats.Average
	flags["coef_max"] = stats.Max
	flags["coef_min"] = stats.Min
	flags["coef_mode"] = stats.Mode
	flags["coef_median"] = stats.Median
	flags["vet_transfers"] = vetTransferCount
	flags["vet_transfers_amount"] = vetAmount

	return
}

func (i *DB) appendBlockStats(block *client.ExpandedBlock, flags map[string]interface{}) {
	flags["block_number"] = block.Number
	flags["block_gas_used"] = block.GasUsed
	flags["block_gas_limit"] = block.GasLimit
	flags["block_gas_usage"] = float64(block.GasUsed) * 100 / float64(block.GasLimit)
	flags["storage_size"] = block.Size
	prev, ok := i.prevBlock.Load().(*client.ExpandedBlock)
	gap := uint64(0)
	if ok {
		gap = block.Timestamp - prev.Timestamp
	} else {
		gap = uint64(10)
	}
	flags["block_mine_gap"] = (gap - 10) / 10
}

func (i *DB) appendB3trStats(block *client.ExpandedBlock, flags map[string]interface{}) {
	b3trTxs, b3trClauses, b3trGas := b3trStats(block)
	flags["b3tr_total_txs"] = b3trTxs
	flags["b3tr_total_clauses"] = b3trClauses
	flags["b3tr_gas_amount"] = b3trGas
	if block.GasUsed > 0 {
		flags["b3tr_gas_percent"] = float64(b3trGas) * 100 / float64(block.GasUsed)
	} else {
		flags["b3tr_gas_percent"] = float64(0)
	}
}

func (i *DB) appendSlotStats(block *client.ExpandedBlock, flags map[string]interface{}) {
	blockTime := time.Unix(int64(block.Timestamp), 0).UTC()

	currentEpoch := block.Number / 180 * 180
	esitmatedFinalized := currentEpoch - 360
	estimatedJustified := currentEpoch - 180
	flags["current_epoch"] = currentEpoch // first block of currentEpoch

	// if blockTime is within the 15 mins, call to chain for the real finalized block
	if time.Since(blockTime) < time.Minute*3 {
		finalized, err := i.thor.Blocks.Finalized()
		if err != nil {
			slog.Error("failed to get finalized block", "error", err)
			flags["finalized"] = esitmatedFinalized
			flags["liveliness"] = (currentEpoch - esitmatedFinalized) / 180
		} else {
			flags["finalized"] = finalized.Number
			flags["liveliness"] = (currentEpoch - finalized.Number) / 180
		}
	} else {
		flags["finalized"] = esitmatedFinalized
		flags["liveliness"] = (currentEpoch - esitmatedFinalized) / 180
	}

	if time.Since(blockTime) < time.Minute*3 {
		justified, err := i.thor.Blocks.Expanded("justified")
		if err != nil {
			slog.Error("failed to get justified block", "error", err)
			flags["justified"] = estimatedJustified
		} else {
			flags["justified"] = justified.Number
		}
	} else {
		flags["justified"] = estimatedJustified
	}

}
