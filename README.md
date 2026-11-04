<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="Logo/wordmark-transparent.png">
    <img src="Logo/wordmark.png" alt="Weird Streamer" width="440">
  </picture>
</p>

Weird Streamer is a self-hosted media server for your own videos, photos and documents. A Go
server indexes your folders and streams them, as they are or transcoded on the fly, to a web UI
and to one Android app that runs on both Android TV and phones.

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
- **Skips intros and credits** in the Android app, from what the server finds in each season.
- **Offline downloads** on the phone: the original, a copy converted for the phone where it can't
  play the original, or a smaller version, with their subtitles.
- **Users and groups:** each user is in a group, and each library lists the groups that may see
  it. Admins manage libraries, users and groups from Settings on the web or the phone, and see who
  is watching what on the Streams page.
- **The Android app** also browses SMB shares directly, without a server.
- **Runs at home:** HTTPS with HTTP/2, works behind a reverse proxy such as nginx, and keeps sign-in
  and access logs.

## Install

### Server

Download `weirdstreamer-server-<version>-linux-amd64.tar.gz` from the latest release, then:

```bash
tar xzf weirdstreamer-server-*-linux-amd64.tar.gz
cd weirdstreamer-*/
./setup.sh
```

`setup.sh` asks for the server's name, its port and whether to use HTTPS, writes `config.yml`,
and creates the admin user. It can also install a systemd service that starts the server at boot.
Then it opens the web UI. Sign in as the admin: Settings opens where you add your libraries and,
for movie and TV libraries, a TMDB API token.

Without the service, run `./bin/server` from that folder. The server needs `ffmpeg` and
`ffprobe` (see [ffmpeg](#ffmpeg) below), and HEIC photos need `heif-convert` (see
[HEIC photos](#heic-photos)). Every setting is explained in `config.example.yml`.

### ffmpeg

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
everything but disc images works.

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

### HEIC photos

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
(sideloading), then add your server by its address.

## Build from source

- **Server:** Go 1.24 with a C compiler (for SQLite), and [ffmpeg](#ffmpeg). `Server/start-server.sh` builds
  it, walks through the configuration and runs it. See [Server/README.md](Server/README.md),
  which also covers Docker.
- **Android app:** a JDK 17 or later and the Android SDK, then `./gradlew assembleDebug` in
  `Android/`.
- **A release:** `./release.sh` checks the tools, runs the server tests and builds everything in
  `dist/<version>/`: the server package, the APK and their checksums.

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
| `Android` | The Android TV and phone app (Kotlin, Jetpack Compose, ExoPlayer) |
| `Logo` | The app icon and wordmark |

## License

Copyright (C) 2026 Ash Gandhi.

Weird Streamer is free software under the GNU Affero General Public License, version 3 or (at
your option) any later version: see [LICENSE](LICENSE) and [NOTICE](NOTICE). If you run a
modified version for other people over a network, the AGPL asks you to offer them its source.
