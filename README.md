# thor X influxDB

## Description

This is a simple tool to send VeChain Thor data to an influxDB instance.


## Quick Start

1. Local Run

```bash
make start
```

2. Debug Mode

- Comment out the thorflux service in `./compose.yaml`, then:

```bash
docker compose up
go run . --thor-url=https://mainnet.vechain.org --influx-token=admin-token
```

3. Debug mode along with a dynamic local Thor port (like hayabusa-e2e tests)

- Extension of 2. Run the desired test in your e2e project and get the Thor port (i.e., 65253), then:

```bash
make debug-with-local-thor-port PORT=65253
```

4. Dashboard Generator Development

- When developing dashboard generation features, rebuild the dashgen service and start the stack:

```bash
docker compose build dashgen && docker compose up
```

This will:
- Rebuild the dashgen container with your latest code changes
- Generate all dashboards automatically before Grafana starts
- Enable file watching for automatic dashboard regeneration during development
- Make generated dashboards available at http://localhost:3000

For more details on dashboard development, see [DASHGEN_DOCKER.md](./DASHGEN_DOCKER.md) and [dashgen/README.md](./dashgen/README.md).

5. Cleanup

- This will bring down the Docker containers, delete the volumes folder, kill `go run` processes following the format of this project and delete the thorflux log file

```bash
make clean
```

## Building Grafana Dashboards

In an aim to align dashboards across public and private repositories in the foundation please use the
'Dashboard Template' as the starting point for any new dashboards. This introduces a standardised way
to configure the InfluxDB data source.

*Please see [this](https://vechain.atlassian.net/wiki/x/G4A-W) (**Note:** this will be unavailable to external collaborators)*