#!/usr/bin/env bash
# Builds the release APK and installs it on the destinations given, and only those. Run with
# --help for the destinations: --emulators, --web, and the real devices listed in devices.local.
#
# For example: ./deploy-release.sh --emulators --web
# Emulators go first: if the app crashes on one, nothing else is installed. Exits non-zero if the
# build fails, the app crashes, or a destination given isn't connected or its install fails.
set -euo pipefail

cd "$(dirname "$0")"
ADB="${ADB:-$HOME/Android/Sdk/platform-tools/adb}"
APK=app/build/outputs/apk/release/app-release.apk
PACKAGE=com.vargon.weirdstreamer
ACTIVITY=$PACKAGE/.MainActivity
DEVICES=devices.local

# The real devices, from devices.local (git-ignored: one per line, "flag|display name|model
# prefix"). The flag is the option (--flag); the prefix starts the device's ro.product.model.
local_flags=() local_names=() local_models=() local_wanted=()
if [ -f "$DEVICES" ]; then
    lineno=0
    while IFS='|' read -r flag name model || [ -n "$flag$name$model" ]; do
        lineno=$((lineno + 1))
        [[ -z "$flag" || "$flag" == \#* ]] && continue
        if [[ ! "$flag" =~ ^[a-z][a-z0-9-]*$ ]] || [ -z "$name" ] || [ -z "$model" ]; then
            echo "$DEVICES line $lineno: want flag|display name|model prefix, got: $flag|$name|$model" >&2
            exit 2
        fi
        case "$flag" in emulators|web|no-build|help)
            echo "$DEVICES line $lineno: '$flag' is a built-in option, pick another flag" >&2
            exit 2 ;;
        esac
        local_flags+=("$flag") local_names+=("$name") local_models+=("$model") local_wanted+=(0)
    done <"$DEVICES"
fi

usage() {
    cat <<EOF
Usage: deploy-release.sh [destinations] [--no-build]

Builds the release APK and installs it on the destinations given, and only those:

  --emulators   the running emulators: installs, launches and checks for a crash (smoke test)
  --web         the web UI's download links (Server/web/downloads/WeirdStreamer.apk)
  --no-build    use the APK already built
EOF
    if [ "${#local_flags[@]}" -gt 0 ]; then
        echo
        echo "Real devices (from Android/$DEVICES):"
        for i in "${!local_flags[@]}"; do
            printf '  --%-12s %s (model starts "%s")\n' "${local_flags[$i]}" "${local_names[$i]}" "${local_models[$i]}"
        done
    fi
    cat <<EOF

Devices: add a line to Android/$DEVICES (git-ignored), one per device:

  flag|display name|model prefix

  flag          the option, lowercase: --flag
  display name  shown in messages
  model prefix  the start of the device's model, from: adb shell getprop ro.product.model

For example, a phone whose model is "Pixel 8 Pro" becomes this line, and then --pixel installs on it:

  pixel|Pixel|Pixel 8

Lines starting with # are comments. Only a device whose model matches is installed on, and it must
be connected. The file is never committed, so the list stays with its owner.
EOF
}

build=1 emulators=0 web=0
for arg in "$@"; do
    case "$arg" in
        --emulators) emulators=1 ;;
        --no-build) build=0 ;;
        --web) web=1 ;;
        -h|--help) usage; exit 0 ;;
        *)
            matched=0
            for i in "${!local_flags[@]}"; do
                if [ "$arg" = "--${local_flags[$i]}" ]; then
                    local_wanted[$i]=1
                    matched=1
                fi
            done
            if [ "$matched" = 0 ]; then
                echo "Unknown option: $arg" >&2
                exit 2
            fi
            ;;
    esac
done

selected=0
for w in "${local_wanted[@]+"${local_wanted[@]}"}"; do selected=$((selected + w)); done
if [ $((emulators + web + selected)) = 0 ]; then
    echo "No destination given: pass --emulators, --web and/or a device from $DEVICES (see --help)." >&2
    exit 2
fi

if [ "$build" = 1 ]; then
    echo "== Building release"
    ./gradlew :app:assembleRelease -q
fi
[ -f "$APK" ] || { echo "No APK at $APK" >&2; exit 1; }
echo "   $APK ($(du -h "$APK" | cut -f1))"

# Every connected device, as "serial|model".
devices=()
while read -r serial state _; do
    [ "$state" = device ] || continue
    devices+=("$serial|$("$ADB" -s "$serial" shell getprop ro.product.model </dev/null | tr -d '\r')")
done < <("$ADB" devices | tail -n +2)

failed=0

# Smoke test: install and launch on each emulator (TV and phone take different code paths), and
# fail on any crash in the first seconds.
if [ "$emulators" = 1 ]; then
    found=0
    for d in "${devices[@]}"; do
        serial=${d%%|*} model=${d#*|}
        [[ "$serial" == emulator-* ]] || continue
        found=$((found + 1))
        echo "== Smoke test on $serial ($model)"
        "$ADB" -s "$serial" install -r "$APK" >/dev/null
        "$ADB" -s "$serial" logcat -c -b crash
        "$ADB" -s "$serial" shell am force-stop "$PACKAGE"
        "$ADB" -s "$serial" shell am start -n "$ACTIVITY" >/dev/null
        sleep 8
        if "$ADB" -s "$serial" logcat -d -b crash | grep -q "FATAL EXCEPTION"; then
            echo "   CRASHED. Nothing else was installed. Crash log:" >&2
            "$ADB" -s "$serial" logcat -d -b crash | head -40 >&2
            exit 1
        fi
        if ! "$ADB" -s "$serial" shell pidof "$PACKAGE" >/dev/null; then
            echo "   The app isn't running after launch. Nothing else was installed." >&2
            exit 1
        fi
        echo "   ok"
    done
    if [ "$found" = 0 ]; then
        echo "== --emulators: no emulator is running" >&2
        failed=1
    fi
fi

# The web UI's download links: the live and test servers serve Server/web from disk.
if [ "$web" = 1 ]; then
    downloads=../Server/web/downloads
    mkdir -p "$downloads"
    cp -p "$APK" "$downloads/WeirdStreamer.apk.new" # keeping its build date, which the links show
    mv "$downloads/WeirdStreamer.apk.new" "$downloads/WeirdStreamer.apk" # never half a file
    # What the links show: version, build number (versionCode), size and build date.
    python3 - "$(dirname "$APK")/output-metadata.json" "$downloads/WeirdStreamer.apk" >"$downloads/WeirdStreamer.json" <<'PY'
import datetime, json, os, sys
element = json.load(open(sys.argv[1]))["elements"][0]
built = datetime.datetime.fromtimestamp(os.path.getmtime(sys.argv[2]), datetime.timezone.utc)
print(json.dumps({"version": element["versionName"], "build": element["versionCode"],
                  "size": os.path.getsize(sys.argv[2]), "built": built.isoformat(timespec="seconds")}))
PY
    echo "== Published for download: Server/web/downloads/WeirdStreamer.apk ($(cat "$downloads/WeirdStreamer.json"))"
fi

installed=0
for i in "${!local_flags[@]}"; do
    [ "${local_wanted[$i]}" = 1 ] || continue
    name=${local_names[$i]} prefix=${local_models[$i]}
    serial="" model=""
    for d in "${devices[@]}"; do
        [[ "${d%%|*}" == emulator-* ]] && continue
        if [[ "${d#*|}" == "$prefix"* ]]; then serial=${d%%|*} model=${d#*|}; fi
    done
    if [ -z "$serial" ]; then
        echo "== $name isn't connected" >&2
        failed=1
        continue
    fi
    echo "== Installing on $name ($model)"
    result=$("$ADB" -s "$serial" install -r "$APK" 2>&1) || true
    if grep -q Success <<<"$result"; then
        echo "   ok"
        installed=$((installed + 1))
    else
        echo "   FAILED on $name:" >&2
        tail -3 <<<"$result" | sed 's/^/     /' >&2
        failed=1
    fi
done
echo "== Done: installed on $installed device(s)"
exit "$failed"
