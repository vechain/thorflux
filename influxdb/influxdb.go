package influxdb

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/http"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/vechain/thorflux/config"
)

type DB struct {
	client   influxdb2.Client
	bucket   string
	org      string
	writeAPI api.WriteAPI
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func New(url, token string, org string, bucket string) (*DB, error) {
	influx := influxdb2.NewClient(url, token)

	_, err := influx.Ping(context.Background())

	if err != nil {
		slog.Error("failed to ping influxdb", "error", err)
		return nil, err
	}

	writeAPI := influx.WriteAPI(org, bucket)
	writeAPI.SetWriteFailedCallback(func(batch string, error http.Error, retryAttempts uint) bool {
		slog.Warn("failed to write points to influxdb", "error", error, "batch", batch, "retryAttempts", retryAttempts)
		return retryAttempts < 5
	})

	ctx, cancel := context.WithCancel(context.Background())
	db := &DB{
		client:   influx,
		bucket:   bucket,
		org:      org,
		writeAPI: writeAPI,
		ctx:      ctx,
		cancel:   cancel,
	}

	db.wg.Add(1)
	errChan := writeAPI.Errors()
	go func() {
		defer db.wg.Done()

		for {
			select {
			case <-db.ctx.Done():
				slog.Info("influxdb error handler shutting down")
				return

			case err, ok := <-errChan:
				if !ok {
					slog.Info("influxdb error channel closed")
					return
				}
				slog.Error("write error", "error", err)
			}
		}
	}()

	return db, nil
}

// Latest returns the latest block number stored in the database
func (i *DB) Latest() (uint32, error) {
	queryAPI := i.client.QueryAPI(i.org)

	var query strings.Builder
	query.WriteString(`from(bucket: "`)
	query.WriteString(i.bucket)
	query.WriteString(`")
		|> range(start: `)
	query.WriteString(config.DefaultQueryStartDate)
	query.WriteString(`, stop: `)
	query.WriteString(config.DefaultQueryEndDate)
	query.WriteString(`)
		|> filter(fn: (r) => r["_measurement"] == "`)
	query.WriteString(config.BlockStatsMeasurement)
	query.WriteString(`")
		|> filter(fn: (r) => r["_field"] == "`)
	query.WriteString(config.BestBlockNumberField)
	query.WriteString(`")
        |> group()
        |> last()`)
	res, err := queryAPI.Query(context.Background(), query.String())
	if err != nil {
		slog.Warn("failed to query latest block", "error", err)
		return 0, err
	}
	defer func() {
		if err := res.Close(); err != nil {
			slog.Error("Failed to close query result", "error", err)
		}
	}()

	err = res.Err()
	if err != nil {
		slog.Error("error in result", "error", res.Err())
		return 0, err
	}

	if res.Next() {
		blockNum := res.Record().ValueByKey("_value")
		if blockNum == nil {
			return 0, nil
		}
		num, ok := blockNum.(uint64)
		if !ok {
			return 0, fmt.Errorf("unexpected type for block number: %T", blockNum)
		}
		return uint32(num), nil
	}

	return 0, nil
}

func (i *DB) Delete(start, stop time.Time, predicate string) error {
	slog.Info("deleting points from influxdb", "start", start, "stop", stop, "predicate", predicate)
	return i.client.DeleteAPI().DeleteWithName(context.Background(), i.org, i.bucket, start, stop, predicate)
}

func (i *DB) WritePoints(points []*write.Point) {
	for _, p := range points {
		i.writeAPI.WritePoint(p)
	}
}

func (i *DB) Close() error {
	i.cancel()
	i.writeAPI.Flush()
	i.client.Close()

	done := make(chan struct{})
	go func() {
		i.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("influxdb connection closed cleanly")
		return nil
	case <-time.After(config.DefaultTimeout):
		slog.Warn("timeout waiting for influxdb error handler to stop")
		return fmt.Errorf("timeout waiting for goroutine cleanup")
	}
}
