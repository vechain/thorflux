start:
	@echo "Starting docker compose..."
	@docker compose up --build -d --wait
	@open http://localhost:3000/d/bdw67xl69siyo1/influxdb-stats?orgId=1

debug-with-thor-port:
	@echo "Debugging docker compose with Thor port: $(PORT)"
	@docker compose up --build -d plugin-builder influxdb grafana --wait
	@go run . --thor-url=http://127.0.0.1:$(PORT) --influx-token=admin-token --thor-blocks=15 --influx-url=http://127.0.0.1:8086 &
	@open http://localhost:3000/dashboards

stop:
	@echo "Stopping docker compose..."
	@docker compose down --remove-orphans --volumes

clean:
	@echo "Cleaning up..."
	@docker compose down --remove-orphans --volumes
	@rm -rf ./volumes