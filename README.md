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
go run . --thor-url=https://mainnet.vechain.org --influx-token=admin-token --thor-block=1024
```

3. Debug mode along with a dynamic Thor port (like hayabusa-e2e tests)

- Extension of 3. Run the desired test in your e2e project and get the Thor port (i.e., 65253), then:

```bash
make debug-with-thor-port PORT=65253
```

4. Cleanup

- This will bring down the Docker containers, delete the volumes folder, kill `go run` processes following the format of this project and delete the thorflux log file

```bash
make clean
```