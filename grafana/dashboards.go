package grafana

import (
	"embed"
	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Dashboard struct {
	Panels     []Panel `json:"panels"`
	Templating struct {
		List []Variable `json:"list"`
	} `json:"templating"`
	Title string `json:"title"`
}

func (d *Dashboard) GetPanelByTitle(title string) (*Panel, bool) {
	for _, panel := range d.Panels {
		if panel.Title == title {
			return &panel, true
		}
	}
	return nil, false
}

type Variable struct {
	Name       string `json:"name"`
	Datasource struct {
		Type string `json:"type"`
	} `json:"datasource"`
	Query any `json:"query"`
}

type Panel struct {
	Targets []Target `json:"targets"`
	Title   string   `json:"title"`
}

func (p *Panel) AssertHasResults(setup *TestSetup, overrides *SubstituteOverrides) {
	for _, target := range p.Targets {
		if target.Datasource.Type != "influxdb" {
			continue
		}
		query := setup.SubstituteVariables(target.Query, overrides)
		res, err := setup.Query(query)
		require.NoError(setup.test, err, "Panel '%s' query failed: %s", p.Title, query)
		require.NoError(setup.test, res.Err(), "Panel '%s' query result error: %s", p.Title, query)
		assert.True(setup.test, res.Next(), "Panel '%s' expected at least one result row for query: %s", p.Title, query)
		require.NoError(setup.test, res.Close())
	}
}

type Target struct {
	Datasource struct {
		Type string `json:"type"`
	} `json:"datasource"`
	Query string `json:"query"`
}

//go:embed config/dashboards
var dashboardsFS embed.FS

func ParseDashboards() ([]Dashboard, error) {
	var dashboards []Dashboard

	entries, err := dashboardsFS.ReadDir("dashboards/config")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := dashboardsFS.ReadFile("dashboards/config/" + entry.Name())
		if err != nil {
			return nil, err
		}

		var dashboard Dashboard
		err = json.Unmarshal(data, &dashboard)
		if err != nil {
			return nil, err
		}

		dashboards = append(dashboards, dashboard)
	}

	return dashboards, nil
}

func ParseDashboard(name string) (*Dashboard, error) {
	data, err := dashboardsFS.ReadFile("dashboards/config/" + name)
	if err != nil {
		return nil, err
	}

	var dashboard Dashboard
	err = json.Unmarshal(data, &dashboard)
	if err != nil {
		return nil, err
	}

	return &dashboard, nil
}
