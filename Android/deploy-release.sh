#!/usr/bin/env bash
# Builds the release APK and installs it on the destinations given, and only those:
#
#   --emulators   the running emulators: installs, launches and checks for a crash (smoke test)
#   --onn         the Onn TV box (onn. 4K Plus Streaming)
#   --moto        the Moto G (moto g power - 2025)
#   --fold7       the Galaxy Fold 7 (SM-F966U1)
#   --no-build    use the APK already built
#
# For example: ./deploy-release.sh --emulators --moto --fold7
# Emulators go first: if the app crashes on one, nothing else is installed. Exits non-zero if the
# build fails, the app crashes, or a destination given isn't connected or its install fails.
set -euo pipefail

cd "$(dirname "$0")"
ADB="${ADB:-$HOME/Android/Sdk/platform-tools/adb}"
APK=app/build/outputs/apk/release/app-release.apk
PACKAGE=com.vargon.weirdstreamer
ACTIVITY=$PACKAGE/.MainActivity

build=1 emulators=0 onn=0 moto=0 fold7=0
for arg in "$@"; do
    case "$arg" in
        --emulators) emulators=1 ;;
        --onn) onn=1 ;;
        --moto) moto=1 ;;
        --fold7) fold7=1 ;;
        --no-build) build=0 ;;
        -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
        *) echo "Unknown option: $arg" >&2; exit 2 ;;
    esac
done
if [ $((emulators + onn + moto + fold7)) = 0 ]; then
    echo "No destination given: pass --emulators, --onn, --moto and/or --fold7 (see --help)." >&2
    exit 2
fi

# The real devices, by model (ro.product.model): their name, and whether they were asked for.
device_name() {
    case "$1" in
        "moto g power"*) echo "Moto G" ;;
        SM-F966*) echo "Galaxy Fold 7" ;;   # Galaxy Z Fold7 (the user's reports SM-F966U1)
        "onn. 4K Plus"*) echo "Onn" ;;
    esac
    return 0
}
wanted() {
    case "$1" in
        "Moto G") [ "$moto" = 1 ] ;;
        "Galaxy Fold 7") [ "$fold7" = 1 ] ;;
        Onn) [ "$onn" = 1 ] ;;
        *) return 1 ;;
    esac
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

installed=0
for name in "Onn" "Galaxy Fold 7" "Moto G"; do
    wanted "$name" || continue
    serial=""
    for d in "${devices[@]}"; do
        [[ "${d%%|*}" == emulator-* ]] && continue
        [ "$(device_name "${d#*|}")" = "$name" ] && serial=${d%%|*} model=${d#*|}
    done
    if [ -z "$serial" ]; then
        echo "== $name isn't connected" >&2
        failed=1
        continue
    fi
    echo "== Installing on $name ($model)"
    if "$ADB" -s "$serial" install -r "$APK" 2>&1 | tail -1 | grep -q Success; then
        echo "   ok"
        installed=$((installed + 1))
    else
        echo "   FAILED on $name" >&2
        failed=1
    fi
done
echo "== Done: installed on $installed device(s)"
exit "$failed"
