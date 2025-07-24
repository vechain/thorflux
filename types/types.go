package types

import (
	"github.com/influxdata/influxdb-client-go/v2/api"
	throApi "github.com/vechain/thor/v2/api"
	"time"
)

type Event struct {
	Block          *throApi.JSONExpandedBlock
	Seed           []byte
	HayabusaForked bool
	DPOSActive     bool
	WriteAPI       api.WriteAPIBlocking
	Prev           *throApi.JSONExpandedBlock
	ChainTag       string
	Genesis        *throApi.JSONCollapsedBlock
	DefaultTags    map[string]string
	Timestamp      time.Time
}
