#!/usr/bin/env bash
# Runs Weird Streamer without Docker: creates config.json if there isn't one (scripts/create-config.sh),
# builds the binaries if they're missing (scripts/build.sh), then runs the server in this terminal (scripts/live.sh).
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f config.json ]; then
    scripts/create-config.sh
fi
if [ ! -x bin/server ] || [ ! -x bin/cli ]; then
    scripts/build.sh
fi
if ! scripts/check-ffmpeg.sh; then
    echo "ffmpeg is missing features the server needs (listed above)." >&2
    exit 1
fi
exec scripts/live.sh
