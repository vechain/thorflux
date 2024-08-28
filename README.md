# thor X influxDB

## Description

This is a simple tool to send VeChain Thor data to an influxDB instance.


## Quick Start

```bash
docker compose up
go run . --thor-url=https://mainnet.vechain.org --influx-token=admin-token --start-block=18800000
```
