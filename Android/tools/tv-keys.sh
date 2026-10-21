#!/usr/bin/env bash
# Presses keys on the TV emulator, then saves a screenshot.
#   tv-keys.sh OUT.png [KEY ...]     keys: DPAD_UP/DOWN/LEFT/RIGHT/CENTER, BACK, HOME, ...
#   tv-keys.sh OUT.png HOLD_CENTER   holds OK for 1.5s (a real long press; --longpress is too short)
set -euo pipefail
ADB=$HOME/Android/Sdk/platform-tools/adb; D=${TV_SERIAL:-emulator-5554}
out=$1; shift
for k in "$@"; do
    if [ "$k" = HOLD_CENTER ]; then
        "$ADB" -s "$D" emu event send EV_KEY:KEY_ENTER:1 >/dev/null; sleep 1.5
        "$ADB" -s "$D" emu event send EV_KEY:KEY_ENTER:0 >/dev/null
    else
        "$ADB" -s "$D" shell input keyevent "$k"
    fi
    sleep 0.7
done
sleep 1.5
"$ADB" -s "$D" exec-out screencap -p > "$out"
