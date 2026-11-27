#!/usr/bin/env bash
# Builds a release: the server, the Android and iPhone apps, and the web UI's download of the app.
#
#   ./release.sh              tests and builds everything into dist/<version>/, and publishes the APK
#                             to the web UI's download links (Server/web/downloads)
#   --no-web                  leave the web UI's download links as they are
#   --skip-tests              don't run the server tests first
#   --no-ipa                  don't build the iPhone app
#   --platforms LIST          the server packages to build, comma-separated (default: all of them):
#                             linux-amd64, linux-arm64 (cross-built: needs aarch64-linux-gnu-gcc),
#                             darwin-arm64 (built on the Mac in iOS/remote/remote.local's MAC_HOST,
#                             over SSH: it needs Go and Xcode's command-line tools there),
#                             windows-amd64 (built on the Windows machine Server/windows.local describes,
#                             over SSH: VM_HOST, VM_USER, VM_KEY, VM_KNOWN_HOSTS; it needs Go, Git and
#                             MinGW-w64 gcc there; with VM_START and VM_STOP commands there, a
#                             machine that doesn't answer is started, and stopped after the build)
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
#   weirdstreamer-server-<version>-windows-amd64.zip    Windows' package instead: bin\server.exe and
#       bin\cli.exe, setup.cmd and setup.ps1 (instead of setup.sh and check-ffmpeg.sh), and the same
#       other files.
#   WeirdStreamer-<version>.apk                         the Android app (TV and phone)
#   WeirdStreamer-<version>.ipa                         the iPhone app, unsigned, for AltStore and
#       SideStore (they sign it), built on the Mac (iOS/remote/mac-ios) from the release commit,
#       which must be on origin
#   altstore-source.json                                the AltStore source listing that IPA
#       (iOS/altstore/make-source.py): users add
#       https://github.com/<owner>/<repo>/releases/latest/download/altstore-source.json once, and
#       each release is an update
#   SHA256SUMS
#
# The version is the app's: 0.0.1.<commit count>, the same build number as the APK.
# It installs on no device: Android/deploy-release.sh does that.
# What's built elsewhere (the Windows server; the Mac's server and IPA) runs in the background while
# this machine runs the tests and builds the Linux servers and the APK; each such build's output is
# shown when it's done.
set -euo pipefail
cd "$(dirname "$0")"
root=$PWD

web=1 tests=1 ipa=1 dirty_ok=0 publish=0 notes= notes_file= platforms=linux-amd64,linux-arm64,darwin-arm64,windows-amd64
while [ $# -gt 0 ]; do
    case "$1" in
        --no-web) web=0 ;;
        --skip-tests) tests=0 ;;
        --no-ipa) ipa=0 ;;
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
        -h|--help) sed -n '2,/^# It installs/p' "$0"; exit 0 ;;
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

# The Windows build machine (Server/windows.local, or WS_WINDOWS_LOCAL), reached by SSH with its key
win_conf=${WS_WINDOWS_LOCAL:-Server/windows.local}
win_value() { if [ -f "$win_conf" ]; then sed -n "s/^$1=//p" "$win_conf" | head -1; fi; }
win_ssh() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o IdentitiesOnly=yes -i "$win_key" ${win_known:+-o UserKnownHostsFile="$win_known"} "$win_user@$win_host" "$@"; }
win_scp() { scp -q -o BatchMode=yes -o IdentitiesOnly=yes -i "$win_key" ${win_known:+-o UserKnownHostsFile="$win_known"} "$@"; }
# A machine that doesn't answer is started with its VM_START command (one that returns once SSH
# answers, such as a VM's start script), and stopped with VM_STOP once the Windows build is done,
# or when this script ends early: only a machine this script started.
win_started=0
stop_windows() {
    if [ "$win_started" = 1 ]; then
        win_started=0
        echo "== Stopping the Windows build machine"
        if ! bash -c "$(win_value VM_STOP)" 2>&1 | sed 's/^/   /' >&2; then
            echo "   It didn't stop: stop it yourself ($(win_value VM_STOP))." >&2
        fi
    fi
}

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
        windows-amd64)
            win_host=$(win_value VM_HOST) win_user=$(win_value VM_USER) win_key=$(win_value VM_KEY) win_known=$(win_value VM_KNOWN_HOSTS)
            if [ -z "$win_host" ] || [ -z "$win_user" ] || [ -z "$win_key" ]; then
                problem "windows-amd64 is built on a Windows machine over SSH: describe it in $win_conf (VM_HOST, VM_USER, VM_KEY, VM_KNOWN_HOSTS), or leave it out with --platforms."
            else
                if ! win_ssh 'go version; gcc --version' >/dev/null 2>&1 && [ -n "$(win_value VM_START)" ] && [ -n "$(win_value VM_STOP)" ]; then
                    echo "== Starting the Windows build machine ($win_host)"
                    win_started=1
                    trap stop_windows EXIT
                    bash -c "$(win_value VM_START)" 2>&1 | sed 's/^/   /' >&2 || true
                fi
                if ! win_ssh 'go version; gcc --version' >/dev/null 2>&1; then
                    problem "Can't build windows-amd64 on $win_host: it isn't reachable by SSH with its key (is it running? VM_START and VM_STOP in $win_conf let this script start it), or lacks Go or MinGW-w64 gcc."
                fi
            fi
            ;;
        *) problem "Unknown platform $target: linux-amd64, linux-arm64, darwin-arm64 or windows-amd64." ;;
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

# The iPhone app: built on the Mac by iOS/remote/mac-ios, from a commit on origin (its build number
# is that commit's count), with the same version as Android's.
if [ "$ipa" = 1 ]; then
    ios_host=$(sed -n 's/^MAC_HOST=//p' iOS/remote/remote.local 2>/dev/null | head -1)
    if [ -z "$ios_host" ] || ! grep -q '^MAC_SCRIPT=' iOS/remote/remote.local 2>/dev/null; then
        problem "The iPhone app is built on a Mac: set MAC_HOST and MAC_SCRIPT in iOS/remote/remote.local (see iOS/README.md), or pass --no-ipa."
    elif ! ssh -o BatchMode=yes -o ConnectTimeout=5 "$ios_host" true 2>/dev/null; then
        problem "Can't reach $ios_host by SSH without a password, which builds the iPhone app (or pass --no-ipa)."
    fi
    ios_version=$(sed -n 's/.*MARKETING_VERSION = \(.*\);/\1/p' iOS/WeirdStreamer/WeirdStreamer.xcodeproj/project.pbxproj | sort -u)
    android_version=$(sed -n 's/.*versionName = "\([0-9.]*\)\.\$buildNumber".*/\1/p' Android/app/build.gradle.kts)
    [ "$ios_version" = "$android_version" ] ||
        problem "The iPhone app's version ($ios_version, MARKETING_VERSION in the Xcode project) isn't Android's ($android_version)."
    if [ "$dirty_ok" = 0 ]; then
        git fetch -q origin
        [ -n "$(git branch -r --contains HEAD 2>/dev/null | grep '^ *origin/')" ] ||
            problem "The release commit isn't on origin yet: push it (the Mac builds the iPhone app from origin), or pass --no-ipa."
    fi
fi

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

stage=$(mktemp -d)
# Ending early also stops the builds still running elsewhere (see "At once" below).
stop_lanes() { local p; for p in $(jobs -p); do pkill -P "$p" 2>/dev/null; kill "$p" 2>/dev/null; done; wait 2>/dev/null; }
trap 'stop_lanes; rm -rf "$stage"; rm -f "${notes_tmp:-}"; stop_windows' EXIT
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
# build_windows DIR: bin\server.exe and bin\cli.exe for Windows on x64, built on the Windows machine
# (cgo needs MinGW-w64): Server/src as it is here goes over SSH into a folder in that account's home,
# and the binaries come back.
build_windows() {
    local dir=$1 work="ws-release-$$" built=0
    mkdir -p "$dir"
    git ls-files -z -co --exclude-standard Server/src | tar -czf "$stage/windows-src.tgz" --null -T -
    win_scp "$stage/windows-src.tgz" "$win_user@$win_host:$work.tgz"
    # PowerShell there: the home folder holds the work folder, removed after
    if win_ssh "\$w = Join-Path \$HOME '$work'; New-Item -ItemType Directory (Join-Path \$w 'out') | Out-Null; tar -xzf (Join-Path \$HOME '$work.tgz') -C \$w; Remove-Item (Join-Path \$HOME '$work.tgz'); Set-Location (Join-Path \$w 'Server\\src'); \$env:CGO_ENABLED = '1'; \$env:GOOS = 'windows'; \$env:GOARCH = 'amd64'; foreach (\$c in 'server', 'cli') { go build -trimpath -ldflags '$ldflags' -o (Join-Path \$w \"out\\\$c.exe\") \"./cmd/\$c\" 2>&1 | % { \"\$_\" }; if (\$LASTEXITCODE -ne 0) { exit 1 } }" >&2; then
        built=1
    fi
    if [ "$built" = 1 ]; then
        win_scp "$win_user@$win_host:$work/out/server.exe" "$win_user@$win_host:$work/out/cli.exe" "$dir/" || true
    fi
    win_ssh "Remove-Item -Recurse -Force (Join-Path \$HOME '$work')" >/dev/null 2>&1 || true
    [ -f "$dir/server.exe" ] && [ -f "$dir/cli.exe" ] || { echo "The Windows build failed." >&2; exit 1; }
}
# build_ipa FILE: the iPhone app, built on the Mac from the release commit (or, dirty, with the
# uncommitted changes).
build_ipa() {
    if [ -n "$dirty" ]; then
        iOS/remote/mac-ios ipa -o "$1" 2>&1 | sed 's/^/   /'
    else
        iOS/remote/mac-ios ipa --ref "$(git rev-parse HEAD)" -o "$1" 2>&1 | sed 's/^/   /'
    fi
}

# At once: what's built on other machines (the Windows one; the Mac's server, then the IPA) runs in
# the background while this machine tests and builds its own (the tests first, so nothing here
# slows them). Each background lane writes its own log, shown when it's done; one that fails stops
# the release.
lane_names=() lane_pids=()
lane() { # lane NAME COMMAND...: runs COMMAND in the background, its output in $stage/NAME.log
    local name=$1; shift
    ( start=$SECONDS; "$@"; status=$?; echo "   ($((SECONDS - start)) s)"; exit $status ) >"$stage/lane-$name.log" 2>&1 &
    lane_names+=("$name") lane_pids+=($!)
    echo "== Started: $name (in the background)"
}
mac_lane() {
    if [ -n "$mac_host" ]; then echo "== Server for darwin-arm64"; build_darwin "$stage/bin-darwin-arm64"; fi
    if [ "$ipa" = 1 ]; then echo "== iPhone"; build_ipa "$out/WeirdStreamer-$version.ipa"; fi
}
for target in "${targets[@]}"; do
    [ "$target" = windows-amd64 ] && lane "Windows server" build_windows "$stage/bin-$target"
done
if [ -n "$mac_host" ] || [ "$ipa" = 1 ]; then
    lane "Mac (macOS server, iPhone app)" mac_lane
fi
# wait_lanes: waits for each lane, showing its log; the first that failed ends the release.
wait_lanes() {
    local i
    for i in "${!lane_pids[@]}"; do
        if wait "${lane_pids[$i]}"; then
            echo "== Done: ${lane_names[$i]}"
            cat "$stage/lane-${lane_names[$i]}.log"
        else
            echo "== Failed: ${lane_names[$i]}" >&2
            cat "$stage/lane-${lane_names[$i]}.log" >&2
            exit 1
        fi
        if [ "${lane_names[$i]}" = "Windows server" ]; then stop_windows; fi
    done
}

if [ "$tests" = 1 ]; then
    echo "== Server tests"
    # Not ./...: stray main files in Server/src break it
    (cd Server/src && go vet ./internal/... ./cmd/... && go test ./internal/... >/dev/null) ||
        { echo "Server tests failed (rerun them in Server/src to see why)." >&2; exit 1; }
fi

for target in "${targets[@]}"; do
    case $target in
        linux-*) echo "== Server for $target"; build_linux "${target#linux-}" "$stage/bin-$target" ;;
    esac
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

wait_lanes
for target in "${targets[@]}"; do
    echo "== Server for $target:"
    file "$stage/bin-$target"/server* 2>/dev/null | sed 's/^[^:]*: /   /' || true
done

if [ "$ipa" = 1 ]; then
    ipa_file="$out/WeirdStreamer-$version.ipa"
    [ -f "$ipa_file" ] || { echo "The iPhone app didn't build (iOS/remote/mac-ios log says why)." >&2; exit 1; }
    built=$(python3 -c 'import plistlib,sys,zipfile
z = zipfile.ZipFile(sys.argv[1])
i = plistlib.loads(z.read(next(n for n in z.namelist() if n.count("/") == 2 and n.endswith(".app/Info.plist"))))
print(i["CFBundleShortVersionString"] + "." + i["CFBundleVersion"])' "$ipa_file")
    if [ -z "$dirty" ] && [ "$built" != "$version" ]; then
        echo "The IPA says $built, not $version." >&2
        exit 1
    fi
    # The AltStore source, pointing at this release's IPA on GitHub
    repo=${repo:-$({ git config --get remote.github.url || true; } | sed -nE 's#^(git@github\.com:|https://github\.com/|ssh://git@github\.com/)([^/]+/[^/]+)$#\2#p' | sed 's/\.git$//')}
    if [ -n "$repo" ]; then
        python3 iOS/altstore/make-source.py "$ipa_file" --repo "$repo" --tag "$tag" --notes "$notes" >"$out/altstore-source.json"
    else
        echo "   No \"github\" remote: no AltStore source, which points at the release on GitHub."
    fi
fi

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
cp Server/config.example.yml LICENSE NOTICE Server/package/guide-extras.tsv "$pkg/"
echo "$version ($commit)" >"$pkg/VERSION"
# Each platform's package: the shared files and its own binaries (macOS's also build-ffmpeg.sh)
for target in "${targets[@]}"; do
    rm -rf "$pkg/bin" "$pkg/build-ffmpeg.sh" "$pkg/setup.sh" "$pkg/check-ffmpeg.sh" "$pkg/setup.ps1" "$pkg/setup.cmd"
    cp -R "$stage/bin-$target" "$pkg/bin"
    [ "$target" = darwin-arm64 ] && cp Server/scripts/build-ffmpeg.sh "$pkg/"
    if [ "$target" = windows-amd64 ]; then
        cp Server/package/setup.ps1 Server/package/setup.cmd "$pkg/"
        # $out is relative to the repository and $root is absolute, so the zip goes to "$root/$out"
        (cd "$stage" && python3 -m zipfile -c "$root/$out/weirdstreamer-server-$version-$target.zip" "weirdstreamer-$version")
    else
        cp Server/package/setup.sh Server/scripts/check-ffmpeg.sh "$pkg/"
        tar -C "$stage" -czf "$out/weirdstreamer-server-$version-$target.tar.gz" "weirdstreamer-$version"
    fi
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
