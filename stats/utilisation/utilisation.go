package utilisation

import (
	"strconv"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

func Write(ev *types.Event) []*write.Point {
	epoch := ev.Block.Number / thor.EpochLength()
	blockInEpoch := ev.Block.Number % thor.EpochLength()

	flags := make(map[string]any)

	flags["epoch"] = epoch
	flags["block_in_epoch"] = blockInEpoch

	p := influxdb2.NewPoint(config.BlockspaceUtilizationMeasurement, ev.DefaultTags, map[string]interface{}{
		//"block_in_epoch": blockInEpoch,
		"utilization": float64(ev.Block.GasUsed) * config.GasDivisor / float64(ev.Block.GasLimit),
		"epoch":       strconv.FormatUint(uint64(epoch), 10),
	}, ev.Timestamp)

	return []*write.Point{p}
}
