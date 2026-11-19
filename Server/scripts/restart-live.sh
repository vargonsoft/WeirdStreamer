#!/usr/bin/env bash
# Builds the server and restarts the live one (run by live.sh) with it, unless someone is
# watching: then it lists the streams and stops, exit code 2 (--force restarts anyway).
# Options: --force, --no-build (restart with the server binary as it is).
set -euo pipefail
cd "$(dirname "$0")/.."
force=0 build=1
for arg in "$@"; do
    case "$arg" in
        --force) force=1 ;;
        --no-build) build=0 ;;
        *) echo "usage: $0 [--force] [--no-build]" >&2; exit 1 ;;
    esac
done

# The live server: ./bin/server running in this directory (not the e2e one). Its binary reads as
# "(deleted)" once a new one has replaced it.
pid=""
for p in $(pgrep -u "$USER" -x server || true); do
    exe=$(readlink "/proc/$p/exe")
    if [ "$(readlink "/proc/$p/cwd")" = "$PWD" ] && [ "${exe% (deleted)}" = "$PWD/bin/server" ]; then
        pid=$p
    fi
done
[ -n "$pid" ] || { echo "The live server isn't running. Start it with Server/scripts/live.sh." >&2; exit 1; }
# live.sh restarts it; a server started some other way would just stop.
started_by_live() {
    local p=$1
    while [ "$p" -gt 1 ]; do
        tr '\0' '\n' <"/proc/$p/cmdline" | grep -qE '(^|/)live\.sh$' && return 0
        p=$(ps -o ppid= -p "$p" | tr -d ' ')
    done
    return 1
}
if ! started_by_live "$pid"; then
    echo "The live server (PID $pid) wasn't started by live.sh, so it wouldn't come back." >&2
    echo "Stop it, then run Server/scripts/live.sh in its terminal." >&2
    exit 1
fi

# The server writes a new key beside config.yml at each start, readable by its account only.
streams=$(curl -sk -H "X-Local-Key: $(cat local.key 2>/dev/null)" https://127.0.0.1:9000/local/streams)
active=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["active"])' <<<"$streams" 2>/dev/null) ||
    { echo "Can't read the live server's streams (/local/streams, with local.key)." >&2; exit 1; }
if [ "$active" -gt 0 ] && [ "$force" = 0 ]; then
    echo "Not restarting: $active active stream(s):"
    python3 -c '
import json, sys
for s in json.load(sys.stdin)["streams"]:
    print("  %s on %s: %s (%s), %ss since activity" % (s["user"], s["client"], s["state"], s["mode"], s["seconds_since_activity"]))
' <<<"$streams"
    exit 2
fi

if [ "$build" = 1 ]; then
    mkdir -p bin
    go -C src build -ldflags "-X main.version=$(scripts/version.sh)" -o ../bin/server.new ./cmd/server
    # The CLI too: it reads the same config and databases
    go -C src build -ldflags "-X main.version=$(scripts/version.sh)" -o ../bin/cli.new ./cmd/cli
    mv bin/server.new bin/server
    mv bin/cli.new bin/cli
fi

touch .restart
kill "$pid"
for _ in $(seq 1 50); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.2
done
for _ in $(seq 1 60); do
    if curl -sk -o /dev/null https://127.0.0.1:9000/local/streams; then
        echo "Live server restarted (was PID $pid)."
        exit 0
    fi
    sleep 0.5
done
echo "The live server hasn't come back after 30s: see the newest log file (server.logsPath in config.yml)." >&2
exit 1
