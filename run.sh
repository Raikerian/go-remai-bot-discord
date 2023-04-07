#!/bin/bash

set -e

go build

./go-remai-bot-discord -discord-token "$(cat ~/Dev/discord_token)" -openai-token "$(cat ~/Dev/openai_token)"
