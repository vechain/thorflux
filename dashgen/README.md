# Dashboard Generator (dashgen)

A modular command-line tool to generate Grafana dashboards programmatically using the Grafana Foundation SDK. This tool creates VeChain Thor network monitoring dashboards that integrate with your existing InfluxDB data source.

## Features

- ✅ **Modular Architecture**: Each dashboard is a separate Go file with SDK code
- ✅ **Auto-Registration**: Dashboards register themselves automatically via `init()` functions
- ✅ **Bulk Generation**: Process all registered dashboards in one command
- ✅ **Type-Safe**: Uses Grafana Foundation SDK for compile-time validation
- ✅ **InfluxDB Integration**: Configurable data source integration
- ✅ **VeChain Styling**: Branded themes matching existing dashboards
- ✅ **JSON Output**: Compatible with Grafana provisioning

## Architecture

```
thorflux/                            # Root project
├── go.mod                           # Root module dependencies
├── grafana/gen-dashboards/          # Generated dashboard output
└── dashgen/                         # Dashboard generator package
    ├── registry.go                  # Dashboard registration system
    ├── cmd/
    │   └── main.go                  # CLI tool executable
    └── dashboards/
        ├── network_overview.go      # Network overview dashboard
        ├── validators.go            # Validators dashboard  
        └── ... (add more dashboard files)
```

## Usage

### Basic Usage
```bash
go run ./dashgen/cmd
```
This generates all registered dashboards to `./grafana/gen-dashboards/` with default settings.

### Advanced Usage
```bash
go run ./dashgen/cmd \
  -output-dir "/custom/path" \
  -influx-uid "MY_INFLUX_UID" \
  -pretty=false
```

### Available Options
- `-output-dir`: Output directory for generated dashboards (default: "../grafana/gen-dashboards")
- `-influx-uid`: InfluxDB data source UID (default: "B87265B08D314AF")
- `-pretty`: Pretty print JSON output (default: true)

## Adding New Dashboards

To add a new dashboard, create a new file `dashgen/dashboards/<name>.go`:

```go
package dashboards

import (
    "github.com/grafana/grafana-foundation-sdk/go/dashboard"
    "github.com/vechain/thorflux/dashgen"
    // ... other imports
)

type MyCustomDashboard struct{}

func (d MyCustomDashboard) Name() string {
    return "My Custom Dashboard"
}

func (d MyCustomDashboard) OutputFilename() string {
    return "my-custom-dashboard.json"
}

func (d MyCustomDashboard) Generate(config dashgen.DashboardConfig) (*dashboard.Dashboard, error) {
    // Dashboard SDK code here
    builder := dashboard.NewDashboardBuilder("My Custom Dashboard")
    // ... add panels, configure options
    
    dashboardResource, err := builder.Build()
    if err != nil {
        return nil, err
    }
    
    return &dashboardResource, nil
}

func init() {
    dashgen.RegisterDashboard(MyCustomDashboard{})
}
```

The dashboard will be automatically discovered and generated on the next run.

## Dashboard Components

The generated dashboard includes:

1. **Header Panel**: Styled title banner with VeChain branding
2. **Section Headers**: Categorized sections (e.g., "Missed Slots") 
3. **Stat Panels**: Key metrics display with thresholds and colors
4. **Time Series Panels**: Trend visualization for metrics over time

## Integration with Grafana

The generated JSON files are compatible with:

- **Manual Import**: Copy JSON and import via Grafana UI
- **Provisioning**: Place files in Grafana's dashboard provisioning directory
- **API**: Upload via Grafana HTTP API

For your thorflux setup, the generated dashboards are automatically loaded from `/grafana/gen-dashboards/` due to the existing provisioning configuration.

## Examples

### Generate a validators overview dashboard:
```bash
go run main.go -name "Validators Overview" -uid "validators-overview" -output ../grafana/gen-dashboards/validators.json
```

### Generate compact JSON for API upload:
```bash
go run main.go -pretty=false -output dashboard-compact.json
```

### Use with different InfluxDB instance:
```bash
go run main.go -influx-uid "MY_PRODUCTION_INFLUX" -name "Production Network"
```

## Current Implementation

This PoC currently generates:
- Basic dashboard structure with proper metadata
- VeChain-themed styling and layout  
- InfluxDB data source configuration
- Sample panels (text headers, stat panels, time series)

## Future Enhancements

- [ ] Dynamic panel generation based on available InfluxDB metrics
- [ ] Template variable support for filtering
- [ ] Panel positioning and sizing configuration
- [ ] Query generation from InfluxDB schema
- [ ] Multiple dashboard templates
- [ ] Grafana API integration for direct upload

## Dependencies

- [Grafana Foundation SDK (Go)](https://github.com/grafana/grafana-foundation-sdk)
- Go 1.22+

## Development

The tool uses the Grafana Foundation SDK which provides type-safe dashboard construction. All panels and configurations are validated at build time, reducing runtime errors compared to JSON templating approaches.

### Docker Development Workflow

For rapid development and testing, use the Docker Compose integration:

```bash
# Build and start with file watching enabled
docker compose build dashgen && docker compose up
```

This approach provides:
- **Automatic Rebuilds**: Container is rebuilt with latest code changes
- **File Watching**: Automatic dashboard regeneration when Go files change
- **Integrated Stack**: InfluxDB, Grafana, and dashgen services work together
- **Instant Feedback**: Generated dashboards appear in Grafana at http://localhost:3000

#### Development Workflow:
1. Make changes to dashboard files in `dashgen/dashboards/`
2. Save your files - dashboards regenerate automatically
3. View results immediately in Grafana (no restart needed)
4. Iterate quickly with instant feedback

For detailed Docker integration documentation, see [../DASHGEN_DOCKER.md](../DASHGEN_DOCKER.md).

### Local Development

For Go-only development without Docker:

```bash
# Generate dashboards locally
go run ./cmd/main.go

# With custom options
go run ./cmd/main.go -output-dir "/custom/path" -influx-uid "MY_INFLUX_UID"
```