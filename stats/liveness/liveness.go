package liveness

import (
	"context"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/types"
	"log/slog"
	"time"
)

type Liveness struct {
	client *thorclient.Client
}

func New(client *thorclient.Client) *Liveness {
	return &Liveness{
		client: client,
	}
}

func (l *Liveness) Write(ev *types.Event) error {
	epoch := ev.Block.Number / 180

	flags := make(map[string]any)

	currentEpoch := (ev.Block.Number / 180) * 180
	esitmatedFinalized := currentEpoch - 360
	esitmatedJustified := currentEpoch - 180
	flags["current_epoch"] = currentEpoch
	flags["epoch"] = epoch
	flags["current_block"] = ev.Block.Number

	// if blockTime is within the 15 mins, call to chain for the real finalized block
	if time.Since(ev.Timestamp) < time.Minute*3 {
		finalized, err := l.client.Block("finalized")
		if err != nil {
			slog.Error("failed to get finalized block", "error", err)
			flags["finalized"] = esitmatedFinalized
			flags["justified_block"] = esitmatedJustified
			flags["liveness"] = (currentEpoch - esitmatedFinalized) / 180
		} else {
			justified, _ := l.client.Block("justified")
			flags["finalized"] = finalized.Number
			flags["justified_block"] = justified.Number
			flags["liveness"] = (currentEpoch - finalized.Number) / 180
		}
	} else {
		flags["finalized"] = esitmatedFinalized
		flags["justified_block"] = esitmatedJustified
		flags["liveness"] = (currentEpoch - esitmatedFinalized) / 180
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := influxdb2.NewPoint("liveness", ev.DefaultTags, flags, ev.Timestamp)

	return ev.WriteAPI.WritePoint(ctx, p)
}
