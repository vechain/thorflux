FROM golang:1.22-alpine3.20 as builder

# Install dependencies
RUN apk add --no-cache make gcc musl-dev linux-headers

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

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/thorflux /app/thorflux

# This container exposes port 8080 to the outside world
EXPOSE 8080

# Run the binary program produced by `go build`
ENTRYPOINT ["/app/thorflux"]
