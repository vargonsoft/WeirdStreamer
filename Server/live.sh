#!/usr/bin/env bash
# Runs the live server (port 9000) in this terminal, logging to server.log. When restart-live.sh
# stops it, it starts again with the new binary; Ctrl-C (or a crash) ends it.
cd "$(dirname "$0")"
marker=run/.restart
rm -f "$marker"
while true; do
    ./server 2>&1 | tee -a server.log
    if [ -e "$marker" ]; then
        rm -f "$marker"
        echo "=== restarting with the new server ($(date '+%F %T')) ===" | tee -a server.log
        continue
    fi
    break
done
