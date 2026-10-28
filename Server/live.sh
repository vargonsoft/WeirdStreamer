#!/usr/bin/env bash
# Runs the live server (port 9000) in this terminal. Each start logs to its own file in logs/
# (server-<start time>.log); server.log links to the newest, and only the last 20 are kept.
# When restart-live.sh stops it, it starts again with the new binary; Ctrl-C (or a crash) ends it.
cd "$(dirname "$0")"
marker=run/.restart
rm -f "$marker"
mkdir -p logs
# A server.log from before logs/ is kept there with the rest.
[ -f server.log ] && [ ! -L server.log ] && mv server.log "logs/server-$(date -r server.log '+%F_%H%M%S').log"
while true; do
    log="logs/server-$(date '+%F_%H%M%S').log"
    ln -sfn "$log" server.log
    ls -1t logs/server-*.log 2>/dev/null | tail -n +20 | xargs -r rm -f
    ./server 2>&1 | tee -a "$log"
    if [ -e "$marker" ]; then
        rm -f "$marker"
        echo "=== restarting with the new server ($(date '+%F %T')) ===" | tee -a "$log"
        continue
    fi
    break
done
