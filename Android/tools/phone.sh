#!/usr/bin/env bash
# Drives the phone emulator.
#   phone.sh shot OUT.png             screenshot
#   phone.sh tap X Y                  tap at screen pixels (1080x2400)
#   phone.sh tapt "Text"              tap the element whose text or content-description is "Text"
#   phone.sh texts                    list the texts and content-descriptions on screen
#   phone.sh swipe X1 Y1 X2 Y2 [MS]   swipe
set -euo pipefail
ADB=$HOME/Android/Sdk/platform-tools/adb; D=${PHONE_SERIAL:-emulator-5556}
dump() { "$ADB" -s "$D" shell uiautomator dump /sdcard/u.xml >/dev/null; "$ADB" -s "$D" shell cat /sdcard/u.xml; }
case "${1:-}" in
    shot) "$ADB" -s "$D" exec-out screencap -p > "$2" ;;
    tap) "$ADB" -s "$D" shell input tap "$2" "$3"; sleep 1.5 ;;
    swipe) "$ADB" -s "$D" shell input swipe "$2" "$3" "$4" "$5" "${6:-300}"; sleep 1 ;;
    texts) dump | grep -o '\(text\|content-desc\)="[^"]\+"' ;;
    tapt)
        b=$(dump | grep -o "\(text\|content-desc\)=\"$2\"[^>]*bounds=\"[^\"]*\"" | head -1 | grep -o 'bounds="[^"]*"' | grep -o '[0-9]\+' || true)
        set -- $b
        [ -n "${4:-}" ] || { echo "not found" >&2; exit 1; }
        "$ADB" -s "$D" shell input tap $(( ($1+$3)/2 )) $(( ($2+$4)/2 )); sleep 1.5 ;;
    *) sed -n '2,8p' "$0"; exit 2 ;;
esac
