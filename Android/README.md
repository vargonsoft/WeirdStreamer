# WeirdStreamer for Android

The Android app for a WeirdStreamer server, for Android TV, phones and tablets: one app, and it picks
its screens from the device (the TV's screens on Android TV, the phone's on phones and tablets).
The server's README is [Server/README.md](../Server/README.md), and the iPhone and iPad app's is
[iOS/README.md](../iOS/README.md). The main README covers the whole project.

## Install

The server offers the app for download from its web UI (the sign-in page, or Settings under My
Account), and each release attaches the same APK. It runs on Android TV, on phones and on tablets. It
needs Android 10 (API 29) or later. Install it from the file (allow installs from your browser or
file manager when asked), then add your server by its name or address.

## Build

You need a JDK 17 or later, the Android SDK, and the Android NDK r29 (set `ANDROID_NDK_HOME`, or
the app looks in `~/Android/Sdk/ndk/29.0.14206865`). The build also needs cmake, make, pkg-config
and curl. The ASS subtitle renderer is built with them: `native/build-libass.sh` builds libass
0.17.5 and what it needs (HarfBuzz, FriBiDi, FreeType) from pinned, checksummed sources, and Gradle
runs it before each build. Its output is git-ignored, and the first build takes a while.

```bash
./gradlew assembleDebug
```

The debug APK is `app/build/outputs/apk/debug/app-debug.apk`.

## Test

The Android app has no unit tests yet. Test on an emulator with the server from
`Server/scripts/run-e2e.sh` (port 9100): from an emulator, the server's address is
`http://10.0.2.2:9100`. `tools/start-emulators.sh` starts the TV and phone emulators, and
`tools/phone.sh` and `tools/tv-keys.sh` drive them (screenshots, taps and key presses).

## Release

`deploy-release.sh` builds the release APK (R8, with the keep rules in `app/proguard-rules.pro`)
and installs it only on the destinations you name: `--emulators`, `--web` (which copies the APK to the
server's download folder), and any real devices listed in `devices.local` (git-ignored; see
`./deploy-release.sh --help`). Run `./release.sh` at the repository's root to build a full release.

## Layout

| Path | What's in it |
| --- | --- |
| `app/src/main/java/com/vargon/weirdstreamer/` | The app: `MediaViewModel.kt`, `api/`, `repository/`, `downloads/`, and the screens in `ui/components/` (`Phone*` for phones, the rest for the TV) |
| `app/src/main/cpp/` | `assrender.c`, the libass renderer behind the JNI calls in `AssRenderer.kt` |
| `native/` | `build-libass.sh` and the pinned library versions |
| `tools/` | Emulator and device helper scripts |
