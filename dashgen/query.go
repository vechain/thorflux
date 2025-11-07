package dashgen

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/cog/variants"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/text"
)

// InfluxDBQuery represents an InfluxDB Flux query for Grafana
type InfluxDBQuery struct {
	RefId      string                  `json:"refId"`
	Hide       *bool                   `json:"hide,omitempty"`
	QueryType  *string                 `json:"queryType,omitempty"`
	Datasource dashboard.DataSourceRef `json:"datasource"`
	Query      string                  `json:"query"`
}

// NewInfluxDBQuery creates a new InfluxDB query with common defaults
func NewInfluxDBQuery(refId, query string, datasource dashboard.DataSourceRef) InfluxDBQuery {
	return InfluxDBQuery{
		RefId:      refId,
		QueryType:  cog.ToPtr("flux"),
		Datasource: datasource,
		Query:      query,
	}
}

// Build implements the cog.Builder interface
func (q InfluxDBQuery) Build() (variants.Dataquery, error) {
	return q, nil
}

// ImplementsDataqueryVariant implements the Dataquery interface
func (q InfluxDBQuery) ImplementsDataqueryVariant() {}

// DataqueryType returns the dataquery type
func (q InfluxDBQuery) DataqueryType() string {
	return "influxdb"
}

// Equals compares two queries
func (q InfluxDBQuery) Equals(other variants.Dataquery) bool {
	otherQuery, ok := other.(InfluxDBQuery)
	if !ok {
		return false
	}
	return q.RefId == otherQuery.RefId && q.Query == otherQuery.Query
}

// Validate validates the query
func (q InfluxDBQuery) Validate() error {
	return nil
}

// StandardHeaderPanel creates a standard header panel for dashboards
func StandardHeaderPanel(name string, grafanaDatasource dashboard.DataSourceRef) cog.Builder[dashboard.Panel] {
	return text.NewPanelBuilder().
		Title("").
		Datasource(grafanaDatasource).
		Transparent(true).
		GridPos(dashboard.GridPos{X: 0, Y: 0, W: 24, H: 2}).
		Content(StandardHeader(name)).
		Mode(text.TextModeHTML)
}
