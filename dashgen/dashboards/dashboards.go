package dashboards

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

// Dashboard interface that each dashboard must implement
type Dashboard interface {
	Generate() (*dashboard.Dashboard, error)
	Name() string
	OutputFilename() string
}

// DashboardConfig holds configuration for dashboard generation
type DashboardConfig struct {
	InfluxUID string
	// Add more config fields as needed in the future
}

// Layout constants for panel positioning
const (
	HeaderHeight = 2
	PanelHeight  = 9
	FullWidth    = 24
	HalfWidth    = 12
	ThirdWidth   = 8
)

// CreateBucketVariable creates a bucket selection variable that queries InfluxDB buckets
func CreateBucketVariable(influxUID string) *dashboard.QueryVariableBuilder {
	// Create StringOrMap for query
	query := dashboard.StringOrMap{
		String: cog.ToPtr("buckets()"),
	}

	// Create VariableOption for current value
	current := dashboard.VariableOption{
		Selected: cog.ToPtr(false),
		Text: dashboard.StringOrArrayOfString{
			String: cog.ToPtr("mainnet"),
		},
		Value: dashboard.StringOrArrayOfString{
			String: cog.ToPtr("mainnet"),
		},
	}

	return dashboard.NewQueryVariableBuilder("bucket").
		Datasource(dashboard.DataSourceRef{
			Type: cog.ToPtr("influxdb"),
			Uid:  cog.ToPtr(influxUID),
		}).
		Query(query).
		Label("Bucket").
		Current(current).
		Hide(dashboard.VariableHideDontHide).
		Multi(false).
		Refresh(dashboard.VariableRefreshOnTimeRangeChanged)
}
