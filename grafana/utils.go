package grafana

import (
	"embed"
	"encoding/json"

	"github.com/vechain/thor/v2/thorclient"
	"github.com/vechain/thorflux/config"
	"github.com/vechain/thorflux/influxdb"
)

type Dashboard struct {
	Panels     []Panel `json:"panels"`
	Templating struct {
		List []Variable `json:"list"`
	} `json:"templating"`
	Title string `json:"title"`
}

type Variable struct {
	Name string `json:"name"`
}

type Panel struct {
	Targets []Target `json:"targets"`
	Title   string   `json:"title"`
}

type Target struct {
	Datasource struct {
		Type string `json:"type"`
	} `json:"datasource"`
	Query string `json:"query"`
}

//go:embed dashboards
var dashboardsFS embed.FS

func ParseDashboards() ([]Dashboard, error) {
	var dashboards []Dashboard

	entries, err := dashboardsFS.ReadDir("dashboards")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := dashboardsFS.ReadFile("dashboards/" + entry.Name())
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
	data, err := dashboardsFS.ReadFile("dashboards/" + name)
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

func SetupTest() (*thorclient.Client, *influxdb.DB) {
	client := thorclient.New(config.DefaultThorURL)
	influx, err := influxdb.New(config.DefaultInfluxDB, config.DefaultInfluxToken, config.DefaultInfluxOrg, config.DefaultInfluxBucket)
	if err != nil {
		panic(err)
	}
	return client, influx
}
