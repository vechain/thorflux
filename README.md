# thor X influxDB

## Description

This is a simple tool to send VeChain Thor data to an influxDB instance.


## Quick Start

1. Docker Only

```bash
docker compose up
```

2. Debug Mode

- Comment out the thorflux service in `./compose.yaml`, then:

```bash
docker compose up
go run . --thor-url=https://mainnet.vechain.org --influx-token=admin-token --thor-block=1024
```
