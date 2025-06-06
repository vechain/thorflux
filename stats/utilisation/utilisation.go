package utilisation

import (
	"context"
	"strconv"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thorflux/types"
)

func Write(ev *types.Event) error {
	epoch := ev.Block.Number / 180
	blockInEpoch := ev.Block.Number % 180

	flags := make(map[string]any)

	flags["epoch"] = epoch
	flags["block_in_epoch"] = blockInEpoch

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := influxdb2.NewPoint("blockspace_utilization", ev.DefaultTags, map[string]interface{}{
		//"block_in_epoch": blockInEpoch,
		"utilization": float64(ev.Block.GasUsed) * 100 / float64(ev.Block.GasLimit),
		"epoch":       strconv.FormatUint(uint64(epoch), 10),
	}, ev.Timestamp)

	return ev.WriteAPI.WritePoint(ctx, p)
}
