start:
	@echo "Starting docker compose..."
	@docker compose up --build -d --wait
	@open http://localhost:3000/d/bdw67xl69siyo1/influxdb-stats?orgId=1

dashgen-dev:
	@echo "Starting dashgen development mode..."
	@docker compose build dashgen && docker compose up
	@echo "Dashboard development server running with file watching enabled"

debug-with-local-thor-port:
	@echo "Debugging docker compose with Thor port: $(PORT)"
	@docker compose up --build -d plugin-builder influxdb grafana --wait
	@nohup go run . --thor-url=http://127.0.0.1:$(PORT) --influx-token=admin-token --thor-blocks=15 --influx-url=http://127.0.0.1:8086 > thorflux.log 2>&1 &
	@echo "Logs are being saved to thorflux.log"
	@open http://localhost:3000/dashboards

stop:
	@echo "Stopping docker compose..."
	@docker compose down --remove-orphans --volumes

clean:
	@echo "Cleaning up..."
	@docker compose down --remove-orphans --volumes
	@rm -rf ./volumes
	-@pkill -f "go run . --thor-url"  # Kill any running go process, ignore if none exists
	@rm -f thorflux.log