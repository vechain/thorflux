package grafana

import (
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vechain/thorflux/config"

	"github.com/testcontainers/testcontainers-go"
	tcLog "github.com/testcontainers/testcontainers-go/log"
	dockernet "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const influxHost = "influx"
const influxPort = "8086"

// NewInfluxContainer creates and starts an InfluxDB 2.x container for testing.
// The container is automatically cleaned up when the test completes.
func NewInfluxContainer(t *testing.T, network *testcontainers.DockerNetwork) *testcontainers.DockerContainer {
	ctx := t.Context()
	t.Log("Starting InfluxDB container...")
	influxContainer, err := testcontainers.Run(
		ctx, "influxdb:2",
		dockernet.WithNetwork([]string{influxHost}, network),
		testcontainers.WithExposedPorts(influxPort+"/tcp"),
		testcontainers.WithEnv(map[string]string{
			"DOCKER_INFLUXDB_INIT_MODE":        "setup",
			"DOCKER_INFLUXDB_INIT_USERNAME":    "admin",
			"DOCKER_INFLUXDB_INIT_PASSWORD":    "password",
			"DOCKER_INFLUXDB_INIT_ORG":         config.DefaultInfluxOrg,
			"DOCKER_INFLUXDB_INIT_BUCKET":      config.DefaultInfluxBucket,
			"DOCKER_INFLUXDB_INIT_RETENTION":   "0",
			"DOCKER_INFLUXDB_INIT_ADMIN_TOKEN": config.DefaultInfluxToken,
		}),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/ping").WithPort(influxPort+"/tcp").WithStatusCodeMatcher(func(status int) bool {
			return status == 204
		})),
	)
	require.NoError(t, err)
	testcontainers.CleanupContainer(t, influxContainer)

	t.Log("✅ InfluxDB container started")

	return influxContainer
}

// NewThorfluxContainer creates and starts a Thorflux container for testing.
// The container is built from the local Dockerfile and configured to connect to
// the specified Thor node and InfluxDB instance.
// If genesis is provided, it's mounted into the container for custom network configurations.
func NewThorfluxContainer(
	t *testing.T,
	opts TestOptions,
	network *testcontainers.DockerNetwork,
) *testcontainers.DockerContainer {
	ctx := t.Context()
	t.Log("Starting thorflux container...")
	tcLog.SetDefault(tcLog.TestLogger(t))

	thorfluxContainer, err := testcontainers.Run(ctx, "",
		dockernet.WithNetwork([]string{"thorflux"}, network),
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
			KeepImage:  true,
		}),
		testcontainers.WithWaitStrategyAndDeadline(time.Minute, wait.ForLog("subscriber fully synced")),
		testcontainers.WithEnv(map[string]string{
			"THOR_URL":      opts.ThorURL,
			"INFLUX_TOKEN":  config.DefaultInfluxToken,
			"INFLUX_ORG":    config.DefaultInfluxOrg,
			"INFLUX_BUCKET": config.DefaultInfluxBucket,
			"INFLUX_URL":    "http://" + influxHost + ":" + influxPort,
			"THOR_BLOCKS":   strconv.Itoa(360 * 4), // Sync last 4 hours of blocks
		}),
	)

	// Log container output on test failure for debugging
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		logs, err := thorfluxContainer.Logs(ctx)
		if err != nil {
			t.Logf("failed to get thorflux container logs: %v", err)
			return
		}
		logBytes, err := io.ReadAll(logs)
		if err != nil {
			t.Logf("failed to read thorflux container logs: %v", err)
			return
		}
		t.Logf("thorflux container logs:\n%s", string(logBytes))
	})

	t.Log("✅ Thorflux container started.")
	testcontainers.CleanupContainer(t, thorfluxContainer)
	require.NoError(t, err)
	return thorfluxContainer
}
