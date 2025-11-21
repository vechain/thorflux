package grafana

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
		// run every panel's query and ensure no errors
		for _, panel := range dashboard.Panels {
			for _, target := range panel.Targets {
				if target.Datasource.Type != "influxdb" {
					continue
				}
				query := test.SubstituteVariables(target.Query, nil)

				totalRequests++
				res, err := test.Query(query)
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

		// run every templated variable query and ensure some results are returned
		for _, variable := range dashboard.Templating.List {
			query, ok := variable.Query.(string)
			if !ok || variable.Datasource.Type != "influxdb" {
				continue
			}

			substitutedQuery := test.SubstituteVariables(query, nil)
			failureMessage := fmt.Sprintf("variable query failed (dashboard= %s): \n %s, \n %s", dashboard.Title, query, substitutedQuery)

			res, err := test.Query(substitutedQuery)
			if err != nil {
				t.Errorf("%s \n error: %v", failureMessage, err)
				continue
			}
			assert.True(t, res.Next(), failureMessage)
		}
	}

	t.Logf("Total Requests: %d", totalRequests)
	t.Logf("Queries with Results: %d", hasResults)
	t.Logf("Percent with Results: %.2f%%", (float64(hasResults)/float64(totalRequests))*100)
}
