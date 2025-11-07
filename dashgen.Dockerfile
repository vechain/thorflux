FROM golang:1.24-alpine3.20

# Install dependencies including inotify-tools for file watching
RUN apk add --no-cache make gcc musl-dev inotify-tools

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY dashgen/ ./dashgen/

# Build the dashgen binary
RUN go build -o dashgen-bin ./dashgen/cmd

# Create a script for file watching and dashboard generation
COPY <<EOF /app/watch-and-generate.sh
#!/bin/sh

echo "🚀 Dashboard Generator - Starting..."

# Function to generate dashboards
generate_dashboards() {
    echo "📊 Generating dashboards..."
    
    # Build command with environment variables
    CMD="./dashgen-bin -output-dir /output"
    
    if [ -n "\$INFLUX_UID" ]; then
        CMD="\$CMD -influx-uid \$INFLUX_UID"
    fi
    
    if [ "\$PRETTY_PRINT" = "false" ]; then
        CMD="\$CMD -pretty=false"
    fi
    
    eval \$CMD
    if [ \$? -eq 0 ]; then
        echo "✅ Dashboards generated successfully at \$(date)"
    else
        echo "❌ Dashboard generation failed at \$(date)"
    fi
}

# Initial generation
generate_dashboards

# Check if DEV_MODE is enabled for file watching
if [ "\$DEV_MODE" = "true" ]; then
    echo "🔍 Development mode enabled - watching for file changes..."
    echo "   Watching: /app/dashgen/"
    
    # Watch for changes in Go files and regenerate
    while true; do
        inotifywait -r -e modify,create,delete --include='.*\.go\$' /app/dashgen/ 2>/dev/null
        echo "🔄 File change detected..."
        sleep 1  # Brief debounce
        generate_dashboards
    done
else
    echo "📝 Production mode - dashboards generated once"
    echo "💡 Set DEV_MODE=true to enable file watching"
fi
EOF

# Make the script executable
RUN chmod +x /app/watch-and-generate.sh

# Create output directory
RUN mkdir -p /output

# Default command
CMD ["/app/watch-and-generate.sh"]