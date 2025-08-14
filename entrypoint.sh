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

	influx config create \
		-n token-config \
		-u "$INFLUX_URL" \
		-p "$INFLUX_USERNAME:$INFLUX_PASSWORD" \
		-o "$INFLUX_ORG"

	echo "Checking for existing thorflux-api-token..."
	EXISTING_TOKEN=$(influx auth list \
		--org "$INFLUX_ORG" \
		--json | jq -r '.[] | select(.description == "thorflux-api-token") | .token' | head -1)

	if [[ -n "$EXISTING_TOKEN" && "$EXISTING_TOKEN" != "null" ]]; then
		echo "Found existing thorflux-api-token, reusing it..."
		ALL_ACCESS_TOKEN="$EXISTING_TOKEN"
	else
		echo "No existing thorflux-api-token found, creating new one..."
		ALL_ACCESS_TOKEN=$(influx auth create \
			--org "$INFLUX_ORG" \
			--read-buckets \
			--write-buckets \
			--description "thorflux-api-token" | awk 'NR==2 {print $3}')

		if [[ -z "$ALL_ACCESS_TOKEN" ]]; then
			echo "Error: All Access token not generated."
			exit 1
		fi
	fi

	INFLUX_TOKEN=$ALL_ACCESS_TOKEN /app/thorflux
fi
