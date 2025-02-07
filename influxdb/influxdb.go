package influxdb

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/rlp"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/vechain/thor/v2/api/blocks"
	block2 "github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/accounts"
	"github.com/vechain/thorflux/authority"
	"github.com/vechain/thorflux/block"
)

type DB struct {
	thor       *thorclient.Client
	client     influxdb2.Client
	chainTag   byte
	prevBlock  atomic.Value
	candidates *authority.List
	genesis    *blocks.JSONCollapsedBlock
	bucket     string
	org        string
}

func New(thor *thorclient.Client, url, token string, chainTag byte, org string, bucket string) (*DB, error) {
	influx := influxdb2.NewClient(url, token)

	_, err := influx.Ping(context.Background())

	if err != nil {
		slog.Error("failed to ping influxdb", "error", err)
		return nil, err
	}

	genesis, err := thor.Block("0")
	if err != nil {
		slog.Error("failed to get genesis block", "error", err)
		return nil, err
	}

	return &DB{
		thor:       thor,
		client:     influx,
		chainTag:   chainTag,
		candidates: authority.NewList(thor),
		genesis:    genesis,
		bucket:     bucket,
		org:        org,
	}, nil
}

// Latest returns the latest block number stored in the database
func (i *DB) Latest() (uint32, error) {
	queryAPI := i.client.QueryAPI(i.org)
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: 2015-01-01T00:00:00Z, stop: 2100-01-01T00:00:00Z)
		|> filter(fn: (r) => r["_measurement"] == "block_stats")
		|> filter(fn: (r) => r["_field"] == "block_number")
		|> last()`, i.bucket)
	res, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		return 0, err
	}
	defer res.Close()

	if res.Next() {
		blockNum, ok := res.Record().Value().(uint32)
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

// ResolveFork deletes all the entries in the bucket that has a block time GREATER than the forked block
func (i *DB) ResolveFork(block *block.Block) {
	start := time.Unix(int64(block.ExpandedBlock.Timestamp), 0).Add(time.Second)
	stop := time.Now().Add(time.Hour * 24)
	err := i.client.DeleteAPI().DeleteWithName(context.Background(), i.org, i.bucket, start, stop, "")
	if err != nil {
		slog.Error("failed to delete blocks", "error", err)
		panic(err)
	}
}

// WriteBlock writes a block to the database
func (i *DB) WriteBlock(block *block.Block) {
	defer i.prevBlock.Store(block.ExpandedBlock)
	if block.ForkDetected {
		slog.Warn("fork detected", "block", block.ExpandedBlock.Number)
		i.ResolveFork(block)
		return
	}

	if block.ExpandedBlock.Number%1000 == 0 {
		slog.Info("🪣 saving results to bucket", "number", block.ExpandedBlock.Number)
	}

	writeAPI := i.client.WriteAPIBlocking(i.org, i.bucket)

	tags := map[string]string{
		"chain_tag":    string(i.chainTag),
		"signer":       block.ExpandedBlock.Signer.String(),
		"block_number": strconv.FormatUint(uint64(block.ExpandedBlock.Number), 10),
	}

	if i.candidates.ShouldReset(block.ExpandedBlock) {
		i.candidates.Invalidate()
		if err := i.candidates.Init(block.ExpandedBlock.ID); err != nil {
			slog.Error("failed to init candidates", "error", err)
		} else {
			slog.Info("candidates reset", "length", i.candidates.Len())
		}
	}

	flags := map[string]interface{}{}
	i.appendBlockStats(block.ExpandedBlock, flags)
	i.appendTxStats(block.ExpandedBlock, flags)
	i.appendB3trStats(block.ExpandedBlock, flags)
	i.appendSlotStats(block, flags, writeAPI)
	i.appendEpochStats(block.ExpandedBlock, flags, writeAPI)

	p := influxdb2.NewPoint("block_stats", tags, flags, time.Unix(int64(block.ExpandedBlock.Timestamp), 0))

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

func (i *DB) appendTxStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}) (total, success, failed int) {
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
				amount := (*big.Int)(tr.Amount)
				amountFloat := new(big.Float).SetInt(amount)
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

func (i *DB) appendBlockStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}) {
	flags["best_block_number"] = block.Number
	flags["block_gas_used"] = block.GasUsed
	flags["block_gas_limit"] = block.GasLimit
	flags["block_gas_usage"] = float64(block.GasUsed) * 100 / float64(block.GasLimit)
	flags["storage_size"] = block.Size
	gap := float64(10)
	prev, ok := i.prevBlock.Load().(*blocks.JSONExpandedBlock)
	if ok {
		gap = float64(block.Timestamp - prev.Timestamp)
	}
	flags["block_mine_gap"] = (gap - 10) / 10
}

func (i *DB) appendB3trStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}) {
	b3trTxs, b3trClauses, b3trGas := accounts.B3trStats(block)
	flags["b3tr_total_txs"] = b3trTxs
	flags["b3tr_total_clauses"] = b3trClauses
	flags["b3tr_gas_amount"] = b3trGas
	if block.GasUsed > 0 {
		flags["b3tr_gas_percent"] = float64(b3trGas) * 100 / float64(block.GasUsed)
	} else {
		flags["b3tr_gas_percent"] = float64(0)
	}
}

func (i *DB) generateSeed(parentID thor.Bytes32) (seed []byte, err error) {
	blockNum := binary.BigEndian.Uint32(parentID[:]) + 1
	epoch := blockNum / 8640
	seedNum := (epoch - 1) * 8640

	seedBlock, err := i.thor.Block(fmt.Sprintf("%d", seedNum))
	if err != nil {
		return
	}
	seedID := seedBlock.ID

	rawBlock := blocks.JSONRawBlockSummary{}
	res, status, err := i.thor.RawHTTPClient().RawHTTPGet("/blocks/" + hex.EncodeToString(seedID.Bytes()) + "?raw=true")
	if status != 200 {
		return
	}
	if err = json.Unmarshal(res, &rawBlock); err != nil {
		return
	}
	data, err := hex.DecodeString(rawBlock.Raw[2:])
	if err != nil {
		return
	}
	header := block2.Header{}
	err = rlp.DecodeBytes(data, &header)
	if err != nil {
		return
	}

	return header.Beta()
}

func (i *DB) appendSlotStats(
	block *block.Block,
	flags map[string]interface{},
	writeAPI api.WriteAPIBlocking,
) {
	blockTime := time.Unix(int64(block.ExpandedBlock.Timestamp), 0).UTC()
	prevBlock, ok := i.prevBlock.Load().(*blocks.JSONExpandedBlock)

	epoch := block.ExpandedBlock.Number / 180
	if ok {
		genesisBlockTimestamp := i.genesis.Timestamp
		slots := ((block.ExpandedBlock.Timestamp - genesisBlockTimestamp) / 10) + 1
		flags["slots_per_block"] = slots
		flags["blocks_slots_percentage"] = (float64(block.ExpandedBlock.Number) / float64(slots)) * 100

		// Process recent slots
		slotsSinceLastBlock := (block.ExpandedBlock.Timestamp - prevBlock.Timestamp + 9) / 10
		missedSlots := slotsSinceLastBlock - 1
		flags["recent_missed_slots"] = missedSlots

		// Write detailed slot data for the last hour (360 slots)
		const detailedSlotWindow = 360
		startSlot := uint64(0)
		if slotsSinceLastBlock > detailedSlotWindow {
			startSlot = slotsSinceLastBlock - detailedSlotWindow
		}
		proposer := block.ExpandedBlock.Signer

		for a := startSlot; a < slotsSinceLastBlock; a++ {
			rawTime := prevBlock.Timestamp + a*10
			slotTime := time.Unix(int64(rawTime), 0)
			isFilled := a == slotsSinceLastBlock-1
			value := 0
			if isFilled {
				value = 1
			} else {
				slog.Warn("EMPTY SLOT", "number", block.ExpandedBlock.Number)
				// shuffling the proposer for the block is expensive, only do it if we missed a slot. Otherwise, the signer is he proposer
				shuffledCandidates := i.candidates.Shuffled(prevBlock)
				proposer = shuffledCandidates[a]
			}

			p := influxdb2.NewPoint(
				"recent_slots",
				map[string]string{"chain_tag": string(i.chainTag), "filled": fmt.Sprintf("%d", value), "proposer": proposer.String()},
				map[string]interface{}{"epoch": epoch, "block_number": block.ExpandedBlock.Number},
				slotTime,
			)
			if err := writeAPI.WritePoint(context.Background(), p); err != nil {
				slog.Error("Failed to write recent slot point", "error", err)
			}
		}

		// Aggregate older slot data
		if slotsSinceLastBlock > detailedSlotWindow {
			olderMissedSlots := slotsSinceLastBlock - detailedSlotWindow - 1
			olderFilledSlots := 1 // The previous block
			aggregateTime := time.Unix(int64(prevBlock.Timestamp), 0)

			p := influxdb2.NewPoint(
				"aggregated_slots",
				map[string]string{"chain_tag": string(i.chainTag)},
				map[string]interface{}{
					"missed": olderMissedSlots,
					"filled": olderFilledSlots,
				},
				aggregateTime,
			)
			if err := writeAPI.WritePoint(context.Background(), p); err != nil {
				slog.Error("Failed to write aggregated slot point", "error", err)
			}
		}
	}

	currentEpoch := block.ExpandedBlock.Number / 180 * 180
	esitmatedFinalized := currentEpoch - 360
	esitmatedJustified := currentEpoch - 180
	flags["current_epoch"] = currentEpoch
	flags["epoch"] = epoch

	// if blockTime is within the 15 mins, call to chain for the real finalized block
	if time.Since(blockTime) < time.Minute*3 {
		finalized, err := i.thor.Block("finalized")
		if err != nil {
			slog.Error("failed to get finalized block", "error", err)
			flags["finalized"] = esitmatedFinalized
			flags["justified_block"] = esitmatedJustified
			flags["liveliness"] = (currentEpoch - esitmatedFinalized) / 180
		} else {
			flags["finalized"] = finalized.Number
			flags["justified_block"] = finalized.Number + 180
			flags["liveliness"] = (currentEpoch - finalized.Number) / 180
		}
	} else {
		flags["finalized"] = esitmatedFinalized
		flags["justified_block"] = esitmatedJustified
		flags["liveliness"] = (currentEpoch - esitmatedFinalized) / 180
	}
}

func (i *DB) appendEpochStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}, writeAPI api.WriteAPIBlocking) {
	epoch := block.Number / 180
	blockInEpoch := block.Number % 180

	flags["epoch"] = epoch
	flags["block_in_epoch"] = blockInEpoch

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"blockspace_utilization",
		map[string]string{
			"chain_tag": string(i.chainTag),
		},
		map[string]interface{}{
			//"block_in_epoch": blockInEpoch,
			"utilization": float64(block.GasUsed) * 100 / float64(block.GasLimit),
			"epoch":       strconv.FormatUint(uint64(epoch), 10),
		},
		time.Unix(int64(block.Timestamp), 0),
	)

	if err := writeAPI.WritePoint(context.Background(), heatmapPoint); err != nil {
		slog.Error("Failed to write heatmap point", "error", err)
	}
}
