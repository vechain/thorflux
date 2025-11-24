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
	"github.com/vechain/thorflux/cmd/thorflux"
	"github.com/vechain/thorflux/config"
)

var (
	thorFlag        = flag.String("thor-url", "https://testnet.vechain.org", "thor node URL, (env var: THOR_URL)")
	genesisURLFlag  = flag.String("genesis-url", "", "thor genesis node URL, (env var: GENESIS_URL)")
	blocksFlag      = flag.Uint64("thor-blocks", config.DefaultThorBlocks, "number of blocks to sync (best - <thor-blocks>) (env var: THOR_BLOCKS)")
	endBlockFlag    = flag.Uint64("end-block", 0, "thor end block number to stop indexing at (env var: END_BLOCK)")
	influxUrlFlag   = flag.String("influx-url", config.DefaultInfluxDB, "influxdb URL, (env var: INFLUX_URL)")
	influxTokenFlag = flag.String("influx-token", config.DefaultInfluxToken, "influxdb auth token, (env var: INFLUX_TOKEN)")
	influxOrg       = flag.String("influx-org", config.DefaultInfluxOrg, "influxdb organization, (env var: INFLUX_ORG)")
	influxBucket    = flag.String("influx-bucket", config.DefaultInfluxBucket, "influxdb bucket, (env var: INFLUX_BUCKET)")
	ownersRepo      = flag.String("owners-repo-path", "", "owners excel file path repo, (env var: OWNERS_REPO)")
)

func main() {
	thorURL, influxURL, influxToken, err := parseFlags()
	if err != nil {
		slog.Error("failed to parse flags", "error", err)
		flag.PrintDefaults()
		os.Exit(1)
	}
	ctx := exitContext()

	cmd, err := thorflux.New(ctx, thorflux.Options{
		ThorURL:      thorURL,
		GenesisURL:   *genesisURLFlag,
		Blocks:       *blocksFlag,
		EndBlock:     *endBlockFlag,
		InfluxURL:    influxURL,
		InfluxToken:  influxToken,
		InfluxOrg:    *influxOrg,
		InfluxBucket: *influxBucket,
		OwnersRepo:   *ownersRepo,
	})
	if err != nil {
		slog.Error("failed to create thorflux command", "error", err)
		os.Exit(1)
	}
	cmd.Run()
	<-ctx.Done()
}

func parseFlags() (string, string, string, error) {
	if err := envflag.Parse(); err != nil {
		return "", "", "", err
	}

	influxToken := *influxTokenFlag
	if influxToken == "" {
		return "", "", "", errors.New(config.ErrInfluxTokenRequired)
	}

	thorURL := *thorFlag
	if thorURL == config.DefaultThorURL {
		slog.Warn("thor node URL not set via flag or env, using default", "url", config.DefaultThorURL)
	}

	influxURL := *influxUrlFlag
	if influxURL == config.DefaultInfluxDB {
		slog.Warn("influxdb URL not set via flag or env, using default", "url", config.DefaultInfluxDB)
	}

	return thorURL, influxURL, influxToken, nil
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
