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

func (d *Dashboard) GetVariableByName(name string) (*Variable, bool) {
	for _, variable := range d.Templating.List {
		if variable.Name == name {
			return &variable, true
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

func (v *Variable) AssertHasResults(setup *TestSetup) {
	if v.Datasource.Type != "influxdb" {
		setup.test.Errorf("Expected datasource.Type to be 'influxdb', got '%s'", v.Datasource.Type)
	}
	queryStr, ok := v.Query.(string)
	if !ok {
		return
	}
	query := setup.SubstituteVariables(queryStr)
	res, err := setup.Query(query)
	require.NoError(setup.test, err, "Variable '%s' query failed: %s", v.Name, query)
	require.NoError(setup.test, res.Err(), "Variable '%s' query result error: %s", v.Name, query)
	assert.True(setup.test, res.Next(), "Variable '%s' expected at least one result row for query: %s", v.Name, query)
	require.NoError(setup.test, res.Close())
}

type Panel struct {
	Targets    []Target `json:"targets"`
	Title      string   `json:"title"`
	Datasource struct {
		Type string `json:"type"`
	} `json:"datasource"`
}

func (p *Panel) AssertHasResults(setup *TestSetup) {
	if p.Datasource.Type != "influxdb" {
		setup.test.Errorf("Expected datasource.Type to be 'influxdb', got '%s'", p.Datasource.Type)
	}
	for _, target := range p.Targets {
		query := setup.SubstituteVariables(target.Query)
		res, err := setup.Query(query)
		require.NoError(setup.test, err, "Panel '%s' query failed: %s", p.Title, query)
		require.NoError(setup.test, res.Err(), "Panel '%s' query result error: %s", p.Title, query)
		assert.True(setup.test, res.Next(), "Panel '%s' expected at least one result row for query: %s", p.Title, query)
		require.NoError(setup.test, res.Close())
	}
}

type Target struct {
	Query      string `json:"query"`
	Datasource struct {
		Type string `json:"type"`
	} `json:"datasource"`
}

var dashboardsPath = "config/dashboards"

//go:embed config/dashboards
var dashboardsFS embed.FS

func ParseDashboards() ([]Dashboard, error) {
	var dashboards []Dashboard

	entries, err := dashboardsFS.ReadDir(dashboardsPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := dashboardsFS.ReadFile(dashboardsPath + "/" + entry.Name())
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
	data, err := dashboardsFS.ReadFile(dashboardsPath + "/" + name)
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
