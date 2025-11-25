package grafana

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPoS_HistoricalStakes(t *testing.T) {
	dashboard, err := ParseDashboard("dpos-historical-staker-overview.json")
	require.NoError(t, err)
	require.NotNil(t, dashboard)

	test := NewTestSetup(t, TestOptions{
		ThorURL:  TestnetURL,
		Blocks:   250,
		EndBlock: 23402949,
	})
	overrides := &SubstituteOverrides{
		StartPeriod:  "-10y",
		EndPeriod:    "-1h",
		WindowPeriod: "1y",
	}

	for _, panel := range dashboard.Panels {
		t.Run(panel.Title, func(t *testing.T) {
			panel.AssertHasResults(test, overrides)
		})
	}
}
