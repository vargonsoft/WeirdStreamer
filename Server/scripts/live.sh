#!/usr/bin/env bash
# Runs the live server (port 9000) in this terminal. Each start logs to its own file in the
# server.logsPath folder of config.yml
# (server-<start time>.log), and only the last 20 are kept. The server itself keeps auth.log and
# access.log there (internal/logs).
# When restart-live.sh stops it, it starts again with the new binary (bin/server); Ctrl-C (or a crash) ends it.
cd "$(dirname "$0")/.."
umask 077 # the logs are for the server's account only
# The server's own ffmpeg (scripts/build-ffmpeg.sh), when built, comes before the system's.
[ -x ffmpeg/bin/ffmpeg ] && export PATH="$(pwd)/ffmpeg/bin:$PATH"
marker=.restart
rm -f "$marker"
logs=$(python3 -c 'import yaml; print((yaml.safe_load(open("config.yml"))["server"] or {}).get("logsPath") or "")') ||
    { echo "Can't read server.logsPath from config.yml (python3 needs PyYAML: python3-yaml)." >&2; exit 1; }
[ -n "$logs" ] || { echo "Set server.logsPath in config.yml first." >&2; exit 1; }
mkdir -p "$logs"
while true; do
    log="$logs/server-$(date '+%F_%H%M%S').log"
    ls -1t "$logs"/server-*.log 2>/dev/null | tail -n +20 | xargs -r rm -f
    ./bin/server 2>&1 | tee -a "$log"
    if [ -e "$marker" ]; then
        rm -f "$marker"
        echo "=== restarting with the new server ($(date '+%F %T')) ===" | tee -a "$log"
        continue
    fi
    break
done
