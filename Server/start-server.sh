#!/usr/bin/env bash
# Runs Weird Streamer without Docker: builds the binaries if they're missing (scripts/build.sh), creates
# config.yml if there isn't one (scripts/create-config.sh, which uses the CLI), then runs the server in this
# terminal (scripts/live.sh).
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -x bin/server ] || [ ! -x bin/cli ]; then
    scripts/build.sh
fi
if [ ! -f config.yml ]; then
    scripts/create-config.sh
fi
if ! scripts/check-ffmpeg.sh; then
    echo "ffmpeg is missing features the server needs (listed above)." >&2
    exit 1
fi
exec scripts/live.sh
