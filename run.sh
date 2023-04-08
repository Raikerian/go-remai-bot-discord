#!/bin/bash

set -e

IMAGE_NAME="${PWD##*/}"

docker build -t "${IMAGE_NAME}" .

docker run -it --rm --name remai -e DISCORD_TOKEN="$(cat ~/Dev/discord_token)" -e OPENAI_TOKEN="$(cat ~/Dev/openai_token)" "${IMAGE_NAME}"
