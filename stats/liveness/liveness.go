package liveness

import (
	"log/slog"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/thor"
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

func (l *Liveness) Write(ev *types.Event) []*write.Point {
	epoch := ev.Block.Number / thor.EpochLength()

	flags := make(map[string]any)

	currentEpoch := (ev.Block.Number / thor.EpochLength()) * thor.EpochLength()
	esitmatedFinalized := currentEpoch - (thor.EpochLength() * 2)
	esitmatedJustified := currentEpoch - thor.EpochLength()
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
			flags["liveness"] = (currentEpoch - finalized.Number) / thor.EpochLength()
		}
	} else {
		flags["finalized"] = esitmatedFinalized
		flags["justified_block"] = esitmatedJustified
		flags["liveness"] = (currentEpoch - esitmatedFinalized) / thor.EpochLength()
	}

	p := influxdb2.NewPoint(config.LivenessMeasurement, ev.DefaultTags, flags, ev.Timestamp)
	return []*write.Point{p}
}
