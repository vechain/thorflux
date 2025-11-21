package grafana

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDashboard_Queries_NoError(t *testing.T) {
	test := NewTestSetup(t, TestOptions{
		ThorURL: TestnetURL,
	})

	dashboards, err := ParseDashboards()
	if err != nil {
		t.Fatalf("failed to parse dashboards: %v", err)
	}

	totalRequests := 0
	hasResults := 0
	for _, dashboard := range dashboards {
		for _, panel := range dashboard.Panels {
			for _, target := range panel.Targets {
				if target.Datasource.Type != "influxdb" {
					continue
				}
				query := test.SubstituteVariables(target.Query, nil)

				totalRequests++
				res, err := test.DB().Query(query)
				if err != nil {
					t.Error("failed to execute query:", target.Query)
					continue
				}
				if res.Err() != nil {
					t.Error("query result error:", res.Err(), "for query:", target.Query)
				}
				if res.Next() {
					hasResults++
				}
				require.NoError(t, res.Close())
			}
		}
	}

	t.Logf("Total Requests: %d", totalRequests)
	t.Logf("Queries with Results: %d", hasResults)
	t.Logf("Percent with Results: %.2f%%", (float64(hasResults)/float64(totalRequests))*100)
}
