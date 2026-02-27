#!/usr/bin/env bash

set -e

# Cross-compile for different architectures

echo "Building for linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o kula ./cmd/kula/

echo "Building for linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o kula-arm64 ./cmd/kula/

echo "Building for linux/riscv64..."
CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o kula-riscv64 ./cmd/kula/

echo "Done!"
