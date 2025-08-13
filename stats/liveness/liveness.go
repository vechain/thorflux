package liveness

import (
	"context"
	"log/slog"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/types"
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
	epoch := ev.Block.Number / config.EpochLength

	flags := make(map[string]any)

	currentEpoch := (ev.Block.Number / config.EpochLength) * config.EpochLength
	esitmatedFinalized := currentEpoch - (config.EpochLength * 2)
	esitmatedJustified := currentEpoch - config.EpochLength
	flags["current_epoch"] = currentEpoch
	flags["epoch"] = epoch
	flags["current_block"] = ev.Block.Number

	// if blockTime is within the 3 mins, call to chain for the real finalized block
	if time.Since(ev.Timestamp) < config.ForkDetectionTimeout {
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
			flags["liveness"] = (currentEpoch - finalized.Number) / config.EpochLength
		}
	} else {
		flags["finalized"] = esitmatedFinalized
		flags["justified_block"] = esitmatedJustified
		flags["liveness"] = (currentEpoch - esitmatedFinalized) / config.EpochLength
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultTimeout)
	defer cancel()
	p := influxdb2.NewPoint(config.LivenessMeasurement, ev.DefaultTags, flags, ev.Timestamp)

	return ev.WriteAPI.WritePoint(ctx, p)
}
