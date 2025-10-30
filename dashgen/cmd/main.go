package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/vechain/thorflux/dashgen/dashboards"
	"log"
	"os"
	"path/filepath"
)

var (
	outputDir   = flag.String("output-dir", "./grafana/gen-dashboards", "Output directory for generated dashboards")
	influxUID   = flag.String("influx-uid", "B87265B08D314AF", "InfluxDB data source UID")
	prettyPrint = flag.Bool("pretty", true, "Pretty print JSON output")
)

func main() {
	flag.Parse()

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Create InfluxDB data source reference
	influxDBRef := dashboard.DataSourceRef{
		Type: cog.ToPtr("influxdb"),
		Uid:  cog.ToPtr(*influxUID),
	}
	defaultGrafanaRef := dashboard.DataSourceRef{Type: cog.ToPtr("grafana"), Uid: cog.ToPtr("-- Grafana --")}

	// Get all registered dashboards
	dashboards := []dashboards.Dashboard{
		&dashboards.NetworkOverviewDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.ValidatorsDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.AuthorityOverviewDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.ChainForksDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.SingleAuthViewDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposVthoDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposSlotsDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposFinalityDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposContractEventsDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposHistoricalStakerOverviewDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposSingleValidatorDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
		&dashboards.DposCurrentValidatorOverviewDashboard{InfluxDBRef: influxDBRef, DefaultGrafana: defaultGrafanaRef},
	}
	if len(dashboards) == 0 {
		log.Fatal("No dashboards registered")
	}

	fmt.Printf("Found %d registered dashboard(s)\n", len(dashboards))
	fmt.Printf("Output directory: %s\n", *outputDir)
	fmt.Printf("InfluxDB UID: %s\n", *influxUID)
	fmt.Println()

	// Generate all dashboards
	for i, dashGen := range dashboards {
		fmt.Printf("[%d/%d] Generating %s...\n", i+1, len(dashboards), dashGen.Name())

		// Generate the dashboard
		dashboard, err := dashGen.Generate()
		if err != nil {
			log.Fatalf("Failed to generate dashboard '%s': %v", dashGen.Name(), err)
		}

		// Convert to JSON with proper formatting
		var dashboardJSON []byte
		if *prettyPrint {
			dashboardJSON, err = json.MarshalIndent(dashboard, "", "  ")
		} else {
			dashboardJSON, err = json.Marshal(dashboard)
		}
		if err != nil {
			log.Fatalf("Failed to marshal dashboard '%s': %v", dashGen.Name(), err)
		}

		// Write to file
		outputPath := filepath.Join(*outputDir, dashGen.OutputFilename())
		err = os.WriteFile(outputPath, dashboardJSON, 0644)
		if err != nil {
			log.Fatalf("Failed to write dashboard '%s' to file: %v", dashGen.Name(), err)
		}

		fmt.Printf("✓ Generated: %s\n", outputPath)
	}

	fmt.Printf("\n✓ Successfully generated %d dashboard(s)\n", len(dashboards))
}
