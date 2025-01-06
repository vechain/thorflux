start:
	@echo "Starting docker compose..."
	@docker compose up --build -d --wait
	@open http://localhost:3000/d/bdw67xl69siyo1/influxdb-stats?orgId=1

stop:
	@echo "Stopping docker compose..."
	@docker compose down --remove-orphans --volumes

clean:
	@echo "Cleaning up..."
	@docker compose down --remove-orphans --volumes
	@rm -rf ./volumes
