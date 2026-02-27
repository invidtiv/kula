#!/usr/bin/env bash

set -e

echo "Running golangci-lint..."
golangci-lint run ./...

echo -e "\nRunning go test..."
go test -v ./...

echo -e "\nAll checks passed!"
