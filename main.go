package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kouhin/envflag"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/pubsub"
)

var (
	thorFlag        = flag.String("thor-url", "https://hayabusa.live.dev.node.vechain.org", "thor node URL, (env var: THOR_URL)")
	blocksFlag      = flag.Uint64("thor-blocks", config.DefaultThorBlocks, "number of blocks to sync (best - <thor-blocks>) (env var: THOR_BLOCKS)")
	syncBlockFlag   = flag.Uint64("sync-from-block", 0, "start sync from block height - takes precedence to thor-blocks is set (env var: SYNC_FROM_BLOCK)")
	influxUrlFlag   = flag.String("influx-url", config.DefaultInfluxDB, "influxdb URL, (env var: INFLUX_URL)")
	influxTokenFlag = flag.String("influx-token", config.DefaultInfluxToken, "influxdb auth token, (env var: INFLUX_TOKEN)")
	influxOrg       = flag.String("influx-org", config.DefaultInfluxOrg, "influxdb organization, (env var: INFLUX_ORG)")
	influxBucket    = flag.String("influx-bucket", config.DefaultInfluxBucket, "influxdb bucket, (env var: INFLUX_BUCKET)")
)

func main() {
	thorURL, influxURL, influxToken, blocks, syncFromBlock, err := parseFlags()
	if err != nil {
		slog.Error("failed to parse flags", "error", err)
		flag.PrintDefaults()
		os.Exit(1)
	}
	slog.Info("starting thorflux",
		"thor-url", thorURL,
		"influx-url", influxURL,
		"influx-org", *influxOrg,
		"influx-bucket", *influxBucket,
		"blocks", blocks,
		"sync-from-block", syncFromBlock,
	)

	influx, err := influxdb.New(influxURL, influxToken, *influxOrg, *influxBucket)
	if err != nil {
		slog.Error("failed to create influxdb", "error", err)
		os.Exit(1)
	}

	// if set syncFromBlock will override the thorBlocks number
	if syncFromBlock != 0 {
		block, err := thorclient.New(thorURL).Block("best")
		if err != nil {
			slog.Error("failed to retrieve best block", "error", err)
			os.Exit(1)
		}
		blocks = block.Number - uint32(syncFromBlock)
		slog.Info("syncing from specified block", "syncFromBlock", syncFromBlock, "blocks", blocks)
	}

	ctx := exitContext()
	publisher, blockChan, err := pubsub.New(thorURL, influx, blocks)
	if err != nil {
		slog.Error("failed to create publisher", "error", err)
		os.Exit(1)
	}
	subscriber, err := pubsub.NewSubscriber(thorURL, influx, blockChan)
	if err != nil {
		slog.Error("failed to create subscriber", "error", err)
		os.Exit(1)
	}

	go publisher.Publish(ctx)
	go subscriber.Subscribe(ctx)

	slog.Info("thorflux started")

	<-ctx.Done()
}

func parseFlags() (string, string, string, uint32, uint64, error) {
	if err := envflag.Parse(); err != nil {
		return "", "", "", 0, 0, err
	}

	influxToken := *influxTokenFlag
	if influxToken == "" {
		return "", "", "", 0, 0, errors.New(config.ErrInfluxTokenRequired)
	}

	thorURL := *thorFlag
	if thorURL == config.DefaultThorURL {
		slog.Warn("thor node URL not set via flag or env, using default", "url", config.DefaultThorURL)
	}

	influxURL := *influxUrlFlag
	if influxURL == config.DefaultInfluxDB {
		slog.Warn("influxdb URL not set via flag or env, using default", "url", config.DefaultInfluxDB)
	}

	syncFromBlock := *syncBlockFlag

	blocks := *blocksFlag

	return thorURL, influxURL, influxToken, uint32(blocks), syncFromBlock, nil
}

func exitContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		exitSignalCh := make(chan os.Signal, 1)
		signal.Notify(exitSignalCh, os.Interrupt, syscall.SIGTERM)

		sig := <-exitSignalCh
		slog.Info("exit signal received", "signal", sig)
		cancel()
	}()
	return ctx
}
