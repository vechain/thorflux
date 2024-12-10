package influxdb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/darrenvechain/thor-go-sdk/client"
	"github.com/darrenvechain/thor-go-sdk/thorgo"
	"github.com/darrenvechain/thor-go-sdk/transaction"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/vechain/thorflux/accounts"
	"github.com/vechain/thorflux/block"
	innerRlp "github.com/vechain/thorflux/rlp"
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
	  |> filter(fn: (r) => r["_measurement"] == "block_stats")
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
func (i *DB) WriteBlock(block *block.Block) {
	defer i.prevBlock.Store(block.ExpandedBlock)

	if block.ExpandedBlock.Number%1000 == 0 {
		slog.Info("🪣 saving results to bucket", "number", block.ExpandedBlock.Number)
	}

	writeAPI := i.client.WriteAPIBlocking("vechain", "vechain")

	tags := map[string]string{
		"chain_tag":    string(i.chainTag),
		"signer":       block.ExpandedBlock.Signer.Hex(),
		"block_number": strconv.FormatUint(block.ExpandedBlock.Number, 10),
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
	flags["best_block_number"] = block.Number
	flags["block_gas_used"] = block.GasUsed
	flags["block_gas_limit"] = block.GasLimit
	flags["block_gas_usage"] = float64(block.GasUsed) * 100 / float64(block.GasLimit)
	flags["storage_size"] = block.Size
	gap := uint64(10)
	prev, ok := i.prevBlock.Load().(*client.ExpandedBlock)
	if ok {
		gap = block.Timestamp - prev.Timestamp
	}
	flags["block_mine_gap"] = (gap - 10) / 10
}

func (i *DB) appendB3trStats(block *client.ExpandedBlock, flags map[string]interface{}) {
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

func (i *DB) generateSeed(parentID common.Hash) (seed []byte, err error) {
	blockNum := binary.BigEndian.Uint32(parentID[:]) + 1
	epoch := blockNum / 8640
	seedNum := (epoch - 1) * 8640

	seedBlock, err := i.thor.Blocks.ByNumber(uint64(seedNum))
	if err != nil {
		return
	}
	seedID := seedBlock.ID

	rawBlock := innerRlp.JSONRawBlockSummary{}
	client.Get(i.thor.Client(), "/blocks/"+hex.EncodeToString(seedID.Bytes())+"?raw=true", &rawBlock)
	data, err := hex.DecodeString(rawBlock.Raw[2:])
	if err != nil {
		panic(err)
	}
	header := innerRlp.Header{}
	err = rlp.DecodeBytes(data, &header)
	if err != nil {
		return
	}

	return header.Beta()
}

type Candidate struct {
	Master    common.Address
	Endorsor  common.Address
	Indentity []byte
	Active    bool
}

func listAllCandidates(thorClient *thorgo.Thor, blockNumber uint64) ([]Candidate, error) {
	gas := uint64(3000000)
	gasPrice := "100000000"
	caller := common.HexToAddress("0x6d95e6dca01d109882fe1726a2fb9865fa41e7aa")
	provedWork := "1000"
	gasPayer := common.HexToAddress("0xd3ae78222beadb038203be21ed5ce7c9b1bff602")
	expiration := uint64(1000)
	blockRef := "0x00000000851caf3c"
	authorityContract := common.HexToAddress("0x841a6556c524d47030762eb14dc4af897e605d9b")

	contract, _ := hex.DecodeString(AuthorityListAll)
	clauses := [2]*transaction.Clause{
		transaction.NewClause(nil).WithData(contract),
		transaction.NewClause(&authorityContract).WithData(common.Hex2Bytes("6f0470aa")),
	}

	url := fmt.Sprintf("/accounts/*?revision=%d", blockNumber)

	type InspectRequest struct {
		Gas        *uint64               `json:"gas,omitempty"`
		GasPrice   *string               `json:"gasPrice,omitempty"`
		Caller     *common.Address       `json:"caller,omitempty"`
		ProvedWork *string               `json:"provedWork,omitempty"`
		GasPayer   *common.Address       `json:"gasPayer,omitempty"`
		Expiration *uint64               `json:"expiration,omitempty"`
		BlockRef   *string               `json:"blockRef,omitempty"`
		Clauses    []*transaction.Clause `json:"clauses"`
	}
	body := InspectRequest{
		Gas:        &gas,
		GasPrice:   &gasPrice,
		Caller:     &caller,
		ProvedWork: &provedWork,
		GasPayer:   &gasPayer,
		Expiration: &expiration,
		BlockRef:   &blockRef,
		Clauses:    clauses[:],
	}

	response, err := client.Post(thorClient.Client(), url, body, new([]client.InspectResponse))
	if err != nil {
		return nil, err
	}

	data := (*response)[1].Data[2:]

	valueType, _ := big.NewInt(0).SetString(data[:64], 16)
	if valueType.Cmp(big.NewInt(32)) != 0 {
		return nil, errors.New("Wrong type returned by the contract")
	}
	data = data[64:]
	amount, _ := big.NewInt(0).SetString(data[:64], 16)
	data = data[64:]

	candidates := make([]Candidate, amount.Uint64(), amount.Uint64())
	for index := uint64(0); index < amount.Uint64(); index++ {
		master := common.HexToAddress(data[24:64])
		data = data[64:]
		endorsor := common.HexToAddress(data[24:64])
		data = data[64:]
		identity, _ := hex.DecodeString(data[:64])
		data = data[64:]

		activeString := data[:64]
		active := true
		if activeString == "0000000000000000000000000000000000000000000000000000000000000000" {
			active = false
		}
		data = data[64:]

		candidate := Candidate{
			Master:    master,
			Endorsor:  endorsor,
			Indentity: identity,
			Active:    active,
		}
		candidates[index] = candidate
	}

	return candidates, nil
}

func shuffleCandidates(candidates []Candidate, seed []byte, blockNumber uint64) []common.Address {
	var num [4]byte
	binary.BigEndian.PutUint32(num[:], uint32(blockNumber))
	var list []struct {
		addr common.Address
		hash common.Hash
	}
	for _, p := range candidates {
		if p.Active {
			list = append(list, struct {
				addr common.Address
				hash common.Hash
			}{
				p.Master,
				innerRlp.Blake2b(seed, num[:], p.Master.Bytes()),
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return bytes.Compare(list[i].hash.Bytes(), list[j].hash.Bytes()) < 0
	})

	shuffled := make([]common.Address, 0, len(list))
	for _, t := range list {
		shuffled = append(shuffled, t.addr)
	}
	return shuffled
}

func (i *DB) appendSlotStats(
	block *block.Block,
	flags map[string]interface{},
	writeAPI api.WriteAPIBlocking,
) {
	blockTime := time.Unix(int64(block.ExpandedBlock.Timestamp), 0).UTC()
	prevBlock, ok := i.prevBlock.Load().(*client.ExpandedBlock)

	epoch := block.ExpandedBlock.Number / 180
	if ok {
		candidates, err := listAllCandidates(i.thor, prevBlock.Number)
		if err != nil {
			return
		}
		seed, _ := i.generateSeed(prevBlock.ID)
		shuffledCandidates := shuffleCandidates(candidates, seed, prevBlock.Number)

		genesisBlockTimestamp := i.thor.Client().GenesisBlock().Timestamp
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

		for a := startSlot; a < slotsSinceLastBlock; a++ {
			rawTime := prevBlock.Timestamp + a*10
			slotTime := time.Unix(int64(rawTime), 0)
			isFilled := (a == slotsSinceLastBlock-1)
			value := 0
			if isFilled {
				value = 1
			} else {
				println("EMPTY SLOT EMPTY SLOT", block.ExpandedBlock.Number)
			}
			p := influxdb2.NewPoint(
				"recent_slots",
				map[string]string{"chain_tag": string(i.chainTag)},
				map[string]interface{}{"filled": value, "epoch": epoch, "proposer": shuffledCandidates[a]},
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
		finalized, err := i.thor.Blocks.Finalized()
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

func (i *DB) appendEpochStats(block *client.ExpandedBlock, flags map[string]interface{}, writeAPI api.WriteAPIBlocking) {
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
			"epoch":       strconv.FormatUint(epoch, 10),
		},
		time.Unix(int64(block.Timestamp), 0),
	)

	if err := writeAPI.WritePoint(context.Background(), heatmapPoint); err != nil {
		slog.Error("Failed to write heatmap point", "error", err)
	}
}
