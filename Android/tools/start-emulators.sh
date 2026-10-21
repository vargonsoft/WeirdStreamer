#!/usr/bin/env bash
# Starts the test emulators if they aren't running: Android TV (emulator-5554) and a phone
# (emulator-5556), then waits until both have booted. AVDs live in ~/.config/.android/avd.
set -euo pipefail
SDK=$HOME/Android/Sdk
ADB=$SDK/platform-tools/adb
export ANDROID_AVD_HOME=${ANDROID_AVD_HOME:-$HOME/.config/.android/avd}
running() { "$ADB" devices | grep -q "^$1[[:space:]]"; }
start() { # avd port
    if running "emulator-$2"; then echo "emulator-$2 ($1) already running"; return; fi
    echo "Starting $1 on emulator-$2"
    nohup "$SDK/emulator/emulator" -avd "$1" -port "$2" -no-snapshot-save -no-boot-anim \
        -netdelay none -netspeed full >"/tmp/emulator-$2.log" 2>&1 &
}
start Television_1080p_API_34 5554
start Medium_Phone_API_35 5556
for s in emulator-5554 emulator-5556; do
    "$ADB" -s "$s" wait-for-device
    until [ "$("$ADB" -s "$s" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ]; do sleep 2; done
    echo "$s booted"
done
