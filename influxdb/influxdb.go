package influxdb

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/vechain/thorflux/pos"

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
		|> filter(fn: (r) => r["_field"] == "best_block_number")
        |> group()
        |> last()`, i.bucket)
	res, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		slog.Warn("failed to query latest block", "error", err)
		return 0, err
	}
	defer res.Close()

	if res.Next() {
		blockNum := res.Record().ValueByKey("block_number")
		if blockNum == nil {
			return 0, nil
		}
		slog.Info("found latest in flux", "result", blockNum)
		num, err := strconv.ParseUint(blockNum.(string), 10, 32)
		if err != nil {
			return 0, err
		}
		return uint32(num), nil
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

	posData := pos.PoSDataExtractor{
		Thor: i.thor,
	}
	isHayabusa := posData.IsHayabusaFork()

	tags := map[string]string{
		"chain_tag":    string(i.chainTag),
		"signer":       block.ExpandedBlock.Signer.String(),
		"block_number": strconv.FormatUint(uint64(block.ExpandedBlock.Number), 10),
	}

	if err := i.candidates.Init(block.ExpandedBlock.ID); err != nil {
		slog.Error("failed to init candidates", "error", err)
	} else {
		slog.Info("candidates reset", "length", i.candidates.Len())
	}

	flags := map[string]interface{}{}
	i.appendBlockStats(block.ExpandedBlock, flags)
	i.appendTxStats(block.ExpandedBlock, flags)
	i.appendB3trStats(block.ExpandedBlock, flags)
	i.appendSlotStats(block, flags, writeAPI)
	i.appendEpochStats(block.ExpandedBlock, flags, writeAPI)

	if isHayabusa {
		i.appendHayabusaEpochStats(block.ExpandedBlock, flags, writeAPI)
		i.appendHayabusaEpochGasStats(block.ExpandedBlock, flags, writeAPI)
		i.appendStakerStats(block.ExpandedBlock, writeAPI)
		flags["pos_active"] = strconv.FormatBool(posData.IsHayabusaActive())
	} else {
		flags["pos_active"] = strconv.FormatBool(isHayabusa)
	}

	p := influxdb2.NewPoint("block_stats", tags, flags, time.Unix(int64(block.ExpandedBlock.Timestamp), 0))

	if err := writeAPI.WritePoint(context.Background(), p); err != nil {
		slog.Error("Failed to write point", "error", err)
	}
}

func (i *DB) appendTxStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}) (total, success, failed int) {
	txs := block.Transactions

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

	flags["total_txs"] = len(txs)
	flags["total_clauses"] = txStat.clauseCount
	flags["vet_transfers"] = txStat.vetTransferCount
	flags["vet_transfers_amount"] = txStat.vetTransfersAmount
	flags["validator_rewards"] = txStat.totalRewards / math.Pow10(18)

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

	// New code to capture block base fee in wei.
	// Since block.ExpandedBlock.BaseFee is a *big.Int, we'll store its string representation.
	if block.BaseFeePerGas != nil {
		baseFee := (*big.Int)(block.BaseFeePerGas)

		flags["block_base_fee"] = baseFee.String()
		totalBurnt := big.NewInt(0).Mul(baseFee, big.NewInt((int64)(len(block.Transactions))))
		totalBurntFloat := new(big.Float).SetInt(totalBurnt)
		divisor := new(big.Float).SetFloat64(math.Pow10(18))
		totalBurntFloat.Quo(totalBurntFloat, divisor)
		totalBurntFinal, _ := totalBurntFloat.Float64()
		flags["block_total_burnt"] = totalBurntFinal

		totalTip := big.NewInt(0)
		for _, transaction := range block.Transactions {
			totalTip.Add(totalTip, (*big.Int)(transaction.Reward))
		}
		flags["block_total_total_tip"] = totalTip
	} else {
		flags["block_base_fee"] = "0"
	}
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
	if epoch <= 1 {
		return
	}
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
		p := influxdb2.NewPoint(
			"recent_slots",
			map[string]string{"chain_tag": string(i.chainTag), "filled": "1", "proposer": proposer.String()},
			map[string]interface{}{"epoch": epoch, "block_number": block.ExpandedBlock.Number},
			time.Unix(int64(block.ExpandedBlock.Timestamp), 0),
		)
		if err := writeAPI.WritePoint(context.Background(), p); err != nil {
			slog.Error("Failed to write recent slot point", "error", err)
		}

		shuffledCandidates, err := i.candidates.Shuffled(prevBlock)
		if err != nil {
			slog.Error("Error shuffling", "err", err.Error())
		}
		for a := startSlot; a < slotsSinceLastBlock-1; a++ {
			rawTime := prevBlock.Timestamp + a*10
			slotTime := time.Unix(int64(rawTime), 0)
			isFilled := a == slotsSinceLastBlock-1
			value := 0
			if isFilled {
				value = 1
			} else {
				slog.Warn("EMPTY SLOT", "number", block.ExpandedBlock.Number)
				if int(a) >= len(shuffledCandidates) {
					slog.Error("Out of bounds", "shuffleCandidates", shuffledCandidates)
					proposer = thor.Address{}
				} else {
					proposer = shuffledCandidates[a]
				}
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

func (i *DB) appendHayabusaEpochStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}, writeAPI api.WriteAPIBlocking) {
	epoch := block.Number / 180
	blockInEpoch := block.Number % 180
	chainTag, err := i.thor.ChainTag()
	posData := pos.PoSDataExtractor{
		Thor: i.thor,
	}

	flags["epoch"] = epoch
	flags["block_in_epoch"] = blockInEpoch

	parsedABI, err := abi.JSON(strings.NewReader(accounts.StakerAbi))
	if err != nil {
		slog.Error("Failed to write hayabusa epoch stats", "error", err)
	}

	totalStakedVet, totalWeightVet, err := posData.FetchStakeWeight(parsedABI, block, chainTag, "totalStake", accounts.StakerContract)
	if err != nil {
		slog.Error("Failed to fetch total stake for hayabusa", "error", err)
	}

	if totalStakedVet == nil || totalStakedVet.Cmp(big.NewInt(0)) <= 0 {
		return
	}

	totalQueuedVet, totalQueuedWeight, err := posData.FetchStakeWeight(parsedABI, block, chainTag, "queuedStake", accounts.StakerContract)
	if err != nil {
		slog.Error("Failed to fetch active stake for hayabusa", "error", err)
	}

	parsedExtensionABI, err := abi.JSON(strings.NewReader(accounts.ExtensionAbi))
	if err != nil {
		slog.Error("Failed to parse extension abi", "error", err)
	}
	totalCirculatingVet, err := posData.FetchAmount(parsedExtensionABI, block, chainTag, "totalSupply", accounts.ExtensionContract)
	if err != nil {
		slog.Error("Failed to fetch total circulating VET", "error", err)
	}

	var candidates []*pos.Candidate
	if blockInEpoch == 0 || len(candidates) == 0 {
		candidates, err = posData.ExtractCandidates(block, chainTag)
		if err != nil {
			slog.Error("Error while fetching validators", "error", err)
		}
	}

	expectedValidator := &thor.Address{}
	if candidates != nil && len(candidates) > 0 {
		expectedValidator, err = i.expectedValidator(candidates, block)
		if err != nil {
			slog.Error("Cannot extract expected validator", "error", err)
		}
	}

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"hayabusa_validators",
		map[string]string{
			"chain_tag": string(i.chainTag),
		},
		map[string]interface{}{
			"total_stake":     big.NewInt(0).Add(totalStakedVet, totalQueuedVet).Int64(),
			"active_stake":    totalStakedVet.Int64(),
			"active_weight":   totalWeightVet.Int64(),
			"queued_stake":    totalQueuedVet.Int64(),
			"queued_weight":   totalQueuedWeight.Int64(),
			"circulating_vet": totalCirculatingVet.Int64(),
			"next_validator":  expectedValidator.String(),
			"epoch":           strconv.FormatUint(uint64(epoch), 10),
		},
		time.Unix(int64(block.Timestamp), 0),
	)

	if err := writeAPI.WritePoint(context.Background(), heatmapPoint); err != nil {
		slog.Error("Failed to write heatmap point", "error", err)
	}
}

func (i *DB) appendHayabusaEpochGasStats(block *blocks.JSONExpandedBlock, flags map[string]interface{}, writeAPI api.WriteAPIBlocking) {
	epoch := block.Number / 180
	blockInEpoch := block.Number % 180
	chainTag, err := i.thor.ChainTag()
	posData := pos.PoSDataExtractor{
		Thor: i.thor,
	}

	flags["epoch"] = epoch
	flags["block_in_epoch"] = blockInEpoch

	parsedABI, err := abi.JSON(strings.NewReader(accounts.EnergyAbi))
	if err != nil {
		slog.Error("Failed to parse energy abi", "error", err)
	}

	parentBlock, err := i.thor.ExpandedBlock(block.ParentID.String())
	if err != nil {
		slog.Error("Failed to fetch parent block", "error", err)
	}

	totalSupply, err := posData.FetchAmount(parsedABI, block, chainTag, "totalSupply", accounts.EnergyContract)
	if err != nil {
		slog.Error("Failed to fetch energy total supply", "error", err)
	}

	parentTotalSupply, err := posData.FetchAmount(parsedABI, parentBlock, chainTag, "totalSupply", accounts.EnergyContract)
	if err != nil {
		slog.Error("Failed to fetch energy total supply", "error", err)
	}

	totalBurned, err := posData.FetchAmount(parsedABI, block, chainTag, "totalBurned", accounts.EnergyContract)
	if err != nil {
		slog.Error("Failed to fetch energy total burned", "error", err)
	}

	parentTotalBurned, err := posData.FetchAmount(parsedABI, parentBlock, chainTag, "totalBurned", accounts.EnergyContract)
	if err != nil {
		slog.Error("Failed to fetch energy total burned", "error", err)
	}

	if parentTotalSupply == nil || parentTotalSupply.Cmp(big.NewInt(0)) <= 0 || parentTotalBurned == nil || parentTotalBurned.Cmp(big.NewInt(0)) <= 0 {
		return
	}

	vthoIssued := big.NewInt(0).Sub(totalSupply, parentTotalSupply)
	vthoBurned := big.NewInt(0).Sub(totalBurned, parentTotalBurned)

	vthoBurnedDivider := vthoBurned
	if vthoBurned == nil || vthoBurned.Cmp(big.NewInt(0)) == 0 {
		vthoBurnedDivider = big.NewInt(1)
	}

	issuedBurnedRatio, exact := new(big.Rat).Quo(new(big.Rat).SetInt(big.NewInt(0).Abs(vthoIssued)), new(big.Rat).SetInt(vthoBurnedDivider)).Float64()
	if !exact {
		slog.Warn("issued burned ration is truncated")
	}

	validatorsShare := big.NewInt(0).Mul(vthoIssued, big.NewInt(3))
	validatorsShare = validatorsShare.Div(validatorsShare, big.NewInt(10))

	delegatorsShare := big.NewInt(0).Mul(vthoIssued, big.NewInt(7))
	delegatorsShare = delegatorsShare.Div(delegatorsShare, big.NewInt(10))

	// Prepare data for heatmap
	heatmapPoint := influxdb2.NewPoint(
		"hayabusa_gas",
		map[string]string{
			"chain_tag": string(i.chainTag),
		},
		map[string]interface{}{
			"vtho_issued":         vthoIssued.Int64(),
			"vtho_burned":         vthoBurned.Int64(),
			"issued_burned_ratio": issuedBurnedRatio,
			"validators_share":    validatorsShare.Int64(),
			"delegators_share":    delegatorsShare.Int64(),
			"epoch":               strconv.FormatUint(uint64(epoch), 10),
		},
		time.Unix(int64(block.Timestamp), 0),
	)

	if err := writeAPI.WritePoint(context.Background(), heatmapPoint); err != nil {
		slog.Error("Failed to write heatmap point", "error", err)
	}
}

func (i *DB) expectedValidator(candidates []*pos.Candidate, currentBlock *blocks.JSONExpandedBlock) (*thor.Address, error) {
	seed, err := i.generateSeed(currentBlock.ID)
	if err != nil {
		return nil, err
	}
	return pos.ExpectedValidator(candidates, currentBlock, seed)
}

func (i *DB) appendStakerStats(block *blocks.JSONExpandedBlock, writeAPI api.WriteAPIBlocking) {
	stakerStats := NewStakerStats()

	if err := stakerStats.CollectActiveStakers(i.thor, block); err != nil {
		slog.Error("Failed to collect active stakers", "error", err)
	}

	txs := block.Transactions
	for _, tx := range txs {
		for _, output := range tx.Outputs {
			for _, event := range output.Events {
				stakerStats.processEvent(event)
			}
		}
	}

	for _, staker := range stakerStats.AddStaker {
		p := influxdb2.NewPoint(
			"queued_stakers",
			map[string]string{
				"chain_tag": string(i.chainTag),
				"staker":    staker.Master.String(),
			},
			map[string]any{
				"period":        staker.Period,
				"auto_renew":    staker.AutoRenew,
				"staked_amount": staker.Stake,
			},
			time.Unix(int64(block.Timestamp), 0),
		)

		if err := writeAPI.WritePoint(context.Background(), p); err != nil {
			slog.Error("Failed to write point", "error", err)
		}
	}

	for _, staker := range stakerStats.StakersStatus {
		p := influxdb2.NewPoint(
			"stakers_status",
			map[string]string{
				"chain_tag": string(i.chainTag),
				"staker":    staker.Master.String(),
			},
			map[string]any{
				"auto_renew":    staker.AutoRenew,
				"status":        staker.Status.Uint64(),
				"staked_amount": staker.Stake.Uint64(),
			},
			time.Unix(int64(block.Timestamp), 0),
		)

		if err := writeAPI.WritePoint(context.Background(), p); err != nil {
			slog.Error("Failed to write point", "error", err)
		}
	}
}
