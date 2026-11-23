#!/usr/bin/env bash
# Builds and tests the iPhone app on this Mac for another machine (run over SSH by mac-ios, beside
# this script), or here. It works in its own clone of origin, never in a checkout someone edits:
# each run fetches, checks out the commit asked for, applies the patch sent (changes not pushed
# or not committed yet), then builds.
#
#   ios-remote.sh build  [--ref REF] [--patch FILE|-] [--verbose]   the app for the simulator
#   ios-remote.sh test   [--ref REF] [--patch FILE|-] [--verbose]   the unit tests, on a simulator of its own
#   ios-remote.sh ipa    [--ref REF] [--patch FILE|-] [--verbose] [--out FILE|-]
#                        an unsigned IPA for AltStore (it signs it when installing); - writes it to stdout
#   ios-remote.sh log      the last run's full xcodebuild log
#   ios-remote.sh status   what the clone has checked out, and the simulator it tests on
#
# REF is a commit or branch on origin (default origin/main). One run at a time: others wait.
# Work folder: $IOS_REMOTE_DIR (default ~/Library/Caches/WeirdStreamer-remote): the clone, its
# DerivedData, logs and IPAs. The first run also builds the subtitle renderer's libraries there
# (iOS/native/build-libass.sh: needs cmake and pkg-config).
set -euo pipefail

usage() { sed -n '2,19p' "$0" | sed 's/^# \{0,1\}//'; }

# Everything but an IPA asked for on stdout goes to stderr, so `ipa --out -` is the file alone.
exec 3>&1 1>&2

here="$(cd "$(dirname "$0")" && pwd)"
work="${IOS_REMOTE_DIR:-$HOME/Library/Caches/WeirdStreamer-remote}"
src="$work/src"
derived="$work/DerivedData"
logs="$work/logs"
sim_name="WeirdStreamer Remote"
# SSH sessions start with a short PATH: Homebrew's tools (cmake, pkg-config) for build-libass.sh.
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

cmd="${1:-}"
[ -n "$cmd" ] && shift
ref="origin/main" patch="" verbose=0 out=""
while [ $# -gt 0 ]; do
    case "$1" in
        --ref) ref="$2"; shift 2 ;;
        --patch) patch="$2"; shift 2 ;;
        --out) out="$2"; shift 2 ;;
        --verbose) verbose=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
    esac
done
case "$cmd" in
    build|test|ipa|log|status) ;;
    ""|-h|--help|help) usage; exit 0 ;;
    *) echo "Unknown command: $cmd" >&2; usage; exit 2 ;;
esac
case "$ref" in -*|*[!A-Za-z0-9._/-]*) echo "Not a commit or branch: $ref" >&2; exit 2 ;; esac
mkdir -p "$work" "$logs"

if [ "$cmd" = log ]; then
    last="$(ls -t "$logs"/*.log 2>/dev/null | head -1 || true)"
    [ -n "$last" ] || { echo "No runs yet." >&2; exit 1; }
    cat "$last" >&3
    exit 0
fi

# The simulator tests run on: one of its own, so they don't disturb (or depend on) any other.
simulator() {
    local id
    id="$(xcrun simctl list devices available | grep -F "$sim_name (" | grep -oE '[0-9A-F-]{36}' | head -1 || true)"
    if [ -z "$id" ]; then
        local type
        type="$(xcrun simctl list devicetypes | grep -oE 'com\.apple\.CoreSimulator\.SimDeviceType\.iPhone-[0-9]+-Pro\b' | sort -V | tail -1)"
        [ -n "$type" ] || { echo "No iPhone simulator type found: install an iOS simulator runtime in Xcode." >&2; exit 1; }
        id="$(xcrun simctl create "$sim_name" "$type")"
        echo "Made the simulator \"$sim_name\" ($id)." >&2
    fi
    echo "$id"
}

if [ "$cmd" = status ]; then
    if [ -d "$src/.git" ]; then
        echo "Clone: $src" >&2
        git -C "$src" log -1 --format='Checked out: %h %s (%cr)' >&2
        git -C "$src" status --short | sed 's/^/  changed: /' >&2
    else
        echo "No clone yet ($src): the first build makes it." >&2
    fi
    echo "Simulator: $sim_name $(simulator)" >&2
    exit 0
fi

# One run at a time (the clone and DerivedData are shared): wait for one that's going.
lock="$work/lock"
waited=0
until mkdir "$lock" 2>/dev/null; do
    holder="$(cat "$lock/pid" 2>/dev/null || true)"
    if [ -n "$holder" ] && ! kill -0 "$holder" 2>/dev/null; then
        rm -rf "$lock" # left by a run that's gone
        continue
    fi
    [ $waited -eq 0 ] && echo "Another run is going (pid ${holder:-?}); waiting…" >&2
    waited=$((waited + 1))
    [ $waited -gt 360 ] && { echo "Gave up waiting after 30 minutes." >&2; exit 1; }
    sleep 5
done
echo $$ > "$lock/pid"
incoming="$work/incoming.patch"
trap 'rm -rf "$lock" "$incoming"' EXIT

# The patch comes first: on stdin it's read before anything else can take stdin.
if [ "$patch" = "-" ]; then
    cat > "$incoming"
elif [ -n "$patch" ]; then
    cp "$patch" "$incoming"
else
    : > "$incoming"
fi

# The source: origin's ref, plus the patch.
if [ ! -d "$src/.git" ]; then
    origin_url="$(git -C "$here" remote get-url origin)"
    echo "Cloning $origin_url…" >&2
    git clone --quiet "$origin_url" "$src"
fi
git -C "$src" fetch --quiet --prune origin
git -C "$src" reset --quiet --hard
git -C "$src" clean -fdq # ignored files (the renderer's libraries) stay
if ! commit="$(git -C "$src" rev-parse --verify --quiet "$ref^{commit}")"; then
    echo "$ref isn't on origin. Push it, or send what isn't pushed as a patch (mac-ios does)." >&2
    exit 1
fi
git -C "$src" checkout --quiet --detach "$commit"
label="$(git -C "$src" log -1 --format='%h %s' "$commit")"
if [ -s "$incoming" ]; then
    if ! git -C "$src" apply --binary --whitespace=nowarn "$incoming"; then
        echo "The patch doesn't apply on $label." >&2
        exit 1
    fi
    files="$(git -C "$src" apply --numstat "$incoming" | wc -l | tr -d ' ')"
    echo "Building $label, plus $files changed file(s) from the patch." >&2
else
    echo "Building $label." >&2
fi

# The subtitle renderer's libraries: built once in the clone; after that only assrender.c is
# compiled again, when it changed.
if ! "$src/iOS/native/build-libass.sh" > "$logs/native.log" 2>&1; then
    tail -20 "$logs/native.log" >&2
    echo "Building the subtitle renderer failed (full log: $logs/native.log)." >&2
    exit 1
fi

log="$logs/$(date +%Y%m%d-%H%M%S)-$cmd.log"
ls -t "$logs"/2*.log 2>/dev/null | tail -n +20 | while read -r old; do rm -f "$old"; done # the last 20 stay
project="$src/iOS/WeirdStreamer/WeirdStreamer.xcodeproj"
# The build number is the commit count, as Android's versionCode.
build_number="$(git -C "$src" rev-list --count "$commit")"

# Runs xcodebuild, keeping its whole output in the log and showing errors, test results and the
# outcome (everything with --verbose).
run_xcodebuild() {
    local status=0
    if [ $verbose -eq 1 ]; then
        xcodebuild "$@" 2>&1 | tee "$log" >&2 || status=${PIPESTATUS[0]}
    else
        xcodebuild "$@" > "$log" 2>&1 || status=$?
        grep -E ' error: |^error: |✘|recorded an issue|Test run with|Executed [0-9]+ test|\*\* [A-Z ]+ \*\*' "$log" \
            | grep -v -e '^Test Suite' -e 'Executed 0 tests' | sed -E 's#'"$src"'/##' | awk '!seen[$0]++' | head -80 >&2 || true
    fi
    if [ $status -ne 0 ]; then
        echo "xcodebuild failed ($status). Full log: \`ios-remote.sh log\` (or $log on the Mac)." >&2
    fi
    return $status
}

case "$cmd" in
    build)
        run_xcodebuild build -project "$project" -scheme WeirdStreamer -destination "id=$(simulator)" \
            -derivedDataPath "$derived" CODE_SIGN_IDENTITY=-
        ;;
    test)
        # Signed ad hoc: the app (the tests' host) uses the Keychain, which unsigned builds can't.
        run_xcodebuild test -project "$project" -scheme WeirdStreamer -destination "id=$(simulator)" \
            -derivedDataPath "$derived" -parallel-testing-enabled NO CODE_SIGN_IDENTITY=-
        ;;
    ipa)
        # Unsigned: AltStore signs it with the installing Apple ID.
        run_xcodebuild build -project "$project" -scheme WeirdStreamer -configuration Release \
            -destination 'generic/platform=iOS' -derivedDataPath "$derived" \
            CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO CURRENT_PROJECT_VERSION="$build_number"
        app="$derived/Build/Products/Release-iphoneos/WeirdStreamer.app"
        [ -d "$app" ] || { echo "No app was built at $app." >&2; exit 1; }
        name="WeirdStreamer-$build_number$([ -s "$incoming" ] && echo -patched || true).ipa" # (no patch: the test fails, which set -e would end on)
        staging="$work/ipa"
        rm -rf "$staging" && mkdir -p "$staging/Payload"
        cp -R "$app" "$staging/Payload/"
        (cd "$staging" && zip -qry "$name" Payload)
        mkdir -p "$work/out"
        mv "$staging/$name" "$work/out/$name"
        ls -t "$work/out"/*.ipa | tail -n +6 | while read -r old; do rm -f "$old"; done # the last 5 stay
        rm -rf "$staging"
        echo "IPA: $name ($(du -h "$work/out/$name" | cut -f1 | tr -d ' '), build $build_number)." >&2
        case "$out" in
            "") echo "Saved on the Mac: $work/out/$name" >&2 ;;
            -) cat "$work/out/$name" >&3 ;;
            *) cp "$work/out/$name" "$out"; echo "Saved: $out" >&2 ;;
        esac
        ;;
esac
