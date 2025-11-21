package grafana

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/domain"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
)

// TestSetup provides a test fixture with running Thor, InfluxDB, and Thorflux containers.
type TestSetup struct {
	test   *testing.T
	db     influxdb2.Client
	client *thorclient.Client
	bucket *domain.Bucket
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
	binary, ok := os.LookupEnv("THORFLUX_BINARY")
	if !ok {
		t.Skip("THORFLUX_BINARY not set, skipping grafana tests")
	}
	// Get the host-accessible URL for the test
	influx := influxdb2.NewClient(config.DefaultInfluxDB, config.DefaultInfluxToken)
	client := thorclient.New(opts.ThorURL)

	org, err := influx.OrganizationsAPI().FindOrganizationByName(t.Context(), "vechain")
	require.NoError(t, err)

	bucket, err := influx.BucketsAPI().CreateBucketWithNameWithID(t.Context(), *org.Id, t.Name())
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			bucket, err = influx.BucketsAPI().FindBucketByName(t.Context(), t.Name())
			if err != nil {
				t.Fatalf("failed to find existing influxdb bucket: %v", err)
			}
		} else {
			t.Fatalf("failed to create influxdb bucket: %v", err)
		}
	}

	args := []string{
		"--thor-url", opts.ThorURL,
		"--influx-bucket", bucket.Name,
		"--thor-blocks", strconv.Itoa(int(opts.Blocks)),
	}
	if opts.EndBlock != "" {
		args = append(args, "--end-block", opts.EndBlock)
	}

	cmd := exec.Command(binary, args...)

	// Capture stdout and stderr to monitor for completion
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start thorflux: %v", err)
	}

	slog.Info("started thorflux process", "pid", cmd.Process.Pid)

	// Wait for thorflux to finish processing
	syncComplete := make(chan struct{})
	go scanOutput(stdout, stderr, syncComplete)

	select {
	case <-syncComplete:
		slog.Info("thorflux backward sync completed")
	case <-t.Context().Done():
		t.Fatal("test context cancelled while waiting for thorflux to complete sync")
	}

	t.Cleanup(func() {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			slog.Error("failed to send interrupt to thorflux process", "error", err)
		}
		slog.Info("stopping thorflux process")
		if err := cmd.Process.Kill(); err != nil {
			slog.Error("failed to kill thorflux process", "error", err)
		}

		_, err := cmd.Process.Wait()
		if err != nil {
			slog.Error("failed to wait for thorflux process to exit", "error", err)
		}
		slog.Info("thorflux process stopped")
	})

	return &TestSetup{
		db:     influx,
		test:   t,
		client: client,
		bucket: bucket,
	}
}

// scanOutput monitors stdout and stderr for the "backward worker finished" log message
func scanOutput(stdout, stderr io.Reader, done chan struct{}) {
	// Channel to merge lines from both stdout and stderr
	lines := make(chan string, 100)

	var wg sync.WaitGroup
	wg.Add(2)

	// Read from stdout in a goroutine
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			slog.Error("error reading stdout", "error", err)
		}
	}()

	// Read from stderr in a goroutine
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			slog.Error("error reading stderr", "error", err)
		}
	}()

	// Close lines channel when both readers finish
	go func() {
		wg.Wait()
		close(lines)
	}()

	// Process lines from both streams
	signaled := false
	for line := range lines {
		// Signal completion when we see the backward sync complete message
		if !signaled && strings.Contains(line, "backward sync complete") {
			close(done)
			signaled = true
			// Continue reading to prevent blocking the process
		}
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
