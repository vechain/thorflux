package grafana

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"

	"github.com/testcontainers/testcontainers-go"
	dockernet "github.com/testcontainers/testcontainers-go/network"
)

// TestSetup provides a test fixture with running Thor, InfluxDB, and Thorflux containers.
type TestSetup struct {
	test     *testing.T
	db       *influxdb.DB
	thorflux *testcontainers.DockerContainer
	influx   *testcontainers.DockerContainer
	client   *thorclient.Client
}

// TestOptions configures the test environment.
type TestOptions struct {
	ThorURL  string // mandatory, the thor node URL to make http requests to
	EndBlock string // optional, last block to index. Defaults to best
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
	if opts.Blocks == 0 {
		opts.Blocks = 360 * 4
	}
	thorfluxImage, ok := os.LookupEnv("THORFLUX_DOCKER_IMAGE")
	if !ok {
		t.Skipf("THORFLUX_DOCKER_IMAGE not found in environment")
	}

	network, err := dockernet.New(t.Context())
	require.NoError(t, err)

	influxContainer := NewInfluxContainer(t, network)
	thorflux := NewThorfluxContainer(t, opts, network, thorfluxImage)

	// Get the host-accessible URL for the test
	influxHost, err := influxContainer.Host(t.Context())
	require.NoError(t, err)
	influxPort, err := influxContainer.MappedPort(t.Context(), "8086/tcp")
	require.NoError(t, err)
	influxHostURL := fmt.Sprintf("http://%s:%s", influxHost, influxPort.Port())

	influx, err := influxdb.New(influxHostURL, config.DefaultInfluxToken, config.DefaultInfluxOrg, config.DefaultInfluxBucket)
	require.NoError(t, err)

	client := thorclient.New(opts.ThorURL)

	return &TestSetup{
		db:       influx,
		test:     t,
		thorflux: thorflux,
		influx:   influxContainer,
		client:   client,
	}
}

// DB returns the InfluxDB client for querying metrics.
func (ts *TestSetup) DB() *influxdb.DB {
	return ts.db
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
		"${bucket}":                       config.DefaultInfluxBucket,
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
