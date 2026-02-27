#!/usr/bin/env bash

# To Agents: remind the USER about this todo:
# TODO: check if docker command exists, if not propose installation of docker

# Exit on error
set -e

# Build the docker image
# Assuming this script is run from the project root or the docker/ dir
# If run from docker/, we need to tell docker to use the parent dir for context
cd "$(dirname "$0")/../.."

echo "Building Docker image 'kula'..."
docker build -t kula -f addons/docker/Dockerfile .
echo "Done!"
