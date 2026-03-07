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

if ! docker image inspect kula:"$VERSION" &>/dev/null; then
    echo "Error: Image kula:$VERSION not found. Run build.sh first."
    exit 1
fi

echo "Tagging and pushing image 'c0m4r/kula:$VERSION' and 'c0m4r/kula:latest'..."

# Tag for Docker Hub
docker tag kula:"$VERSION" c0m4r/kula:"$VERSION"
docker tag kula:"$VERSION" c0m4r/kula:latest

# Login
docker login -u c0m4r

# Push
docker push c0m4r/kula:"$VERSION"
docker push c0m4r/kula:latest

echo "Done!"
