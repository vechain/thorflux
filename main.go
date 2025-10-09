package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/kouhin/envflag"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/pubsub"
)

var (
	thorFlag        = flag.String("thor-url", "https://hayabusa.live.dev.node.vechain.org", "thor node URL, (env var: THOR_URL)")
	genesisURLFlag  = flag.String("genesis-url", "", "thor genesis node URL, (env var: GENESIS_URL)")
	blocksFlag      = flag.Uint64("thor-blocks", config.DefaultThorBlocks, "number of blocks to sync (best - <thor-blocks>) (env var: THOR_BLOCKS)")
	influxUrlFlag   = flag.String("influx-url", config.DefaultInfluxDB, "influxdb URL, (env var: INFLUX_URL)")
	influxTokenFlag = flag.String("influx-token", config.DefaultInfluxToken, "influxdb auth token, (env var: INFLUX_TOKEN)")
	influxOrg       = flag.String("influx-org", config.DefaultInfluxOrg, "influxdb organization, (env var: INFLUX_ORG)")
	influxBucket    = flag.String("influx-bucket", config.DefaultInfluxBucket, "influxdb bucket, (env var: INFLUX_BUCKET)")
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
	defer func() {
		if err := influx.Close(); err != nil {
			slog.Error("error closing influxdb", "error", err)
		}
	}()

	if err := setGenesisConfig(*genesisURLFlag, influx); err != nil {
		slog.Error("failed to set genesis config", "error", err)
		os.Exit(1)
	}

	if *blocksFlag > math.MaxUint32 {
		slog.Error("thor-blocks cannot be greater than max uint32")
		os.Exit(1)
	}

	ctx := exitContext()
	publisher, blockChan, err := pubsub.NewPublisher(thorURL, uint32(*blocksFlag), influx)
	if err != nil {
		slog.Error("failed to create publisher", "error", err)
		os.Exit(1)
	}
	subscriber, err := pubsub.NewSubscriber(thorURL, influx, blockChan)
	if err != nil {
		slog.Error("failed to create subscriber", "error", err)
		os.Exit(1)
	}

	go publisher.Start(ctx)
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
		return "", "", "", 0, errors.New(config.ErrInfluxTokenRequired)
	}

	thorURL := *thorFlag
	if thorURL == config.DefaultThorURL {
		slog.Warn("thor node URL not set via flag or env, using default", "url", config.DefaultThorURL)
	}

	influxURL := *influxUrlFlag
	if influxURL == config.DefaultInfluxDB {
		slog.Warn("influxdb URL not set via flag or env, using default", "url", config.DefaultInfluxDB)
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

func setGenesisConfig(genesisURL string, influx *influxdb.DB) error {
	if genesisURL == "" {
		return nil
	}
	res, err := http.Get(genesisURL)
	if err != nil || res.StatusCode != http.StatusOK {
		return errors.New("failed to fetch genesis config")
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Warn("failed to close genesis response body", "error", err)
		}
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var genesis genesis.CustomGenesis
	if err := json.Unmarshal(body, &genesis); err != nil {
		return err
	}
	if genesis.Config != nil {
		slog.Info("setting custom genesis config", "config", genesis.Config)
		thor.SetConfig(*genesis.Config)
	}

	point := write.NewPoint("chain_config", map[string]string{
		"genesis_url": genesisURL,
	}, map[string]interface{}{
		"block_interval":               thor.BlockInterval(),
		"epoch_length":                 thor.EpochLength(),
		"seeder_interval":              thor.SeederInterval(),
		"validator_eviction_threshold": thor.ValidatorEvictionThreshold(),
		"low_staking_period":           thor.LowStakingPeriod(),
		"medium_staking_period":        thor.MediumStakingPeriod(),
		"high_staking_period":          thor.HighStakingPeriod(),
		"cooldown_period":              thor.CooldownPeriod(),
		"hayabusa_tp":                  thor.HayabusaTP(),
		"fc_hayabusa":                  genesis.ForkConfig.HAYABUSA,
		"fc_galactica":                 genesis.ForkConfig.GALACTICA,
	}, time.Now())

	influx.WritePoints([]*write.Point{point})

	return nil
}
