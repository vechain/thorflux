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
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/pubsub"
)

var (
	defaultThorURL  = "http://localhost:8669"
	defaultInfluxDB = "http://localhost:8086"
	thorFlag        = flag.String("thor-url", defaultThorURL, "thor node URL, (env var: THOR_URL)")
	blocksFlag      = flag.Uint64("thor-blocks", 360*24*7, "number of blocks to sync (best - <thor-blocks>) (env var: THOR_BLOCKS)")
	influxUrlFlag   = flag.String("influx-url", defaultInfluxDB, "influxdb URL, (env var: INFLUX_URL)")
	influxTokenFlag = flag.String("influx-token", "", "influxdb auth token, (env var: INFLUX_TOKEN)")
	influxOrg       = flag.String("influx-org", "vechain", "influxdb organization, (env var: INFLUX_ORG)")
	influxBucket    = flag.String("influx-bucket", "vechain", "influxdb bucket, (env var: INFLUX_BUCKET)")
)

func main() {
	thorURL, influxURL, influxToken, blocks, err := parseFlags()
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
	)

	influx, err := influxdb.New(influxURL, influxToken, *influxOrg, *influxBucket)
	if err != nil {
		slog.Error("failed to create influxdb", "error", err)
		os.Exit(1)
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

func parseFlags() (string, string, string, uint32, error) {
	if err := envflag.Parse(); err != nil {
		return "", "", "", 0, err
	}

	influxToken := *influxTokenFlag
	if influxToken == "" {
		return "", "", "", 0, errors.New("--influx-token or INFLUX_DB_TOKEN is required")
	}

	thorURL := *thorFlag
	if thorURL == defaultThorURL {
		slog.Warn("thor node URL not set via flag or env, using default", "url", defaultThorURL)
	}

	influxURL := *influxUrlFlag
	if influxURL == defaultInfluxDB {
		slog.Warn("influxdb URL not set via flag or env, using default", "url", defaultInfluxDB)
	}

	blocks := *blocksFlag

	return thorURL, influxURL, influxToken, uint32(blocks), nil
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
