#!/usr/bin/env bash
# Builds the release APK, checks that it launches without crashing on the running emulators, then
# installs it on the allowed real devices that are connected.
#
#   ./deploy-release.sh            build, smoke test, install on the connected Moto G and Galaxy Fold 7
#   ./deploy-release.sh --onn      also install on the Onn TV box (left out by default: it's in use
#                                  during the day)
#   ./deploy-release.sh --no-build use the APK already built
#   ./deploy-release.sh --no-install   build and smoke test only
#
# Exits non-zero, before installing anything, if the build fails or the app crashes on an emulator.
set -euo pipefail

cd "$(dirname "$0")"
ADB="${ADB:-$HOME/Android/Sdk/platform-tools/adb}"
APK=app/build/outputs/apk/release/app-release.apk
PACKAGE=com.vargon.weirdstreamer
ACTIVITY=$PACKAGE/.MainActivity

with_onn=0 build=1 install=1
for arg in "$@"; do
    case "$arg" in
        --onn) with_onn=1 ;;
        --no-build) build=0 ;;
        --no-install) install=0 ;;
        -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
        *) echo "Unknown option: $arg" >&2; exit 2 ;;
    esac
done

# Which real devices may get the app, by model (ro.product.model). The Onn is opt-in.
allowed_device() {
    case "$1" in
        "moto g power"*) echo "Moto G" ;;
        SM-F966*|SM-F9660*) echo "Galaxy Fold 7" ;;   # Galaxy Z Fold7 model numbers
        "onn. 4K Plus"*) [ "$with_onn" = 1 ] && echo "Onn" ;;
    esac
    return 0
}

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

# Smoke test: install and launch on each emulator (TV and phone take different code paths), and
# fail on any crash in the first seconds.
emulators=0
for d in "${devices[@]}"; do
    serial=${d%%|*} model=${d#*|}
    [[ "$serial" == emulator-* ]] || continue
    emulators=$((emulators + 1))
    echo "== Smoke test on $serial ($model)"
    "$ADB" -s "$serial" install -r "$APK" >/dev/null
    "$ADB" -s "$serial" logcat -c -b crash
    "$ADB" -s "$serial" shell am force-stop "$PACKAGE"
    "$ADB" -s "$serial" shell am start -n "$ACTIVITY" >/dev/null
    sleep 8
    if "$ADB" -s "$serial" logcat -d -b crash | grep -q "FATAL EXCEPTION"; then
        echo "   CRASHED. Nothing was installed on devices. Crash log:" >&2
        "$ADB" -s "$serial" logcat -d -b crash | head -40 >&2
        exit 1
    fi
    if ! "$ADB" -s "$serial" shell pidof "$PACKAGE" >/dev/null; then
        echo "   The app isn't running after launch. Nothing was installed on devices." >&2
        exit 1
    fi
    echo "   ok"
done
if [ "$emulators" = 0 ]; then
    echo "No emulator running: can't smoke test, so nothing was installed. Start one and rerun." >&2
    exit 1
fi

[ "$install" = 1 ] || { echo "== Skipping installs (--no-install)"; exit 0; }

installed=0
for d in "${devices[@]}"; do
    serial=${d%%|*} model=${d#*|}
    [[ "$serial" == emulator-* ]] && continue
    name=$(allowed_device "$model")
    if [ -z "$name" ]; then
        case "$model" in
            "onn. 4K Plus"*) echo "== Skipping Onn ($model): pass --onn to include it" ;;
            *) echo "== Skipping $model ($serial): not an allowed device" ;;
        esac
        continue
    fi
    echo "== Installing on $name ($model)"
    if "$ADB" -s "$serial" install -r "$APK" 2>&1 | tail -1 | grep -q Success; then
        echo "   ok"
        installed=$((installed + 1))
    else
        echo "   FAILED on $name" >&2
    fi
done
echo "== Done: installed on $installed device(s)"
