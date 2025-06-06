package influxdb

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/vechain/thor/v2/api/blocks"
	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/stats/authority"
	"github.com/vechain/thorflux/stats/pos"
)

type DB struct {
	thor       *thorclient.Client
	client     influxdb2.Client
	chainTag   byte
	prevBlock  *atomic.Pointer[blocks.JSONExpandedBlock] // Pointer to the previous block, used for slot calculations
	candidates *authority.List
	genesis    *blocks.JSONCollapsedBlock
	staker     *pos.Staker
	bucket     string
	org        string
}

func New(thor *thorclient.Client, url, token string, chainTag byte, org string, bucket string) (*DB, error) {
	influx := influxdb2.NewClient(url, token)

	_, err := influx.Ping(context.Background())

	if err != nil {
		slog.Error("failed to ping influxdb", "error", err)
		return nil, err
	}

	staker, err := pos.NewStaker(thor)
	if err != nil {
		slog.Error("failed to create staker instance", "error", err)
		return nil, err
	}

	genesis, err := thor.Block("0")
	if err != nil {
		slog.Error("failed to get genesis block", "error", err)
		return nil, err
	}

	prevBlock := &atomic.Pointer[blocks.JSONExpandedBlock]{}

	return &DB{
		thor:       thor,
		client:     influx,
		chainTag:   chainTag,
		candidates: authority.NewList(thor),
		genesis:    genesis,
		bucket:     bucket,
		org:        org,
		staker:     staker,
		prevBlock:  prevBlock,
	}, nil
}

// Latest returns the latest block number stored in the database
func (i *DB) Latest() (uint32, error) {
	queryAPI := i.client.QueryAPI(i.org)
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: 2015-01-01T00:00:00Z, stop: 2100-01-01T00:00:00Z)
		|> filter(fn: (r) => r["_measurement"] == "block_stats")
		|> filter(fn: (r) => r["_field"] == "best_block_number")
        |> group()
        |> last()`, i.bucket)
	res, err := queryAPI.Query(context.Background(), query)
	if err != nil {
		slog.Warn("failed to query latest block", "error", err)
		return 0, err
	}
	defer res.Close()

	if res.Next() {
		blockNum := res.Record().ValueByKey("block_number")
		if blockNum == nil {
			return 0, nil
		}
		slog.Info("found latest in flux", "result", blockNum)
		num, err := strconv.ParseUint(blockNum.(string), 10, 32)
		if err != nil {
			return 0, err
		}
		return uint32(num), nil
	}

	err = res.Err()
	if err != nil {
		slog.Error("error in result", "error", res.Err())
		return 0, err
	}

	return 0, nil
}

// ResolveFork deletes all the entries in the bucket that has a block time GREATER than the forked block
func (i *DB) ResolveFork(start time.Time) {
	start = start.Add(time.Second)
	stop := time.Now().Add(time.Hour * 24)
	err := i.client.DeleteAPI().DeleteWithName(context.Background(), i.org, i.bucket, start, stop, "")
	if err != nil {
		slog.Error("failed to delete blocks", "error", err)
		panic(err)
	}
}

func (i *DB) WriteAPI() api.WriteAPIBlocking {
	return i.client.WriteAPIBlocking(i.org, i.bucket)
}
