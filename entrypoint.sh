#!/bin/sh

# if INFLUX_TOKEN is set, then run thorflux
if [ -n "$INFLUX_TOKEN" ]; then
	/app/thorflux
else

	# if INFLUX_URL is not set, then throw an error
	if [ -z "$INFLUX_URL" ]; then
		echo "INFLUX_URL is not set. Exiting..."
		exit 1
	fi

	# if INFLUX_USERNAME is not set, then throw an error
	if [ -z "$INFLUX_USERNAME" ]; then
		echo "INFLUX_USERNAME is not set. Exiting..."
		exit 1
	fi

	# if INFLUX_PASSWORD is not set, then throw an error
	if [ -z "$INFLUX_PASSWORD" ]; then
		echo "INFLUX_PASSWORD is not set. Exiting..."
		exit 1
	fi

	# if INFLUX_ORG is not set, then throw an error
	if [ -z "$INFLUX_ORG" ]; then
		echo "INFLUX_ORG is not set. Exiting..."
		exit 1
	fi

	# if INFLUX_BUCKET is not set, then throw an error
	if [ -z "$INFLUX_BUCKET" ]; then
		echo "INFLUX_BUCKET is not set. Exiting..."
		exit 1
	fi

	echo "Generating InfluxDB All Access token..."

	influx config create \
		-n token-config \
		-u "$INFLUX_URL" \
		-p "$INFLUX_USERNAME:$INFLUX_PASSWORD" \
		-o "$INFLUX_ORG"

	RAW_RESPONSE=$(influx auth create \
		--org "$INFLUX_ORG" \
		--read-buckets \
		--write-buckets \
		--description "thorflux-api-token" \
		--json)

	if ! echo "$RAW_RESPONSE" | jq -e . >/dev/null 2>&1; then
		echo "Error: Failed to create InfluxDB All Access token or invalid JSON response."
		echo "Response: $RAW_RESPONSE"
		exit 1
	fi

	ALL_ACCESS_TOKEN=$(echo "$RAW_RESPONSE" | jq -r '.token')

	if [[ -z "$ALL_ACCESS_TOKEN" ]]; then
		echo "Error: All Access token not generated."
		exit 1
	fi

	INFLUX_TOKEN=$ALL_ACCESS_TOKEN /app/thorflux
fi
