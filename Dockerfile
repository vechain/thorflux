FROM golang:1.23-alpine3.20 as builder

# Install dependencies
RUN apk add --no-cache make gcc musl-dev linux-headers


WORKDIR /app/influxcli

ENV INFLUXDB_VERSION=2.7.5
# wget https://dl.influxdata.com/influxdb/releases/influxdb2-client-2.7.5-linux-amd64.tar.gz
# wget https://dl.influxdata.com/influxdb/releases/influxdb2-client-2.7.5-linux-arm64.tar.gz
# depending on the architecture, download the appropriate version of the influxdb2-client
RUN case "$(uname -m)" in \
    x86_64) wget https://dl.influxdata.com/influxdb/releases/influxdb2-client-${INFLUXDB_VERSION}-linux-amd64.tar.gz && \
            tar xvzf ./influxdb2-client-${INFLUXDB_VERSION}-linux-amd64.tar.gz  ;; \
    aarch64) wget https://dl.influxdata.com/influxdb/releases/influxdb2-client-${INFLUXDB_VERSION}-linux-arm64.tar.gz && \
              tar xvzf ./influxdb2-client-${INFLUXDB_VERSION}-linux-arm64.tar.gz ;; \
    *) echo "Unsupported architecture"; exit 1 ;; \
  esac

# Set the Current Working Directory inside the container
WORKDIR /app
COPY go.mod .
COPY go.sum .

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o thorflux

FROM alpine:3.20

RUN wget https://github.com/stedolan/jq/releases/download/jq-1.6/jq-linux64 && \
    mv jq-linux64 /usr/local/bin/jq && \
    chmod +x /usr/local/bin/jq

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/thorflux /app/thorflux
COPY --from=builder /app/influxcli/influx /usr/local/bin/influx

WORKDIR /app

COPY entrypoint.sh /app/entrypoint.sh

# This container exposes port 8080 to the outside world
EXPOSE 8080

# Run the binary program produced by `go build`
ENTRYPOINT ["sh", "entrypoint.sh"]
