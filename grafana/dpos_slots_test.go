package grafana

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DPoS_SlotsDashboard(t *testing.T) {
	dashboard, err := ParseDashboard("dpos-slots.json")
	require.NoError(t, err)
	require.NotNil(t, dashboard)

	missedSlotBlock := 23342968

	test := NewTestSetup(t, TestOptions{
		ThorURL:  TestnetURL,
		Blocks:   100,
		EndBlock: strconv.Itoa(missedSlotBlock + 50),
	})

	panels := map[string]bool{
		"😈😈 Missed Slot Leader board": true,
		"🚨🚨 Missed Slots":             true,
	}

	for _, panel := range dashboard.Panels {
		if _, ok := panels[panel.Title]; !ok {
			continue
		}
		t.Logf("Checking panel: %s", panel.Title)
		for _, target := range panel.Targets {
			if target.Datasource.Type != "influxdb" {
				continue
			}
			query := test.SubstituteVariables(target.Query, &SubstituteOverrides{
				StartPeriod:  "now-20y",
				EndPeriod:    "now",
				WindowPeriod: "1y",
			})
			res, err := test.DB().Query(query)
			require.NoError(t, err)
			assert.True(t, res.Next(), "Expected at least one result row in panel '%s'", panel.Title)
		}
	}
}
