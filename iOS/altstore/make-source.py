#!/usr/bin/env python3
"""Writes the AltStore source (altstore-source.json) for a release's IPA, to standard output.

AltStore and SideStore install from a source: a JSON file at a fixed URL listing the app and where
its IPA is. Users add the source's URL once; each new release's source is then an update. AltStore
refuses an install whose IPA doesn't match the source (version, build number, permissions), so
everything that must match is read from the IPA itself, never written by hand.

    make-source.py IPA --repo OWNER/NAME --tag vVERSION [--notes TEXT]

The release puts the IPA and this file on GitHub, so the source's address never changes:
https://github.com/OWNER/NAME/releases/latest/download/altstore-source.json
"""
import argparse
import datetime
import json
import os
import plistlib
import sys
import zipfile

ICON = "iOS/WeirdStreamer/WeirdStreamer/Assets.xcassets/AppIcon.appiconset/icon-1024.png"


def app_info(ipa):
    """The app's Info.plist, from Payload/<name>.app/Info.plist in the IPA."""
    with zipfile.ZipFile(ipa) as z:
        for name in z.namelist():
            parts = name.split("/")
            if len(parts) == 3 and parts[0] == "Payload" and parts[1].endswith(".app") and parts[2] == "Info.plist":
                return plistlib.loads(z.read(name))
    sys.exit(f"{ipa} has no Payload/*.app/Info.plist")


def main():
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("ipa")
    p.add_argument("--repo", required=True, help="the GitHub repository, OWNER/NAME")
    p.add_argument("--tag", required=True, help="the release's tag, v<version>")
    p.add_argument("--notes", default="", help="what's new, for AltStore's update screen")
    a = p.parse_args()

    info = app_info(a.ipa)
    version, build = info["CFBundleShortVersionString"], info["CFBundleVersion"]
    asset = os.path.basename(a.ipa)
    # Only the permissions the app asks the user for are listed (AltStore checks them against the
    # app's). The app has no entitlements of its own: it's built unsigned, and AltStore signs it.
    privacy = {k: v for k, v in sorted(info.items()) if k.startswith("NS") and k.endswith("UsageDescription")}
    release = f"https://github.com/{a.repo}/releases/tag/{a.tag}"
    source = {
        "name": "Weird Streamer",
        "identifier": "com.vargon.weirdstreamer.source",
        "subtitle": "The iPhone and iPad app for your own Weird Streamer server.",
        "website": f"https://github.com/{a.repo}",
        "iconURL": f"https://raw.githubusercontent.com/{a.repo}/{a.tag}/{ICON}",
        "tintColor": "F6C445",
        "apps": [{
            "name": info.get("CFBundleDisplayName") or info.get("CFBundleName", "WeirdStreamer"),
            "bundleIdentifier": info["CFBundleIdentifier"],
            "developerName": a.repo.split("/")[0],
            "subtitle": "Watch your own media server's films, shows and photos.",
            "localizedDescription": (
                "The iPhone and iPad app for a Weird Streamer server: browse your libraries, play "
                "anything (the server converts what the phone can't play), pick up where you left "
                "off, with styled subtitles, downloads to watch offline, and Continue Watching "
                "shared with the TV, phone and web apps. You need a Weird Streamer server: "
                f"https://github.com/{a.repo}"),
            "iconURL": f"https://raw.githubusercontent.com/{a.repo}/{a.tag}/{ICON}",
            "tintColor": "F6C445",
            "category": "entertainment",
            "versions": [{
                "version": version,
                "buildVersion": build,
                "date": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
                "localizedDescription": a.notes or f"What's new: {release}",
                "downloadURL": f"https://github.com/{a.repo}/releases/download/{a.tag}/{asset}",
                "size": os.path.getsize(a.ipa),
                "minOSVersion": info.get("MinimumOSVersion", "17.0"),
            }],
            "appPermissions": {"entitlements": [], "privacy": privacy},
        }],
        "news": [],
    }
    json.dump(source, sys.stdout, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
