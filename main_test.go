package main

import (
	"github.com/darrenvechain/thor-go-sdk/thorgo"
	"log/slog"
	"testing"
	"time"
)

func TestBlocks(t *testing.T) {
	thor, err := thorgo.FromURL("https://mainnet.vechain.org")
	if err != nil {
		t.Fatal(err)
	}

	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			best, err := thor.Client().BestBlock()
			if err != nil {
				continue
			}
			finalized, err := thor.Client().Block("finalized")
			if err != nil {
				continue
			}
			currentEpoch := best.Number / 180 * 180
			finalizedEpoch := currentEpoch - 360

			if finalized.Number != finalizedEpoch {
				slog.Warn("block IS NOT as expected", "expected", finalizedEpoch, "actual", finalized.Number, "best", best.Number)
			} else {
				slog.Info("block expected", "expected", finalizedEpoch, "actual", finalized.Number, "best", best.Number)
			}
		}
	}
}
