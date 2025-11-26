package grafana

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/cmd/thorflux"
	"github.com/vechain/thorflux/config"
)

// TestSetup provides a test fixture with running Thor, InfluxDB, and Thorflux containers.
type TestSetup struct {
	cmd    *thorflux.Cmd
	test   *testing.T
	db     influxdb2.Client
	client *thorclient.Client
	bucket *domain.Bucket
}

// TestOptions configures the test environment.
type TestOptions struct {
	ThorURL  string // mandatory, the thor node URL to make http requests to
	EndBlock uint64 // optional, last block to index. Defaults to best
	Blocks   uint32 // optional, number of blocks to index. Defaults to 4 hours
}

// Predefined test options for public networks
var (
	TestnetURL = "https://testnet.vechain.org"
	MainnetURL = "https://mainnet.vechain.org"
)

// NewTestSetup creates a new test fixture with Thor, InfluxDB, and Thorflux containers.
func NewTestSetup(t *testing.T, opts TestOptions) *TestSetup {
	if opts.ThorURL == "" {
		t.Fatal("ThorURL must be provided in TestOptions")
	}
	if opts.EndBlock == 0 {
		t.Fatal("EndBlock must be provided in TestOptions")
	}
	if opts.Blocks == 0 {
		opts.Blocks = 360 * 4
	}

	// Get the host-accessible URL for the test
	influx := influxdb2.NewClient(config.DefaultInfluxDB, config.DefaultInfluxToken)
	if ok, err := influx.Ping(t.Context()); !ok || err != nil {
		t.Skip("Skipping test since InfluxDB is not reachable at", config.DefaultInfluxDB)
	}
	client := thorclient.New(opts.ThorURL)

	org, err := influx.OrganizationsAPI().FindOrganizationByName(t.Context(), "vechain")
	require.NoError(t, err)

	// recreate the bucket
	bucket, err := influx.BucketsAPI().FindBucketByName(t.Context(), t.Name())
	if bucket != nil && bucket.Id != nil {
		require.NoError(t, influx.BucketsAPI().DeleteBucketWithID(t.Context(), *bucket.Id))
	}
	bucket, err = influx.BucketsAPI().CreateBucketWithNameWithID(t.Context(), *org.Id, t.Name())
	require.NoError(t, err)

	cmd, err := thorflux.New(t.Context(), thorflux.Options{
		ThorURL:      opts.ThorURL,
		Blocks:       uint64(opts.Blocks),
		InfluxURL:    config.DefaultInfluxDB,
		InfluxToken:  config.DefaultInfluxToken,
		EndBlock:     opts.EndBlock,
		InfluxBucket: bucket.Name,
		InfluxOrg:    config.DefaultInfluxOrg,
	})
	require.NoError(t, err)

	setup := &TestSetup{
		db:     influx,
		test:   t,
		client: client,
		bucket: bucket,
		cmd:    cmd,
	}

	go cmd.Publisher().Run(t.Context())
	cmd.Subscriber().Subscribe(t.Context())

	return setup
}

// WaitForBest waits until InfluxDB has indexed up to the best block.
func (ts *TestSetup) WaitForBest() {
	best, err := ts.client.Block("best")
	require.NoError(ts.test, err)

	dbBest, err := ts.cmd.InfluxDB().Latest()
	require.NoError(ts.test, err)

	for dbBest < best.Number {
		best, err = ts.client.Block("best")
		require.NoError(ts.test, err)

		dbBest, err = ts.cmd.InfluxDB().Latest()
		require.NoError(ts.test, err)
		time.Sleep(100 * time.Millisecond)
	}
}

func (ts *TestSetup) Query(query string) (*api.QueryTableResult, error) {
	queryAPI := ts.db.QueryAPI(config.DefaultInfluxOrg)
	result, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		slog.Error("failed to execute query", "error", err, "query", query)
		return nil, err
	}
	return result, nil
}

// Client returns the Thor client for blockchain queries.
func (ts *TestSetup) Client() *thorclient.Client {
	return ts.client
}

// SubstituteOverrides allows overriding time range and window period in queries.
type SubstituteOverrides struct {
	StartPeriod  string // eg "-30m"
	EndPeriod    string // eg "now()"
	WindowPeriod string // eg "10s"
}

// SubstituteVariables replaces Grafana template variables in a query with actual values.
// This allows testing Grafana dashboard queries with real data.
func (ts *TestSetup) SubstituteVariables(query string, overrides *SubstituteOverrides) string {
	best, err := ts.client.Block("best")
	require.NoError(ts.test, err)

	variableReplacements := map[string]string{
		"${staker}":                       best.Signer.String(),
		"${proposer}":                     best.Signer.String(),
		"${selected_block}":               strconv.FormatUint(uint64(best.Number-thor.EpochLength()), 10),
		"${manual_block}":                 strconv.FormatUint(uint64(best.Number-thor.EpochLength()), 10),
		"${bucket}":                       ts.bucket.Name,
		"${vet_price}":                    "0.02",
		"${vtho_price}":                   "0.001",
		"${epoch_length}":                 "180",
		"${block_interval}":               "10",
		"${seeder_interval}":              "8640",
		"${validator_eviction_threshold}": "60480",
		"${low_staking_period}":           "60480",
		"${medium_staking_period}":        "259200",
		"${high_staking_period}":          "777600",
		"${cooldown_period}":              "60480",
		"${hayabusa_tp}":                  "1500",
		"${hayabusa_fork_block}":          "11000000",
		"${amount_of_epochs}":             "5",
		"${datasource}":                   "InfluxDB",
		"${region}":                       "eu-west-1",
		"${color}":                        "blue",
		"${group}":                        "dev-pn",
	}

	result := query
	for placeholder, replacement := range variableReplacements {
		if strings.Contains(query, placeholder) {
			result = strings.ReplaceAll(result, placeholder, replacement)
		}
	}

	if overrides == nil {
		overrides = &SubstituteOverrides{}
	}
	if overrides.StartPeriod != "" {
		result = strings.ReplaceAll(result, "v.timeRangeStart", overrides.StartPeriod)
	} else {
		result = strings.ReplaceAll(result, "v.timeRangeStart", "-24h")
	}
	if overrides.EndPeriod != "" {
		result = strings.ReplaceAll(result, "v.timeRangeStop", overrides.EndPeriod)
	} else {
		result = strings.ReplaceAll(result, "v.timeRangeStop", "now()")
	}
	if overrides.WindowPeriod != "" {
		result = strings.ReplaceAll(result, "v.windowPeriod", overrides.WindowPeriod)
	} else {
		result = strings.ReplaceAll(result, "v.windowPeriod", "1h")
	}

	return result
}
