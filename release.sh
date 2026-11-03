#!/usr/bin/env bash
# Builds a release: the server, the Android app, and the web UI's download of the app.
#
#   ./release.sh              tests and builds everything into dist/<version>/, and publishes the APK
#                             to the web UI's download links (Server/web/downloads)
#   --no-web                  leave the web UI's download links as they are
#   --skip-tests              don't run the server tests first
#   --allow-dirty             build with uncommitted changes (the version then ends in "-dirty")
#
# dist/<version>/ then holds:
#   weirdstreamer-server-<version>-linux-<arch>.tar.gz  the server ready to run: bin/server and
#       bin/cli, the web UI (with this APK to download), config.example.yml and the scripts that set
#       it up and run it (./start-server.sh). It needs ffmpeg and PyYAML where it runs, not Go.
#   WeirdStreamer-<version>.apk                         the Android app (TV and phone)
#   SHA256SUMS
#
# The version is the app's: 0.0.1.<commit count>, the same build number as the APK.
# It installs on no device: Android/deploy-release.sh does that.
set -euo pipefail
cd "$(dirname "$0")"
root=$PWD

web=1 tests=1 dirty_ok=0
for arg in "$@"; do
    case "$arg" in
        --no-web) web=0 ;;
        --skip-tests) tests=0 ;;
        --allow-dirty) dirty_ok=1 ;;
        -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
        *) echo "Unknown option: $arg (see --help)" >&2; exit 2 ;;
    esac
done

# A release is a commit: what's built is what's in git, unless asked otherwise.
dirty=""
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
    if [ "$dirty_ok" = 0 ]; then
        echo "There are uncommitted changes: commit them, or pass --allow-dirty." >&2
        git status --short --untracked-files=no >&2
        exit 1
    fi
    dirty="-dirty"
fi
# The app's version (Android/app/build.gradle.kts): its hand-set part and the commit count.
base=$(sed -n 's/.*versionName = "\([0-9.]*\)\.\$buildNumber".*/\1/p' Android/app/build.gradle.kts)
[ -n "$base" ] || { echo "Can't read versionName from Android/app/build.gradle.kts" >&2; exit 1; }
version="$base.$(git rev-list --count HEAD)$dirty"
commit=$(git rev-parse --short HEAD)
arch=$(go env GOARCH)
out="dist/$version"
echo "== Release $version ($commit)"
rm -rf "$out"
mkdir -p "$out"

if [ "$tests" = 1 ]; then
    echo "== Server tests"
    # Not ./...: stray main files in Server/src break it
    (cd Server/src && go vet ./internal/... ./cmd/... && go test ./internal/... >/dev/null) ||
        { echo "Server tests failed (rerun them in Server/src to see why)." >&2; exit 1; }
fi

echo "== Server"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
pkg="$stage/weirdstreamer-$version"
mkdir -p "$pkg/bin" "$pkg/scripts"
for cmd in server cli; do
    # (go-sqlite3's C has a harmless const warning)
    CGO_ENABLED=1 CGO_CFLAGS="-O2 -g -Wno-discarded-qualifiers" go -C Server/src build -trimpath -ldflags "-s -w" -o "$pkg/bin/$cmd" "./cmd/$cmd"
done

echo "== Android"
(cd Android && ./gradlew :app:assembleRelease -q)
apk=Android/app/build/outputs/apk/release/app-release.apk
built=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["elements"][0]["versionName"])' \
    "$(dirname "$apk")/output-metadata.json")
if [ "$built" != "${version%-dirty}" ]; then
    echo "The APK says $built, not ${version%-dirty}." >&2
    exit 1
fi
cp -p "$apk" "$out/WeirdStreamer-$version.apk"

if [ "$web" = 1 ]; then
    echo "== Web download"
    Android/deploy-release.sh --web --no-build | sed 's/^/   /'
fi

echo "== Server package"
cp -R Server/web "$pkg/web"
rm -rf "$pkg/web/downloads"
# The package's own web UI offers this release's app
mkdir -p "$pkg/web/downloads"
cp -p "$apk" "$pkg/web/downloads/WeirdStreamer.apk"
python3 - "$(dirname "$apk")/output-metadata.json" "$pkg/web/downloads/WeirdStreamer.apk" \
    >"$pkg/web/downloads/WeirdStreamer.json" <<'PY'
import datetime, json, os, sys
element = json.load(open(sys.argv[1]))["elements"][0]
built = datetime.datetime.fromtimestamp(os.path.getmtime(sys.argv[2]), datetime.timezone.utc)
print(json.dumps({"version": element["versionName"], "build": element["versionCode"],
                  "size": os.path.getsize(sys.argv[2]), "built": built.isoformat(timespec="seconds")}))
PY
cp Server/scripts/live.sh Server/scripts/create-config.sh Server/scripts/check-ffmpeg.sh "$pkg/scripts/"
cp Server/start-server.sh Server/config.example.yml Server/README.md LICENSE "$pkg/"
echo "$version ($commit)" >"$pkg/VERSION"
tarball="weirdstreamer-server-$version-linux-$arch.tar.gz"
tar -C "$stage" -czf "$out/$tarball" "weirdstreamer-$version"

(cd "$out" && sha256sum -- * >SHA256SUMS)
echo "== Done: $root/$out"
(cd "$out" && ls -l --block-size=K -- * | awk '{print "   " $5 "  " $NF}')
