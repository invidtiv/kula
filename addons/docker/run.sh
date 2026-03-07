#!/usr/bin/env bash

set -e

# Check if docker command exists
if ! command -v docker &>/dev/null; then
    echo "Error: docker is not installed."
    exit 1
fi

cd "$(dirname "$0")/../.."

# Read version from VERSION file
VERSION=$(cat VERSION | tr -d '[:space:]')

# Use local image tagged by build.sh
IMAGE="kula:$VERSION"

if ! docker image inspect "$IMAGE" &>/dev/null; then
    # Fallback to latest local tag
    if docker image inspect "kula:latest" &>/dev/null; then
        IMAGE="kula:latest"
    else
        echo "Error: Image $IMAGE or kula:latest not found. Run build.sh first."
        exit 1
    fi
fi

echo "Running $IMAGE (no data persistence)..."

docker run --rm -it \
  --name kula-run \
  --pid host \
  --network host \
  -v /proc:/proc:ro \
  "$IMAGE"
