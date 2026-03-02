FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies if you have them (go.mod, go.sum exist)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -buildvcs=false -o kula ./cmd/kula/

# ---- Final Stage ----
FROM alpine:3.19

WORKDIR /app

# Install required tools (e.g., bash for completion script if used, or tzdata)
RUN apk --no-cache add ca-certificates tzdata bash

# Copy the binary from the builder stage
COPY --from=builder /app/kula /usr/local/bin/kula

# Create directories for config and data
RUN mkdir -p /etc/kula /var/lib/kula

# Copy example config (can be used as default)
COPY config.example.yaml /etc/kula/config.example.yaml

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/kula"]
CMD ["serve", "--config", "/etc/kula/config.yaml"]
