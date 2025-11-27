package grafana

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPoS_CurrentOverview(t *testing.T) {
	dashboard, err := ParseDashboard("dpos-current-validator-overview.json")
	require.NoError(t, err)
	require.NotNil(t, dashboard)

	test := NewTestSetup(t, TestOptions{
		ThorURL:  TestnetURL,
		Blocks:   250,
		EndBlock: 23363950,
	})
	overrides := &SubstituteOverrides{
		StartPeriod:  "-10y",
		EndPeriod:    "-1h",
		WindowPeriod: "1y",
	}

	for _, panel := range dashboard.Panels {
		t.Run(panel.Title, func(t *testing.T) {
			for i, target := range panel.Targets {
				if strings.Contains(target.Query, "range(start: -120m)") {
					panel.Targets[i].Query = strings.ReplaceAll(target.Query, "range(start: -120m)", "range(start: -10y)")
				}
			}
			if panel.Title == "Validations Healthy Production Rate" {
				t.Log("Skipping panel 'Validations Healthy Production Rate' due to known data gaps")
			}
			panel.AssertHasResults(test, overrides)
		})
	}
}

func Test_DPoS_Prices(t *testing.T) {
	dashboards := []string{
		"dpos-current-validator-overview.json",
		"dpos-single-validator.json",
		"dpos-historical-staker-overview.json",
	}

	block := uint64(23342968)

	test := NewTestSetup(t, TestOptions{
		ThorURL:  TestnetURL,
		Blocks:   100,
		EndBlock: block + 50,
	})

	for _, dashboard := range dashboards {
		t.Run(dashboard, func(t *testing.T) {
			dashboard, err := ParseDashboard(dashboard)
			require.NoError(t, err)
			require.NotNil(t, dashboard)

			overrides := &SubstituteOverrides{
				StartPeriod:  "-20y",
				EndPeriod:    "now()",
				WindowPeriod: "1y",
			}

			vetVariable, ok := dashboard.GetVariableByName("vet_price")
			require.True(t, ok, "vet_price variable not found")
			vetVariable.AssertHasResults(test, overrides)

			vthoVariable, ok := dashboard.GetVariableByName("vtho_price")
			require.True(t, ok, "vtho_price variable not found")
			vthoVariable.AssertHasResults(test, overrides)
		})
	}
}
