#!/usr/bin/env bash
# Builds a release: the server, the Android app, and the web UI's download of the app.
#
#   ./release.sh              tests and builds everything into dist/<version>/, and publishes the APK
#                             to the web UI's download links (Server/web/downloads)
#   --no-web                  leave the web UI's download links as they are
#   --skip-tests              don't run the server tests first
#   --allow-dirty             build with uncommitted changes (the version then ends in "-dirty")
#   --publish                 then publish it on GitHub: tag the commit v<version>, push the tag to the
#                             "github" remote, and create the release with dist/<version>/'s files.
#                             Needs gh, signed in (gh auth login), and a clean tree. Checked before
#                             anything is built; a release that already exists is never replaced.
#
# dist/<version>/ then holds:
#   weirdstreamer-server-<version>-linux-<arch>.tar.gz  the server ready to run: bin/server and
#       bin/cli, the web UI (with this APK to download), setup.sh (writes config.yml, can create
#       the admin user and install a systemd service), config.example.yml, LICENSE, NOTICE and VERSION.
#       Where it runs it needs ffmpeg, nothing else.
#   WeirdStreamer-<version>.apk                         the Android app (TV and phone)
#   SHA256SUMS
#
# The version is the app's: 0.0.1.<commit count>, the same build number as the APK.
# It installs on no device: Android/deploy-release.sh does that.
set -euo pipefail
cd "$(dirname "$0")"
root=$PWD

web=1 tests=1 dirty_ok=0 publish=0
for arg in "$@"; do
    case "$arg" in
        --no-web) web=0 ;;
        --skip-tests) tests=0 ;;
        --allow-dirty) dirty_ok=1 ;;
        --publish) publish=1 ;;
        -h|--help) sed -n '2,24p' "$0"; exit 0 ;;
        *) echo "Unknown option: $arg (see --help)" >&2; exit 2 ;;
    esac
done

# What the build needs, checked before anything runs: every problem is listed at once.
problems=()
problem() { problems+=("$*"); }
newest() { ls -d "$@" 2>/dev/null | sort -V | tail -1 || true; } # the newest of the paths that exist

# Go, for the server. Often installed off PATH (the go.dev tarball goes to /usr/local/go).
if ! command -v go >/dev/null; then
    found=$(newest /usr/local/go/bin/go "$HOME"/go/bin/go "$HOME"/.local/go/bin/go "$HOME"/sdk/go*/bin/go)
    if [ -n "$found" ]; then
        echo "Using Go from $(dirname "$found") (not on PATH)"
        PATH="$(dirname "$found"):$PATH"
    fi
fi
if ! command -v go >/dev/null; then
    problem "Go isn't installed (or not on PATH): get 1.21 or later from https://go.dev/dl/. It fetches the version go.mod asks for itself."
elif [ "$(printf '%s\n' 1.21 "$(go env GOVERSION | sed 's/^go//')" | sort -V | head -1)" != 1.21 ]; then
    problem "Go $(go env GOVERSION) is too old: 1.21 or later can fetch the version go.mod asks for (https://go.dev/dl/)."
fi
# cgo, for SQLite
command -v gcc >/dev/null || problem "gcc is missing, which the server's SQLite needs (sudo apt install build-essential)."
# The server tests make test videos
if [ "$tests" = 1 ]; then
    command -v ffmpeg >/dev/null && command -v ffprobe >/dev/null ||
        problem "ffmpeg and ffprobe are missing, which the server tests use (sudo apt install ffmpeg), or pass --skip-tests."
fi

# A JDK for Gradle: the Android plugin needs 17 or later.
java="${JAVA_HOME:+$JAVA_HOME/bin/}java"
if ! command -v "$java" >/dev/null; then
    problem "No Java: install a JDK 17 or later (sudo apt install openjdk-21-jdk-headless), or set JAVA_HOME."
else
    jv=$("$java" -version 2>&1 | sed -n 's/.*version "\([0-9]*\).*/\1/p' | head -1)
    [ "${jv:-0}" -ge 17 ] || problem "Java $jv is too old for the Android build: install a JDK 17 or later, or point JAVA_HOME at one."
fi

# The Android SDK: local.properties' sdk.dir, ANDROID_HOME, or Android Studio's usual place.
sdk=$(sed -n 's/^sdk\.dir=//p' Android/local.properties 2>/dev/null || true)
sdk=${sdk:-${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}}
if [ -z "$sdk" ] && [ -d "$HOME/Android/Sdk" ]; then
    sdk=$HOME/Android/Sdk
    export ANDROID_HOME=$sdk
fi
if [ -z "$sdk" ] || [ ! -d "$sdk" ]; then
    problem "No Android SDK: install Android Studio (or the command-line tools), then set sdk.dir in Android/local.properties or ANDROID_HOME."
fi

# The key the app is signed with (Android/app/build.gradle.kts). An APK signed with any other key
# won't install over the app already on the devices.
key="$HOME/.config/.android/debug.keystore"
[ -f "$key" ] || problem "No signing key at $key: copy it from the machine that builds the app. (A new key would make an APK that won't update the installed app.)"

for tool in git python3 tar sha256sum; do
    command -v "$tool" >/dev/null || problem "$tool is missing."
done

# Publishing: gh, signed in, and the GitHub remote it goes to ("owner/name", from its URL).
if [ "$publish" = 1 ]; then
    [ "$dirty_ok" = 0 ] || problem "--publish and --allow-dirty don't go together: a release is a commit."
    if ! command -v gh >/dev/null; then
        problem "gh, GitHub's command-line tool, is missing (sudo apt install gh), which --publish uses."
    elif ! gh auth status >/dev/null 2>&1; then
        problem "gh isn't signed in to GitHub: run gh auth login."
    fi
    repo=$({ git config --get remote.github.url || true; } | sed -nE 's#^(git@github\.com:|https://github\.com/|ssh://git@github\.com/)([^/]+/[^/]+)$#\2#p' | sed 's/\.git$//')
    [ -n "$repo" ] || problem "No \"github\" remote pointing at github.com, which --publish pushes the tag to."
fi
if [ "${#problems[@]}" -gt 0 ]; then
    echo "Can't build a release on this machine yet:" >&2
    printf '  - %s\n' "${problems[@]}" >&2
    exit 1
fi

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
tag="v$version"

# Before building: the tag must be free (or already on this commit), and the release new.
if [ "$publish" = 1 ]; then
    head=$(git rev-parse HEAD)
    local_tag=$(git rev-parse -q --verify "refs/tags/$tag^{commit}" || true)
    remote_tag=$(git ls-remote github "refs/tags/$tag^{}" "refs/tags/$tag" | sort -r | head -1 | cut -f1)
    if [ -n "$local_tag" ] && [ "$local_tag" != "$head" ]; then
        echo "The tag $tag is on another commit ($(git rev-parse --short "$local_tag")), not $commit." >&2
        exit 1
    fi
    if [ -n "$remote_tag" ] && [ "$remote_tag" != "$head" ]; then
        echo "GitHub's tag $tag is on another commit, not $commit." >&2
        exit 1
    fi
    if gh release view "$tag" --repo "$repo" >/dev/null 2>&1; then
        echo "The release $tag is already on GitHub ($repo): nothing to publish." >&2
        exit 1
    fi
fi
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
mkdir -p "$pkg/bin"
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
cp Server/package/setup.sh Server/config.example.yml LICENSE NOTICE "$pkg/"
echo "$version ($commit)" >"$pkg/VERSION"
tarball="weirdstreamer-server-$version-linux-$arch.tar.gz"
tar -C "$stage" -czf "$out/$tarball" "weirdstreamer-$version"

(cd "$out" && sha256sum -- * >SHA256SUMS)
echo "== Done: $root/$out"
(cd "$out" && ls -l --block-size=K -- * | awk '{print "   " $5 "  " $NF}')

if [ "$publish" = 1 ]; then
    echo "== Publishing $tag on GitHub ($repo)"
    git rev-parse -q --verify "refs/tags/$tag" >/dev/null || git tag -a "$tag" -m "Weird Streamer $version"
    git push -q github "refs/tags/$tag"
    # Its notes list the commits since the last release; edit them on GitHub as you like.
    url=$(gh release create "$tag" "$out"/* --repo "$repo" --verify-tag \
        --title "Weird Streamer $version" --generate-notes)
    echo "   $url"
fi
