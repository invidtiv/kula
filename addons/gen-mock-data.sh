#!/bin/bash

CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -buildvcs=false \
    -o gen-mock-data \
    ./cmd/gen-mock-data/
