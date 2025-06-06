package types

import (
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/vechain/thor/v2/api/blocks"
	"time"
)

type Event struct {
	Block          *blocks.JSONExpandedBlock
	Seed           []byte
	HayabusaForked bool
	DPOSActive     bool
	WriteAPI       api.WriteAPIBlocking
	Prev           *blocks.JSONExpandedBlock
	ChainTag       string
	Genesis        *blocks.JSONCollapsedBlock
	DefaultTags    map[string]string
	Timestamp      time.Time
}
