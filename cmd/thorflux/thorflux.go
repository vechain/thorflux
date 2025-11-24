package thorflux

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thor/v2/genesis"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/influxdb"
	"github.com/vechain/thorflux/pubsub"
)

type Cmd struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	publisher  *pubsub.Publisher
	subscriber *pubsub.Subscriber
	influx     *influxdb.DB
}

type Options struct {
	ThorURL      string
	GenesisURL   string
	Blocks       uint64
	EndBlock     uint64
	InfluxURL    string
	InfluxToken  string
	InfluxOrg    string
	InfluxBucket string
	OwnersRepo   string
}

func New(ctx context.Context, opts Options) (*Cmd, error) {
	slog.Info("initializing thorflux",
		"thor-url", opts.ThorURL,
		"influx-url", opts.InfluxURL,
		"influx-org", opts.InfluxOrg,
		"influx-bucket", opts.InfluxBucket,
		"blocks", opts.Blocks,
		"end-block", opts.EndBlock,
	)

	influx, err := influxdb.New(opts.InfluxURL, opts.InfluxToken, opts.InfluxOrg, opts.InfluxBucket)
	if err != nil {
		slog.Error("failed to create influxdb", "error", err)
		return nil, err
	}

	genesisCfg, err := setGenesisConfig(opts.GenesisURL, opts.ThorURL, influx)
	if err != nil {
		slog.Error("failed to set genesis config", "error", err)
		return nil, err
	}

	if opts.Blocks > math.MaxUint32 {
		slog.Error("thor-blocks cannot be greater than max uint32")
		return nil, err
	}

	publisher, blockChan, err := pubsub.NewPublisher(opts.ThorURL, genesisCfg, uint32(opts.Blocks), uint32(opts.EndBlock), influx)
	if err != nil {
		slog.Error("failed to create publisher", "error", err)
		return nil, err
	}
	subscriber, err := pubsub.NewSubscriber(opts.ThorURL, influx, blockChan, opts.OwnersRepo)
	if err != nil {
		slog.Error("failed to create subscriber", "error", err)
		return nil, err
	}

	appCtx, cancel := context.WithCancel(ctx)
	return &Cmd{
		ctx:        appCtx,
		cancel:     cancel,
		publisher:  publisher,
		subscriber: subscriber,
		influx:     influx,
	}, nil
}

// Run starts the publisher and subscriber routines.
func (cmd *Cmd) Run() {
	slog.Info("starting thorflux publisher and subscriber")
	cmd.wg.Go(func() {
		cmd.publisher.Run(cmd.ctx)
	})
	cmd.wg.Go(func() {
		cmd.subscriber.Subscribe(cmd.ctx)
	})
}

func (cmd *Cmd) Stop() error {
	slog.Info("stopping thorflux")
	cmd.cancel()
	cmd.wg.Wait()
	return cmd.influx.Close()
}

func (cmd *Cmd) Publisher() *pubsub.Publisher {
	return cmd.publisher
}

func (cmd *Cmd) Subscriber() *pubsub.Subscriber {
	return cmd.subscriber
}

func (cmd *Cmd) InfluxDB() *influxdb.DB {
	return cmd.influx
}

func setGenesisConfig(genesisURL, thorURL string, influx *influxdb.DB) (*genesis.CustomGenesis, error) {
	var customGenesis *genesis.CustomGenesis
	var err error

	// for custom networks, parse the genesis file
	if genesisURL != "" {
		customGenesis, err = getGenesisFromURL(genesisURL)
		if err != nil {
			return nil, err
		}
		return customGenesis, nil
	}

	// for default networks, create genesis config based on network type
	customGenesis, err = getGenesisFromNetwork(thorURL)
	if err != nil {
		return nil, err
	}

	// write metrics to influxdb
	writeGenesisMetrics(customGenesis, influx)

	return customGenesis, nil
}

func getGenesisFromURL(genesisURL string) (*genesis.CustomGenesis, error) {
	res, err := http.Get(genesisURL)
	if err != nil || res.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch genesis config")
	}
	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Warn("failed to close genesis response body", "error", err)
		}
	}()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var customGenesis genesis.CustomGenesis
	if err := json.Unmarshal(body, &customGenesis); err != nil {
		return nil, err
	}

	return &customGenesis, nil
}

func getGenesisFromNetwork(thorURL string) (*genesis.CustomGenesis, error) {
	var genesisID thor.Bytes32

	// detect network type from URL
	if strings.Contains(thorURL, "testnet") {
		genesisID = thor.MustParseBytes32("0x000000000b2bce3c70bc649a02749e8687721b09ed2e15997f466536b20bb127")
	} else if strings.Contains(thorURL, "mainnet") || thorURL == "https://hayabusa.live.dev.node.vechain.org" {
		genesisID = thor.MustParseBytes32("0x00000000851caf3cfdb6e899cf5958bfb1ac3413d346d43539627e6be7ec1b4a")
	} else {
		return nil, errors.New("network is not mainnet/testnet, please provide a genesis URL")
	}

	fc := thor.GetForkConfig(genesisID)
	if fc == nil {
		return nil, errors.New("failed to get fork config")
	}

	hayabusaTP := thor.HayabusaTP()
	return &genesis.CustomGenesis{
		Config: &thor.Config{
			BlockInterval:              thor.BlockInterval(),
			EpochLength:                thor.EpochLength(),
			SeederInterval:             thor.SeederInterval(),
			ValidatorEvictionThreshold: thor.ValidatorEvictionThreshold(),
			EvictionCheckInterval:      thor.EvictionCheckInterval(),
			LowStakingPeriod:           thor.LowStakingPeriod(),
			MediumStakingPeriod:        thor.MediumStakingPeriod(),
			HighStakingPeriod:          thor.HighStakingPeriod(),
			CooldownPeriod:             thor.CooldownPeriod(),
			HayabusaTP:                 &hayabusaTP,
		},
		ForkConfig: fc,
	}, nil
}

func writeGenesisMetrics(customGenesis *genesis.CustomGenesis, influx *influxdb.DB) {
	if customGenesis.ForkConfig == nil {
		slog.Warn("fork config is nil, skipping metrics")
		return
	}

	point := write.NewPoint("chain_config", map[string]string{
		"block_interval":               strconv.FormatUint(thor.BlockInterval(), 10),
		"epoch_length":                 strconv.FormatUint(uint64(thor.EpochLength()), 10),
		"seeder_interval":              strconv.FormatUint(uint64(thor.SeederInterval()), 10),
		"validator_eviction_threshold": strconv.FormatUint(uint64(thor.ValidatorEvictionThreshold()), 10),
		"low_staking_period":           strconv.FormatUint(uint64(thor.LowStakingPeriod()), 10),
		"medium_staking_period":        strconv.FormatUint(uint64(thor.MediumStakingPeriod()), 10),
		"high_staking_period":          strconv.FormatUint(uint64(thor.HighStakingPeriod()), 10),
		"cooldown_period":              strconv.FormatUint(uint64(thor.CooldownPeriod()), 10),
		"hayabusa_tp":                  strconv.FormatUint(uint64(thor.HayabusaTP()), 10),
		"hayabusa_fork_block":          strconv.FormatUint(uint64(customGenesis.ForkConfig.HAYABUSA), 10),
		"galactica_fork_block":         strconv.FormatUint(uint64(customGenesis.ForkConfig.GALACTICA), 10),
	}, map[string]interface {
	}{
		"null": true,
	}, time.Now())
	influx.WritePoints([]*write.Point{point})
}
