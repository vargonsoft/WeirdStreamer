#!/usr/bin/env bash
# Builds the server and runs the local test server from Server/e2e (port 9100, its own config,
# accounts, databases and sample media; git-ignored). The web UI is served from Server/web.
# Android emulators reach it at http://10.0.2.2:9100. Stop it with Ctrl-C.
set -euo pipefail
cd "$(dirname "$0")"
[ -d e2e ] || { echo "Server/e2e is missing (see CLAUDE.md: Test environment)" >&2; exit 1; }
go build -o e2e/server ./cmd/server
cd e2e
exec env CONFIG_PATH=./config.json ./server
