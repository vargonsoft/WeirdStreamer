#!/usr/bin/env bash
# Builds a release: the server, the Android app, and the web UI's download of the app.
#
#   ./release.sh              tests and builds everything into dist/<version>/, and publishes the APK
#                             to the web UI's download links (Server/web/downloads)
#   --no-web                  leave the web UI's download links as they are
#   --skip-tests              don't run the server tests first
#   --platforms LIST          the server packages to build, comma-separated (default: all of them):
#                             linux-amd64, linux-arm64 (cross-built: needs aarch64-linux-gnu-gcc),
#                             darwin-arm64 (built on the Mac in iOS/remote/remote.local's MAC_HOST,
#                             over SSH: it needs Go and Xcode's command-line tools there)
#   --allow-dirty             build with uncommitted changes (the version then ends in "-dirty")
#   --publish                 then publish it on GitHub: tag the commit v<version>, push the tag to the
#                             "github" remote, and create the release with dist/<version>/'s files.
#                             Needs gh, signed in (gh auth login), and a clean tree. Checked before
#                             anything is built; a release that already exists is never replaced.
#   --notes TEXT              with --publish: notes written above the generated list (GitHub's notes
#                             of the commits since the last release)
#   --notes-file FILE         with --publish: the same, read from FILE (Markdown)
#
# dist/<version>/ then holds:
#   weirdstreamer-server-<version>-<os>-<arch>.tar.gz   the server ready to run, one per platform:
#       bin/server and bin/cli, the web UI (with this APK to download), setup.sh (writes config.yml,
#       can create the admin user and install a service: systemd, or launchd on macOS),
#       check-ffmpeg.sh, config.example.yml, LICENSE, NOTICE and VERSION; macOS's also
#       build-ffmpeg.sh, which setup offers to run (Homebrew's ffmpeg lacks the DVD reader,
#       chromaprint and libass).
#       Where it runs it needs ffmpeg, and for HEIC photos heif-convert (libheif's tools). For CJK
#       subtitle fonts it's optional: fontTools' pyftsubset and Noto Sans CJK (see the README).
#   WeirdStreamer-<version>.apk                         the Android app (TV and phone)
#   SHA256SUMS
#
# The version is the app's: 0.0.1.<commit count>, the same build number as the APK.
# It installs on no device: Android/deploy-release.sh does that.
set -euo pipefail
cd "$(dirname "$0")"
root=$PWD

web=1 tests=1 dirty_ok=0 publish=0 notes= notes_file= platforms=linux-amd64,linux-arm64,darwin-arm64
while [ $# -gt 0 ]; do
    case "$1" in
        --no-web) web=0 ;;
        --skip-tests) tests=0 ;;
        --allow-dirty) dirty_ok=1 ;;
        --publish) publish=1 ;;
        --notes|--notes-file)
            [ $# -ge 2 ] || { echo "$1 needs a value (see --help)" >&2; exit 2; }
            if [ "$1" = --notes ]; then notes=$2; else notes_file=$2; fi
            shift ;;
        --notes=*) notes=${1#--notes=} ;;
        --platforms)
            [ $# -ge 2 ] || { echo "$1 needs a value (see --help)" >&2; exit 2; }
            platforms=$2
            shift ;;
        --platforms=*) platforms=${1#--platforms=} ;;
        --notes-file=*) notes_file=${1#--notes-file=} ;;
        -h|--help) sed -n '2,36p' "$0"; exit 0 ;;
        *) echo "Unknown option: $1 (see --help)" >&2; exit 2 ;;
    esac
    shift
done
if { [ -n "$notes" ] || [ -n "$notes_file" ]; } && [ "$publish" = 0 ]; then
    echo "--notes and --notes-file are for the GitHub release: they need --publish" >&2; exit 2
fi
if [ -n "$notes_file" ] && [ ! -f "$notes_file" ]; then
    echo "--notes-file: $notes_file isn't a file" >&2; exit 2
fi

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
# cgo, for SQLite: a C compiler for each platform built here, and the Mac for macOS
IFS=, read -r -a targets <<<"$platforms"
mac_host=
for target in "${targets[@]}"; do
    case $target in
        linux-amd64)
            command -v gcc >/dev/null || problem "gcc is missing, which the server's SQLite needs (sudo apt install build-essential)."
            [ "$(uname -m)" = x86_64 ] || command -v x86_64-linux-gnu-gcc >/dev/null ||
                problem "linux-amd64 needs x86_64-linux-gnu-gcc on this $(uname -m) machine (sudo apt install gcc-x86-64-linux-gnu)."
            ;;
        linux-arm64)
            [ "$(uname -m)" = aarch64 ] || command -v aarch64-linux-gnu-gcc >/dev/null ||
                problem "linux-arm64 needs aarch64-linux-gnu-gcc, for SQLite (sudo apt install gcc-aarch64-linux-gnu), or leave it out with --platforms."
            ;;
        darwin-arm64)
            [ -f iOS/remote/remote.local ] && mac_host=$(sed -n 's/^MAC_HOST=//p' iOS/remote/remote.local | head -1)
            mac_host=${WS_MAC_HOST:-$mac_host}
            if [ -z "$mac_host" ]; then
                problem "darwin-arm64 is built on a Mac: set MAC_HOST in iOS/remote/remote.local (see iOS/README.md), or leave it out with --platforms."
            elif ! ssh -o BatchMode=yes -o ConnectTimeout=5 "$mac_host" 'PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:$PATH; command -v go >/dev/null && command -v clang >/dev/null' 2>/dev/null; then
                problem "Can't build darwin-arm64 on $mac_host: it isn't reachable by SSH without a password, or lacks Go or Xcode's command-line tools."
            fi
            ;;
        *) problem "Unknown platform $target: linux-amd64, linux-arm64 or darwin-arm64." ;;
    esac
done
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

# The Android build compiles the ASS subtitle renderer (Android/native/build-libass.sh): the NDK
# and the tools it uses.
ndk_dir="${ANDROID_NDK_HOME:-$HOME/Android/Sdk/ndk/29.0.14206865}"
[ -d "$ndk_dir" ] || problem "The Android NDK r29 isn't at $ndk_dir (set ANDROID_NDK_HOME): the ASS renderer is built with it."
for tool in cmake make pkg-config curl; do
    command -v "$tool" >/dev/null || problem "$tool is missing (the ASS renderer's build needs it)."
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

stage=$(mktemp -d)
trap 'rm -rf "$stage"; rm -f "${notes_tmp:-}"' EXIT
ldflags="-s -w -X main.version=$version"
# build_linux ARCH DIR: bin/server and bin/cli for Linux on ARCH, cross-compiled where it isn't
# this machine's (cgo needs that architecture's C compiler).
build_linux() {
    local arch=$1 dir=$2 cc=gcc
    case "$arch:$(uname -m)" in
        amd64:x86_64|arm64:aarch64) ;;
        amd64:*) cc=x86_64-linux-gnu-gcc ;;
        arm64:*) cc=aarch64-linux-gnu-gcc ;;
    esac
    mkdir -p "$dir"
    for cmd in server cli; do
        # (go-sqlite3's C has a harmless const warning)
        CGO_ENABLED=1 GOOS=linux GOARCH="$arch" CC="$cc" CGO_CFLAGS="-O2 -g -Wno-discarded-qualifiers" \
            go -C Server/src build -trimpath -ldflags "$ldflags" -o "$dir/$cmd" "./cmd/$cmd"
    done
}
# build_darwin DIR: bin/server and bin/cli for Apple Silicon Macs, built on the Mac (cgo needs
# Apple's SDK): Server/src as it is here (tracked and new files, not ignored ones) goes over SSH
# into a temporary folder there, and the binaries come back.
build_darwin() {
    local dir=$1
    mkdir -p "$dir"
    git ls-files -z -co --exclude-standard Server/src | tar -czf - --null -T - |
        ssh -o BatchMode=yes "$mac_host" "PATH=/opt/homebrew/bin:/usr/local/go/bin:/usr/local/bin:\$PATH
            set -e; tmp=\$(mktemp -d); trap 'rm -rf \"\$tmp\"' EXIT
            tar -xzf - -C \"\$tmp\"; cd \"\$tmp/Server/src\"
            for cmd in server cli; do
                CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '$ldflags' -o \"\$tmp/out/\$cmd\" ./cmd/\$cmd >&2
            done
            COPYFILE_DISABLE=1 tar --no-xattrs -czf - -C \"\$tmp/out\" server cli" | tar -xzf - -C "$dir"
}
for target in "${targets[@]}"; do
    echo "== Server for $target"
    case $target in
        linux-*) build_linux "${target#linux-}" "$stage/bin-$target" ;;
        darwin-arm64) build_darwin "$stage/bin-$target" ;;
    esac
    file "$stage/bin-$target/server" 2>/dev/null | sed 's/^[^:]*: /   /' || true
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

echo "== Server packages"
pkg="$stage/weirdstreamer-$version"
mkdir -p "$pkg"
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
cp Server/package/setup.sh Server/scripts/check-ffmpeg.sh Server/config.example.yml LICENSE NOTICE "$pkg/"
echo "$version ($commit)" >"$pkg/VERSION"
# Each platform's package: the shared files and its own binaries (macOS's also build-ffmpeg.sh)
for target in "${targets[@]}"; do
    rm -rf "$pkg/bin" "$pkg/build-ffmpeg.sh"
    cp -R "$stage/bin-$target" "$pkg/bin"
    [ "$target" = darwin-arm64 ] && cp Server/scripts/build-ffmpeg.sh "$pkg/"
    tar -C "$stage" -czf "$out/weirdstreamer-server-$version-$target.tar.gz" "weirdstreamer-$version"
done

(cd "$out" && sha256sum -- * >SHA256SUMS)
echo "== Done: $root/$out"
(cd "$out" && ls -l --block-size=K -- * | awk '{print "   " $5 "  " $NF}')

if [ "$publish" = 1 ]; then
    echo "== Publishing $tag on GitHub ($repo)"
    git rev-parse -q --verify "refs/tags/$tag" >/dev/null || git tag -a "$tag" -m "Weird Streamer $version"
    git push -q github "refs/tags/$tag"
    # Its notes: --notes and --notes-file (if given) first, then the commits since the last release
    # (generated by GitHub). Edit them on GitHub as you like.
    notes_args=(--generate-notes)
    if [ -n "$notes" ] || [ -n "$notes_file" ]; then
        notes_tmp=$(mktemp)
        { [ -z "$notes" ] || printf '%s\n\n' "$notes"; [ -z "$notes_file" ] || cat "$notes_file"; } >"$notes_tmp"
        notes_args=(--notes-file "$notes_tmp" --generate-notes)
    fi
    url=$(gh release create "$tag" "$out"/* --repo "$repo" --verify-tag \
        --title "Weird Streamer $version" "${notes_args[@]}")
    echo "   $url"
fi
