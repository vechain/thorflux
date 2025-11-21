package grafana

import (
	"strconv"
	"testing"

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

	overrides := &SubstituteOverrides{
		StartPeriod:  "now-20y",
		EndPeriod:    "now",
		WindowPeriod: "1y",
	}

	panel, ok := dashboard.GetPanelByTitle("😈😈 Missed Slot Leader board")
	require.True(t, ok, "Missed Slot Leader board panel not found")
	panel.AssertHasResults(test, overrides)

	panel, ok = dashboard.GetPanelByTitle("🚨🚨 Missed Slots")
	require.True(t, ok, "Missed Slots panel not found")
	panel.AssertHasResults(test, overrides)
}
