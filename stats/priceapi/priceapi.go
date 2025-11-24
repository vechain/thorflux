package priceapi

import (
	_ "embed"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thor/v2/thorclient/bind"
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/types"
)

const Measurement = "price_api"

var (
	priceFeedAddr = thor.MustParseAddress("0x49eC7192BF804Abc289645ca86F1eD01a6C17713")
	vthoID        = thor.MustParseBytes32("0x7674686f2d757364000000000000000000000000000000000000000000000000")
	vetID         = thor.MustParseBytes32("0x7665742d75736400000000000000000000000000000000000000000000000000")
)

// PriceAPI is a special writer that fetches the price data and then deletes old data. We generally don't care about historical price data.
type PriceAPI struct {
	db         *influxdb.DB
	contract   *bind.Contract
	lastUpdate time.Time
	mu         sync.Mutex
}

//go:embed compiled/PriceFeedOracle.abi
var contractABI []byte

func New(db *influxdb.DB) *PriceAPI {
	client := thorclient.New("https://mainnet.vechain.org") // always use mainnet for price feed
	contract, err := bind.NewContract(client, contractABI, &priceFeedAddr)
	if err != nil {
		panic(err)
	}

	return &PriceAPI{
		contract: contract,
		db:       db,
	}
}

func (p *PriceAPI) Write(e *types.Event) []*write.Point {
	p.mu.Lock()
	defer p.mu.Unlock()
	if time.Since(p.lastUpdate) < time.Minute*5 {
		return nil
	}
	vetPrice, err := p.fetchPrice(vetID)
	if err != nil {
		slog.Error("failed to fetch VET price", "error", err)
		return nil
	}
	vthoPrice, err := p.fetchPrice(vthoID)
	if err != nil {
		slog.Error("failed to fetch VTHO price", "error", err)
		return nil
	}

	slog.Info("updating prices", "vet_price", vetPrice, "vtho_price", vthoPrice)

	stop := time.Now().Add(-time.Minute * 20) // now less 20 mins
	start := time.Now().Add(-time.Hour * 24 * 365)
	predicate := fmt.Sprintf(`_measurement="%s"`, Measurement)
	err = p.db.Delete(start, stop, predicate)
	if err != nil {
		slog.Error("failed to delete old price data", "error", err)
	}

	p.lastUpdate = time.Now()

	point := write.NewPoint(Measurement, map[string]string{
		"t": "true",
	}, map[string]any{
		"vet_price":  vetPrice,
		"vtho_price": vthoPrice,
	}, p.lastUpdate)

	return []*write.Point{point}
}

func (p *PriceAPI) fetchPrice(id thor.Bytes32) (float64, error) {
	// uint128 value, uint128 updatedAt
	outArray := make([]**big.Int, 2)
	outArray[0] = new(*big.Int)
	outArray[1] = new(*big.Int)

	err := p.contract.Method("getLatestValue", id).Call().ExecuteInto(&outArray)
	if err != nil {
		return 0, err
	}

	price, _ := new(big.Float).SetInt(*outArray[0]).Float64()
	return price / 1e12, nil
}
