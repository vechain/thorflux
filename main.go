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
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/sync"
)

var (
	defaultThorURL  = "http://localhost:8669"
	defaultInfluxDB = "http://localhost:8086"
	thorFlag        = flag.String("thor-url", defaultThorURL, "thor node URL, (env var: THOR_URL)")
	startBlockFlag  = flag.Uint64("thor-start-block", 0, "start block number, (env var: THOR_START_BLOCK)")
	influxUrlFlag   = flag.String("influx-url", defaultInfluxDB, "influxdb URL, (env var: INFLUX_URL)")
	influxTokenFlag = flag.String("influx-token", "", "influxdb auth token, (env var: INFLUX_TOKEN)")
	influxOrg       = flag.String("influx-org", "vechain", "influxdb organization, (env var: INFLUX_ORG)")
	influxBucket    = flag.String("influx-bucket", "vechain", "influxdb bucket, (env var: INFLUX_BUCKET)")
)

func main() {
	thorURL, influxURL, influxToken, startBlock, err := parseFlags()
	if err != nil {
		slog.Error("failed to parse flags", "error", err)
		flag.PrintDefaults()
		os.Exit(1)
	}

	thor := thorclient.New(thorURL)
	chainTag, err := thor.ChainTag()
	if err != nil {
		slog.Error("failed to get chain tag", "error", err)
		os.Exit(1)
	}

	influx, err := influxdb.New(thor, influxURL, influxToken, chainTag, *influxOrg, *influxBucket)
	if err != nil {
		slog.Error("failed to create influxdb", "error", err)
		os.Exit(1)
	}

	prev, err := influx.Latest()
	if err != nil {
		slog.Error("failed to get latest block from DB", "error", err)
		os.Exit(1)
	}
	if prev > startBlock {
		startBlock = prev
	}

	slog.Info("stating block sync", "start", startBlock)

	ctx := exitContext()
	syncer := sync.New(thor, influx, startBlock, ctx)
	syncer.Index()
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

	startBlock := *startBlockFlag

	return thorURL, influxURL, influxToken, uint32(startBlock), nil
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
