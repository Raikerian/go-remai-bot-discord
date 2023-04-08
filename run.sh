#!/bin/bash

# This script is meant to be the fastest way of
# getting started with this application. Set up
# your API tokens in the variables below, then
# simply run the script, e.g. `./run.sh`.

set -e

# Set up your API tokens here
DISCORD_TOKEN="$(cat ~/Dev/discord_token)"
OPENAI_TOKEN="$(cat ~/Dev/openai_token)"

IMAGE_NAME="${PWD##*/}"

docker build -t "${IMAGE_NAME}" .
docker run -it --rm --name remai -e DISCORD_TOKEN -e OPENAI_TOKEN "${IMAGE_NAME}"
