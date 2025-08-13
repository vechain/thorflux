package utilisation

import (
	"context"
	"strconv"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
)

func Write(ev *types.Event) error {
	epoch := ev.Block.Number / config.EpochLength
	blockInEpoch := ev.Block.Number % config.EpochLength

	flags := make(map[string]any)

	flags["epoch"] = epoch
	flags["block_in_epoch"] = blockInEpoch

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultTimeout)
	defer cancel()
	p := influxdb2.NewPoint(config.BlockspaceUtilizationMeasurement, ev.DefaultTags, map[string]interface{}{
		//"block_in_epoch": blockInEpoch,
		"utilization": float64(ev.Block.GasUsed) * config.GasDivisor / float64(ev.Block.GasLimit),
		"epoch":       strconv.FormatUint(uint64(epoch), 10),
	}, ev.Timestamp)

	return ev.WriteAPI.WritePoint(ctx, p)
}
