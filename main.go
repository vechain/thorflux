package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/darrenvechain/thor-go-sdk/thorgo"
	"github.com/darrenvechain/thorflux/influxdb"
	"github.com/darrenvechain/thorflux/sync"
)

var (
	defaultThorURL  = "http://localhost:8669"
	defaultInfluxDB = "http://localhost:8086"
	thorFlag        = flag.String("thor-url", defaultThorURL, "thor node URL")
	influxUrlFlag   = flag.String("influx-url", defaultInfluxDB, "influxdb URL")
	influxTokenFlag = flag.String("influx-token", "", "influxdb auth token")
	startBlock      = flag.Uint64("start-block", 0, "start block number")
)

func main() {
	thorURL, influxURL, influxToken, startBlock, err := parseFlags()
	if err != nil {
		slog.Error("failed to parse flags", "error", err)
		flag.PrintDefaults()
		os.Exit(1)
	}

	thor, err := thorgo.FromURL(thorURL)
	if err != nil {
		slog.Error("failed to create thor client", "error", err)
		os.Exit(1)
	}

	influx, err := influxdb.New(influxURL, influxToken, thor.Client().ChainTag())
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

func parseFlags() (string, string, string, uint64, error) {
	flag.Parse()

	influxToken := *influxTokenFlag
	if influxToken == "" {
		return "", "", "", 0, errors.New("influx token is required")
	}

	thorURL := *thorFlag
	if thorURL == defaultThorURL {
		slog.Warn("using default thor URL, make sure it's correct")
	}

	influxURL := *influxUrlFlag
	if influxURL == defaultInfluxDB {
		slog.Warn("using default influx URL, make sure it's correct")
	}

	startBlock := *startBlock

	return thorURL, influxURL, influxToken, startBlock, nil
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
