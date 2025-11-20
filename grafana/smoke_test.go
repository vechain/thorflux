package grafana

import (
	_ "embed"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thorflux/config"
)

func TestDashboard_Queries_NoError(t *testing.T) {
	client, db := SetupTest()
	defer require.NoError(t, db.Close())

	best, err := client.Block("best")
	require.NoError(t, err)

	variableReplacements := map[string]string{
		"${staker}":                       best.Signer.String(),
		"${proposer}":                     best.Signer.String(),
		"${selected_block}":               strconv.FormatUint(uint64(best.Number-thor.EpochLength()), 10),
		"${manual_block}":                 strconv.FormatUint(uint64(best.Number-thor.EpochLength()), 10),
		"${bucket}":                       config.DefaultInfluxBucket,
		"${vet_price}":                    "0.02",
		"${vtho_price}":                   "0.001",
		"${epoch_length}":                 "180",
		"${block_interval}":               "10",
		"${seeder_interval}":              "8640",
		"${validator_eviction_threshold}": "60480",
		"${low_staking_period}":           "60480",
		"${medium_staking_period}":        "259200",
		"${high_staking_period}":          "777600",
		"${cooldown_period}":              "60480",
		"${hayabusa_tp}":                  "1500",
		"${hayabusa_fork_block}":          "11000000",
		"${amount_of_epochs}":             "5",
		"${datasource}":                   "InfluxDB",
		"${region}":                       "eu-west-1",
		"${color}":                        "blue",
		"${group}":                        "dev-pn",
	}

	dashboards, err := ParseDashboards()
	if err != nil {
		t.Fatalf("failed to parse dashboards: %v", err)
	}

	totalRequests := 0
	hasResults := 0
	for _, dashboard := range dashboards {
		// test the dashboard variables
		for _, variable := range dashboard.Templating.List {
			query, ok := variable.Query.(string)
			if !ok || variable.Datasource.Type != "influxdb" {
				continue
			}
			for placeholder, replacement := range variableReplacements {
				if strings.Contains(query, placeholder) {
					query = strings.ReplaceAll(query, placeholder, replacement)
				}
			}

			query = strings.ReplaceAll(query, "v.timeRangeStart", "-10h")
			query = strings.ReplaceAll(query, "v.timeRangeStop", "now()")
			query = strings.ReplaceAll(query, "v.windowPeriod", "1m")

			res, err := db.Query(query)
			if err != nil {
				t.Error("failed to execute variable query:", query)
				continue
			}
			assert.True(t, res.Next(), "dashboard variable should have a value:", variable.Query, query)
		}

		// test the dashboard panel queries
		for _, panel := range dashboard.Panels {
			for _, target := range panel.Targets {
				if target.Datasource.Type != "influxdb" {
					continue
				}
				for placeholder, replacement := range variableReplacements {
					if strings.Contains(target.Query, placeholder) {
						target.Query = strings.ReplaceAll(target.Query, placeholder, replacement)
					}
				}

				target.Query = strings.ReplaceAll(target.Query, "v.timeRangeStart", "-1h")
				target.Query = strings.ReplaceAll(target.Query, "v.timeRangeStop", "now()")
				target.Query = strings.ReplaceAll(target.Query, "v.windowPeriod", "1m")

				totalRequests++
				res, err := db.Query(target.Query)
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
