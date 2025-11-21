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

// NewThorfluxContainer creates and starts a Thorflux container for testing.
// The container is built from the local Dockerfile and configured to connect to
// the specified Thor node and InfluxDB instance.
// If genesis is provided, it's mounted into the container for custom network configurations.
func NewThorfluxContainer(
	t *testing.T,
	opts TestOptions,
	network *testcontainers.DockerNetwork,
	thorfluxImage string,
) *testcontainers.DockerContainer {
	ctx := t.Context()
	t.Log("Starting thorflux container...")
	tcLog.SetDefault(tcLog.TestLogger(t))

	env := map[string]string{
		"THOR_URL":      opts.ThorURL,
		"INFLUX_TOKEN":  config.DefaultInfluxToken,
		"INFLUX_ORG":    config.DefaultInfluxOrg,
		"INFLUX_BUCKET": config.DefaultInfluxBucket,
		"INFLUX_URL":    "http://" + influxHost + ":" + influxPort,
		"THOR_BLOCKS":   strconv.Itoa(360 * 4), // Sync last 4 hours of blocks
	}
	if opts.EndBlock != "" {
		env["END_BLOCK"] = opts.EndBlock
	}

	thorfluxContainer, err := testcontainers.Run(ctx, thorfluxImage,
		dockernet.WithNetwork([]string{"thorflux"}, network),
		// TODO: You can uncomment this and replace 'thorfluxImage' with "" to build from local Dockerfile
		//testcontainers.WithDockerfile(testcontainers.FromDockerfile{
		//	Context:    "../",
		//	Dockerfile: "Dockerfile",
		//	KeepImage:  true,
		//}),
		testcontainers.WithWaitStrategyAndDeadline(time.Minute, wait.ForLog("backward sync complete")),
		testcontainers.WithEnv(env),
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
