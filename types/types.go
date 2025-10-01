package types

import (
	"time"

	tapi "github.com/vechain/thor/v2/api"
)

type Event struct {
	Block          *tapi.JSONExpandedBlock
	Seed           []byte
	HayabusaStatus HayabusaStatus
	Prev           *tapi.JSONExpandedBlock
	ChainTag       string
	Genesis        *tapi.JSONCollapsedBlock
	DefaultTags    map[string]string
	Timestamp      time.Time
}

type HayabusaStatus struct {
	Active bool
	Forked bool
}
