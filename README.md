<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="Logo/wordmark-transparent.png">
    <img src="Logo/wordmark.png" alt="Weird Streamer" width="440">
  </picture>
</p>

Weird Streamer is a self-hosted media server for your own videos, photos and documents. A Go
server indexes your folders and streams them, as they are or transcoded on the fly, to a web UI
and to two apps: one Android app for Android TV, phones and tablets, and one app for iPhone and iPad.

- [Features](#features)
- [Install](#install)
  - [Server](#server)
    - [Linux](#linux)
    - [macOS](#macos)
    - [Hardware encoding](#hardware-encoding)
    - [ffmpeg](#ffmpeg)
    - [CJK subtitle font](#cjk-subtitle-font)
    - [HEIC photos](#heic-photos)
  - [Android app](#android-app)
  - [iPhone and iPad app](#iphone-and-ipad-app)
- [Build from source](#build-from-source)
- [Repository layout](#repository-layout)
- [License](#license)

App READMEs: [Server](Server/README.md), [Android](Android/README.md), [iPhone](iOS/README.md).

## Features

- **Libraries:** a library is a folder. Movie and TV libraries are matched on TMDB, for posters,
  titles and seasons; any other folder is browsed as it is, photos and PDFs included.
- **Plays nearly anything:** each player checks what its device or browser can decode, and the
  server copies or transcodes only what it can't play, down to a single audio track. Compatibility
  and size modes are in every player's Transcode menu.
- **DVD images:** ISOs play like any video, with their titles, chapters, angles and audio tracks.
- **Subtitles:** text and picture (PGS) tracks, files beside the video, and search and download
  from OpenSubtitles. Forced subtitles show on their own; choices are remembered per video.
- **Picks up where you left off:** progress is kept per user across every device, with Continue
  Watching, Recently Added, and seek-bar previews.
- **Skips intros and credits** on the web and in the Android app, from what the server finds in
  each season (chapters, the sound a season's episodes share, dark frames) or an admin marks: a
  Skip button, or Auto Skip per show. Skipping credits, or reaching the end, goes on to the next
  episode.
- **The video players** (web, TV and phone) have their own controls, the same Transcode menu,
  seek-bar previews, and title, chapter, angle and audio choices for DVD images. Each has a
  subtitle menu with online search and a timing correction (kept on the server, for every player).
  Besides that:
  - **TV:** zoom, fill, aspect ratio (16:9, 21:9, 4:3…), rotation and playback speed, all from
    the remote.
  - **Phone:** pinch to zoom and pan, aspect ratio, rotation and playback speed, and
    picture-in-picture when you go home while a video plays.
  - **Web:** a choice of audio track (on Android, only a disc's), and fullscreen with controls
    that hide.
- **Offline downloads** on the phone: the original, a copy converted for the phone where it can't
  play the original, or a smaller version, with their subtitles, intro and credits skips, and
  seek-bar previews. With **Download ahead** on (Settings › Downloads), watching an episode
  downloads the next 3 to 5 in the background (more on a faster connection, Wi-Fi only unless you
  allow mobile data), so a show is there on a flight or when the connection drops; they're removed
  once you've watched past them.
- **Users and groups:** each user is in a group, and each library lists the groups that may see
  it. Admins manage libraries, users and groups from Settings on the web or the phone, and see who
  is watching what on the Streams page.
- **Photos and PDFs** in the Android app: swipe through a folder's photos, zoom and pan (on the TV
  with the D-pad, and rotate), and motion photos play their clip in a loop: Samsung's and Google's,
  and Apple Live Photos (the photo and its .MOV show as one). PDFs page by swipe or D-pad, zoom, and
  show all their pages as thumbnails; the TV also shows two pages side by side. HEIC photos show too
  (see [HEIC photos](#heic-photos)). The web UI shows photos, plays a Live Photo's clip once when it
  opens (LIVE plays it again, with sound), and opens PDFs in the browser.
- **Themes:** Ember (the default) and 20 others, the same on the web and in the Android app,
  picked from a grid of previews in Settings › Themes.
- **The Android app** also browses SMB shares directly, without a server.
- **The iPhone and iPad app** plays the same library: browsing, the same transcode modes, styled (ASS)
  subtitles, PGS, and downloads that play offline with their subtitles and skip data. It runs on
  iOS 17 or later, installed with AltStore or SideStore (see [the iPhone and iPad app](#iphone-and-ipad-app)).
- **Hardware encoding:** transcodes and downloads can be encoded by the GPU instead of the CPU:
  NVIDIA (NVENC), Intel (Quick Sync or VA-API), AMD (VA-API) and Apple Silicon Macs (VideoToolbox).
  The server checks at startup which work and how many encodes NVIDIA's driver allows at once (see
  [Hardware encoding](#hardware-encoding)).
- **Runs at home:** on Linux (x86-64 or ARM64, such as a Raspberry Pi 4 or 5) or an Apple Silicon
  Mac, with HTTPS and HTTP/2, behind a reverse proxy such as nginx if you like, and sign-in and
  access logs.

## Install

### Server

Each [release](https://github.com/vargonsoft/WeirdStreamer/releases/latest) has a server package
for each platform:

| Package | For |
| --- | --- |
| `weirdstreamer-server-<version>-linux-amd64.tar.gz` | Linux on x86-64 (most PCs and servers) |
| `weirdstreamer-server-<version>-linux-arm64.tar.gz` | Linux on ARM64 (Raspberry Pi 4 and 5 with a 64-bit OS, ARM servers) |
| `weirdstreamer-server-<version>-darwin-arm64.tar.gz` | Macs with Apple Silicon (M1 and later) |

Windows isn't supported yet. `SHA256SUMS` in the release lists each file's checksum.

Unpack the package where the server should live (it keeps its data in a `var` folder beside it) and
run `setup.sh` in it. Setup:
- asks for the server's name, its port (9000 unless you pick another) and whether to use HTTPS;
- writes `config.yml` and creates the admin user;
- can set up your graphics card for [hardware encoding](#hardware-encoding);
- can install a service that starts the server at boot;
- then opens the web UI.

Sign in as the admin: Settings opens where you add your libraries (Browse… picks each one's folder on
the server) and, for movie and TV libraries, a TMDB API token. Every setting is explained in
`config.example.yml`.

To upgrade later, run `./setup.sh upgrade` in that folder: it downloads the latest release, checks it,
installs it over this one (your `config.yml`, data and service stay; the files it replaced are kept in
`.previous`) and restarts the service. `./setup.sh upgrade --check` only says whether there's a newer one.

#### Linux

```bash
tar xzf weirdstreamer-server-*-linux-amd64.tar.gz
cd weirdstreamer-*/
./setup.sh
```

(`linux-arm64` on ARM.) The Linux packages need glibc 2.34 or later: Debian 12, Ubuntu 22.04,
RHEL 9, Raspberry Pi OS 12 (64-bit) or newer. The service is a systemd one: `./setup.sh service` installs it later,
`./setup.sh remove-service` removes it. Without it, run `./bin/server` from that folder. The server
needs `ffmpeg` and `ffprobe` (see [ffmpeg](#ffmpeg) below), and HEIC photos need `heif-convert` (see
[HEIC photos](#heic-photos)).

#### macOS

On a Mac with Apple Silicon, with [Homebrew](https://brew.sh) installed:

```bash
tar xzf weirdstreamer-server-*-darwin-arm64.tar.gz
cd weirdstreamer-*/
./setup.sh
```

Homebrew's ffmpeg lacks three things the server uses (the DVD reader, Chromaprint and libass; see
[ffmpeg](#ffmpeg)), so setup offers to build the server's own into the package's `ffmpeg` folder
with Homebrew's libraries (`build-ffmpeg.sh`, a few minutes). The server uses that one first.
Hardware encoding needs nothing installed: VideoToolbox is part of macOS.

The service is a launchd one (`/Library/LaunchDaemons/com.weirdstreamer.server.plist`), started at
boot before anyone logs in. macOS doesn't let a service read Desktop, Documents, Downloads, or
external and network drives until it's allowed: if a library is in one of those, add the package's
`bin/server` to System Settings › Privacy & Security › Full Disk Access, then restart it. Shares
mounted when you log in (in Finder) aren't there at boot, so their libraries are empty until
they're mounted. For HEIC photos, `brew install libheif`.

#### Hardware encoding

When something plays in a format the device can't, or a download is made smaller, the server
encodes video. By default the CPU does that (x264). A GPU can do it instead, which takes most of
that work off the CPU. Tests on a film showed the GPUs' picture as good as x264's at the same file
size:

| GPU | Encoder | What it needs |
| --- | --- | --- |
| NVIDIA | NVENC | NVIDIA's own driver (not nouveau) |
| Intel | Quick Sync, or VA-API | Intel's media driver (and its VPL GPU runtime for Quick Sync); the server's user in the `video` and `render` groups |
| AMD | VA-API | Mesa's VA-API driver; the server's user in the `video` and `render` groups (untested on an AMD card) |
| Apple Silicon Mac | VideoToolbox | Nothing |

On Linux, `./setup.sh gpu` finds your graphics card and lists what's missing. On Debian and Ubuntu
it offers to install the packages and add the groups (`./setup.sh gpu --check` only reports). The
server tests each encoder at startup and logs what it found. Turn it on in Settings › Server ›
Streaming. You can also set how many encodes may run on the GPU at once there; the rest use the CPU.
NVIDIA's GeForce drivers allow only a few at once (8 or 12, depending on the version): the server
measures that at startup and never goes past it.

#### ffmpeg

The server runs `ffmpeg` and `ffprobe` (both come with ffmpeg) for thumbnails, seek-bar previews,
transcoding, subtitles and finding intros. Not every ffmpeg build has all it uses:

| What it needs | For | Without it |
| --- | --- | --- |
| The H.264 encoder, libx264 | Transcoding video | Required |
| libass (the `subtitles` filter) | Burning subtitles into a transcode | Required |
| The DVD reader, `dvdvideo` (ffmpeg 7.0 or later) | Playing disc images (`.iso`) | Discs don't play |
| Chromaprint | Finding intros and credits by the sound a season's episodes share | Only chapters named for them, and credits on black, are found |

The rest (the AAC encoder, MP4 and WebVTT output, the `scale`, `overlay` and `blackdetect`
filters) is in every ffmpeg.

**The easy way:** your system's package. Debian's and Ubuntu's `ffmpeg` has all four, so
`sudo apt install ffmpeg` is enough; with a version older than 7.0 (`ffmpeg -version` shows it),
everything but disc images works. On a Mac, Homebrew's lacks the last three: the macOS package's
setup builds one that has them (see [macOS](#macos)).

**Checking an ffmpeg you have:** each of these prints a line when that part is there. They check
libx264, libass, the DVD reader and Chromaprint, in that order:

```bash
ffmpeg -hide_banner -encoders | grep -w libx264
```

```bash
ffmpeg -hide_banner -filters | grep -w subtitles
```

```bash
ffmpeg -hide_banner -demuxers | grep -w dvdvideo
```

```bash
ffmpeg -hide_banner -muxers | grep -w chromaprint
```

The release package's `check-ffmpeg.sh` does all of this at once (`setup.sh` runs it), and so does
`Server/scripts/check-ffmpeg.sh` in the repository.

**Building ffmpeg yourself,** where no package has what's needed: get its source from
[ffmpeg.org](https://ffmpeg.org/download.html) (7.0 or later) and the libraries it builds with (on
Debian or Ubuntu):

```bash
sudo apt install build-essential nasm pkg-config libx264-dev libass-dev libchromaprint-dev libdvdread-dev libdvdnav-dev zlib1g-dev
```

Then, in ffmpeg's folder, configure it with those libraries switched on (libx264 and the DVD
reader need `--enable-gpl`):

```bash
./configure --enable-gpl --enable-version3 --enable-libx264 --enable-libass --enable-chromaprint --enable-libdvdread --enable-libdvdnav --enable-zlib --disable-debug --disable-doc --disable-ffplay
```

```bash
make -j"$(nproc)" && sudo make install
```

That installs `ffmpeg` and `ffprobe` in `/usr/local/bin`, which comes before the system's on the
usual `PATH`; the systemd service finds them there too.

#### CJK subtitle font

Styled (ASS/SSA) subtitles are drawn with libass, which needs a font for every character. The web
player, the Android apps and the iPhone and iPad app each draw from the fonts they can reach: a browser or a
phone's own fonts rarely include Chinese, Japanese or Korean characters, and a complete CJK font is
20 MB or more. So for the web and the apps the server makes a small subset of one, holding
just the characters a subtitle uses (and basic Latin), which takes a few kilobytes. The iPhone and iPad app
fetches one too, since its system fonts lack some Chinese characters.

It needs two things, both optional:

- **Noto Sans CJK:** found in the usual places (`/usr/share/fonts/opentype/noto/`, Debian's
  `fonts-noto-cjk` package; `/usr/share/fonts/noto/`, Alpine's `font-noto-cjk`), or set
  `media.cjkFontPath` in `config.yml`. The players use its family
  name ("Noto Sans CJK JP"), so other CJK fonts won't be matched.
- **fontTools' `pyftsubset`**, which makes the subsets. On Debian and Ubuntu, install the `fonttools`
  package:

  ```bash
  sudo apt install fonttools
  ```

  Or in a virtualenv, then set `media.subsetCommand` in `config.yml` to the `pyftsubset` it makes
  (the default is `pyftsubset` on the `PATH`):

  ```bash
  python3 -m venv ~/fonttools
  ~/fonttools/bin/pip install fonttools brotli
  ```

Without them, those characters show as boxes; nothing else changes. The iPhone and iPad app's system fonts have
Japanese and Korean, but not simplified Chinese, so it needs them too for those lines.
`Server/scripts/check-ffmpeg.sh` reports both. The Docker image has both (the font adds 19 MB, Python
and fontTools about 60 MB). Otherwise, install them yourself if you want that support: fontTools from
your system's packages, and the Noto Sans CJK font from yours (`fonts-noto-cjk` on Debian and Ubuntu,
`google-noto-sans-cjk-ttc-fonts` on Fedora):

```bash
sudo apt install fonts-noto-cjk
```

#### HEIC photos

Phones save photos as HEIC (`.heic`, `.heif`). The server uses `heif-convert`, from libheif's
tools, to make their thumbnails and to send them to browsers as JPEG (`autoConvertImages`).
Without it, these photos still list but have no thumbnails, and most browsers can't show them.
On Debian or Ubuntu:

```bash
sudo apt install libheif-examples
```

### Android app

The server offers the app for download: use the link on the web UI's sign-in page, or in
Settings under My Account. The same APK is attached to each release. It needs Android 10 or
later, and the same app works on Android TV and on phones. Install it from the file
(sideloading), then add your server by its name or address. Without a port, the app looks for it on
9000, 443, 80 and 8080, over HTTPS or HTTP, and saves the address it found.

Its own README, [Android/README.md](Android/README.md), covers building and testing it.

### iPhone and iPad app

The iPhone and iPad app isn't on the App Store. It runs on iOS 17 or later, and installs with
[AltStore](https://altstore.io) or [SideStore](https://sidestore.io): add this source in it once
(Browse › Sources, +), then install Weird Streamer from it:

```
https://github.com/vargonsoft/WeirdStreamer/releases/latest/download/altstore-source.json
```

Each new release then shows as an update. AltStore signs the app with your Apple ID; with a free
one, the app must be refreshed every 7 days (AltServer on your computer does it, or SideStore on
the phone). Each release also has the IPA itself. Keep the app and the server at the same release.
Add your server by its name or address. The app's README, [iOS/README.md](iOS/README.md), covers
what it does and how to build it yourself.

## Build from source

- **Server:** Go 1.24 with a C compiler (for SQLite), and [ffmpeg](#ffmpeg). `Server/start-server.sh` builds
  it, walks through the configuration and runs it. See [Server/README.md](Server/README.md),
  which also covers Docker.
- **Android app:** a JDK 17 or later, the Android SDK and NDK r29 (`ANDROID_NDK_HOME`, or
  `~/Android/Sdk/ndk/29.0.14206865`), with cmake, make, pkg-config and curl. The build compiles the
  ASS subtitle renderer with them (`Android/native/build-libass.sh`, run by Gradle), then
  `./gradlew assembleDebug` in `Android/`.
- **iPhone and iPad app:** Xcode 26 or later, and cmake and pkg-config (Homebrew). Run
  `iOS/native/build-libass.sh` once, then open `iOS/WeirdStreamer/WeirdStreamer.xcodeproj`. Its
  README has the details and the simulator build.
- **A release:** `./release.sh` checks the tools, runs the server tests and builds everything in
  `dist/<version>/`: the server packages (the ARM64 one cross-compiled with `gcc-aarch64-linux-gnu`,
  the Mac one and the iPhone app built on a Mac over SSH), the APK, the IPA with its AltStore source,
  and their checksums. `--publish` puts them on GitHub as a release.

Server tests run from `Server/src`:

```bash
go vet ./internal/... ./cmd/... && go test ./internal/...
```

## Repository layout

| Folder | What's in it |
| --- | --- |
| `Server/src` | The Go server and its CLI (`cmd/server`, `cmd/cli`, `internal/…`) |
| `Server/web` | The web UI (HTML, CSS and JavaScript, no build step) |
| `Server/scripts`, `Server/package` | Building, running and setting up the server |
| `Server/docker` | Running the server in a container |
| `Android` | The Android TV and phone app (Kotlin, Jetpack Compose, ExoPlayer), with [its README](Android/README.md) |
| `iOS` | The iPhone and iPad app (SwiftUI, SwiftData), with [its README](iOS/README.md) |
| `Logo` | The app icon and wordmark |

## License

Copyright (C) 2026 Ash Gandhi.

Weird Streamer is free software under the GNU Affero General Public License, version 3 or (at
your option) any later version: see [LICENSE](LICENSE) and [NOTICE](NOTICE). If you run a
modified version for other people over a network, the AGPL asks you to offer them its source.
