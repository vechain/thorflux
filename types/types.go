package types

import (
	"github.com/influxdata/influxdb-client-go/v2/api"
	tapi "github.com/vechain/thor/v2/api"
	"time"
)

type Event struct {
	Block          *tapi.JSONExpandedBlock
	Seed           []byte
	HayabusaForked bool
	DPOSActive     bool
	WriteAPI       api.WriteAPIBlocking
	Prev           *tapi.JSONExpandedBlock
	ChainTag       string
	Genesis        *tapi.JSONCollapsedBlock
	DefaultTags    map[string]string
	Timestamp      time.Time
}
