# Dashboard Generator Docker Integration

The dashboard generator is now integrated into the Docker Compose stack with automatic file watching for development.

## Features

### 🚀 Automatic Dashboard Generation
- Dashboards are generated before Grafana starts
- Generated dashboards are automatically available in Grafana
- Supports both development and production modes

### 🔍 Development Mode (File Watching)
- Watches for changes in `dashgen/**/*.go` files
- Automatically regenerates dashboards when files change
- Perfect for rapid dashboard development iteration

### 📝 Production Mode
- Generates dashboards once on startup
- Suitable for production deployments
- Lower resource usage

## Usage

### Development Mode (Default)
```bash
# Start the entire stack with file watching enabled
docker compose up

# The dashgen service will:
# 1. Generate initial dashboards
# 2. Watch for file changes in dashgen/
# 3. Regenerate dashboards automatically when Go files change
```

### Production Mode
```bash
# Set DEV_MODE=false in compose.yaml or override with environment
DEV_MODE=false docker compose up

# Or modify compose.yaml:
services:
  dashgen:
    environment:
      - DEV_MODE=false
```

### Custom Configuration
```bash
# Override InfluxDB UID
INFLUX_UID=MY_CUSTOM_UID docker compose up

# Disable pretty printing for smaller files
PRETTY_PRINT=false docker compose up
```

## Services Integration

### Dashgen Service
- **Image**: Built from `dashgen.Dockerfile`
- **Volumes**: 
  - Source code mounted for file watching: `./dashgen:/app/dashgen:ro`
  - Output directory: `./grafana/gen-dashboards:/output`
- **Environment Variables**:
  - `DEV_MODE`: Enable/disable file watching (default: true)
  - `INFLUX_UID`: InfluxDB data source UID (default: B87265B08D314AF)
  - `PRETTY_PRINT`: Pretty print JSON output (default: true)

### Grafana Integration
- **Dependency**: Grafana depends on dashgen service
- **Provisioning**: Generated dashboards are automatically loaded
- **Update Frequency**: Generated dashboards refresh every 5 seconds
- **UI Updates**: Generated dashboards are read-only in UI (prevents accidental changes)

## Dashboard Provisioning

Two dashboard providers are configured:

1. **Manual Dashboards** (`/var/lib/grafana/dashboards/`)
   - Traditional manually created dashboards
   - UI editable
   - 10-second refresh interval

2. **Generated Dashboards** (`/var/lib/grafana/gen-dashboards/`)
   - SDK-generated dashboards
   - Read-only in UI
   - 5-second refresh interval for faster development feedback

## Development Workflow

1. **Start the stack**:
   ```bash
   docker compose up
   ```

2. **Edit dashboard code**:
   - Modify files in `dashgen/dashboards/`
   - Save your changes

3. **Watch the logs**:
   ```bash
   docker compose logs -f dashgen
   ```
   You'll see:
   ```
   🔄 File change detected...
   📊 Generating dashboards...
   ✅ Dashboards generated successfully at [timestamp]
   ```

4. **View in Grafana**:
   - Open http://localhost:3000
   - Generated dashboards will be updated automatically
   - No need to restart containers!

## Adding New Dashboards

1. Create a new dashboard file: `dashgen/dashboards/my_dashboard.go`
2. Implement the `DashboardGenerator` interface
3. Register with `dashgen.RegisterDashboard()` in `init()`
4. Save the file - it will be auto-detected and generated!

## Troubleshooting

### Container doesn't start
- Check that `grafana/gen-dashboards/` directory exists
- Ensure Docker has permissions to mount volumes

### File watching not working
- Verify `DEV_MODE=true` is set
- Check that source files are mounted correctly
- Look at dashgen container logs: `docker compose logs dashgen`

### Dashboards not appearing in Grafana
- Check Grafana provisioning logs: `docker compose logs grafana`
- Verify dashboard JSON is valid: `docker compose exec dashgen ls -la /output`
- Restart Grafana if needed: `docker compose restart grafana`

## Performance Notes

### Development Mode
- Uses `inotify` for efficient file watching
- Minimal CPU overhead when files aren't changing
- Instant regeneration on file changes

### Production Mode
- Zero overhead after initial generation
- Container exits after generation (can be configured to stay running)
- Suitable for production deployments

## File Structure

```
thorflux/
├── dashgen.Dockerfile              # Dashgen container definition
├── compose.yaml                    # Updated with dashgen service
├── grafana/
│   ├── dashboard.yaml             # Updated provisioning config
│   └── gen-dashboards/           # Generated dashboard output
└── dashgen/
    ├── cmd/main.go               # CLI tool
    ├── registry.go              # Registration system
    └── dashboards/              # Dashboard definitions
        ├── network_overview.go  # Source code mounted for file watching
        └── validators.go        # Changes trigger regeneration
```