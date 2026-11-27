# WeirdStreamer

A self-hosted media server (Go) with a web UI (vanilla JS) and an Android app (Kotlin/Compose)
that runs as both an Android TV app and a phone app. License: AGPL-3.0-or-later (`LICENSE`, and the
copyright notice in `NOTICE`); the history was rewritten so no version was ever MIT. Pending work is in [TODO.md](TODO.md); notes
for each TODO item are at the end of this file.

## Rules (from the user)

- **Off limits:** the personal media libraries kept outside this project (the user's own notes say
  which). Don't browse, list, play or screenshot them. The one exception so far: reading
  codec/format details (ffprobe, no frames) of a file the user names.
- **Secrets:** the TMDB token is in `Server/config.yml` and the test password is in
  `Server/e2e/pw`. Never print or repeat either.
- **Live server restarts:** only when asked, with `Server/scripts/restart-live.sh` (below). It won't
  restart while someone is watching (exit code 2, listing the streams): then say who's watching
  and ask before running it with `--force`.
- **Personal devices:** the real ones are listed in `Android/devices.local` (git-ignored; `deploy-release.sh --help`
  shows them). The TV box among them: no testing or installing on it during the day. Use the emulators.
- **Deploying the Android app:** only through `Android/deploy-release.sh` (below), never manual
  `adb install` of a debug build on real devices. Deploy only to the destinations the user names
  (a device's flag only when they ask for that device).
- **Commits:** only when asked. End messages with
  `Co-Authored-By: <the model that made the change> <noreply@anthropic.com>` (the model running the session that edited the code). `origin` is a local bare repo
  (`../repos/WeirdStreamer`); push only when asked. There's also a `github` remote.
- **No backward compatibility:** server and clients change together. Don't keep or add code for
  older app versions or older requests; fix the client instead.
- **Disk paths stay on the server:** clients must never learn where a library is on disk. A file or
  folder is, to every client, its library's ID and its path below the library
  (`01c6653f/EAC3 Test/EAC3 Test.mkv`); a disk path in a request is refused. Only an admin's Settings
  (`/api/config`) shows library folders. See "Client paths" below.
- Never `pkill -f` (it can match the live server or other processes). Stop processes by PID or
  through the tool that started them.

## Layout

- `Server/`: Go, gorilla/mux, SQLite (`mattn/go-sqlite3`, needs CGO). Entry point
  `src/cmd/server/main.go` (routes). Code is in `src/internal/`:
  - `handlers/`: HTTP API. `media.go` (browse, stream, thumbnails), `library.go` (Library view,
    title pages), `playback.go` (per-user progress, Continue Watching), `streams.go` (admin
    Streams page, player progress reports), `downloads.go` (offline-download encodes), `match.go`
    (Fix dialog), `users.go`, `config.go`, `access.go` (library/group access checks).
  - `media/`: `streamer.go` (direct serving and ffmpeg transcoding), `codecs.go` (codec probe,
    audio-only transcodes, track options, WebVTT subtitles), `dvd.go` and
    `dvdstream.go` (ISO playback: a pure-Go ISO9660/IFO parser and ffmpeg dvdvideo),
    `downloads.go`, `streams.go` (active-stream tracker), thumbnails, trickplay.
  - `metadata/`: TMDB matching and the library index. `database/`: SQLite (users DB has
    `users` and `playback`; media DB has items, TMDB data, discs).
  - `web/`: the web UI, `index.html` plus `js/app.js` (about 2,900 lines, no framework),
    `js/api.js` and `css/style.css`. The live server serves `Server/web` straight from disk, so
    web changes need no restart, only a page reload.
- `Android/`: one app for TV and phone (`MainActivity` picks which). Under
  `app/src/main/java/com/vargon/weirdstreamer/`:
  - `MediaViewModel.kt` (state, navigation, playback progress, resume),
    `repository/ServerRepository.kt` (server API), `api/` (Retrofit `ApiService`, Gson models).
  - TV UI: `ui/components/MediaBrowser.kt`, `BrowserGrid.kt`, `LibraryScreens.kt`,
    `VideoPlayer.kt` (TV player), `PlayerDialogs.kt` (TV transcode/subtitle/speed dialogs),
    `PlayerControls.kt`, `PlayerOverlays.kt`, `TvPlayback.kt` (Continue Watching row and its
    menu), `TvFocus.kt` (`tvClickable`).
  - Phone UI: `Phone*.kt` (`PhoneVideoPlayer.kt` is the phone player), `PlaybackUi.kt` (shared
    progress overlays and the phone Continue row), `PhoneDownloads.kt`.
  - `downloads/`: offline downloads (WorkManager worker, `DownloadStore`, `LocalProgress` for
    progress played offline). The download dialog checks each file against the phone's decoders
    (`OfflinePlayback`: video codec, bit depth and size; the default audio track) and offers
    Original, Compatibility (full quality, keeping what plays: the server job's `keep_video` /
    `keep_audio`, the rest H.264 CRF 18 and stereo AAC; nothing to convert makes it an original)
    or Compressed. A download belongs to the account it was made in (`DownloadItem.account`): only
    that account sees and plays it, and the worker downloads it with that account's token (older
    ones have none, and every account sees them). Each download also saves its subtitle tracks (text as WebVTT, ASS/SSA whole as .ass with the video's fonts, PGS and DVD as .sup; the
    phone's languages, forced ones and files beside the video) in `subtitles/<id>/`, which
    `ServerSubtitles.forDownload` reads offline (a file's by its `key`, the video's own by index), its intro and credits (`segments/<id>.json`, which `segmentsFor` reads for a `file://` path; offline, Skip Credits goes on to the next downloaded episode) and its trickplay frames (`trickplay/<id>/`, from `GET /api/media/trickplay?path=`). The worker saves them again every 12 hours and when a subtitle is downloaded for the video (`DownloadStore.needsExtras`, `extrasChanged`); older downloads get them on its next run.
    A video that's downloaded, opened from the server (a folder, a title page, Continue Watching), asks
    whether to play the download or stream it (`sourceChoice` / `PhoneSourceDialog`; the iPhone's
    `SourceDialog`); going on to the next episode by itself doesn't ask, and keeps to what the last one
    played from.
    The server's front page shows the account's finished downloads from it as a Downloads row after Continue
    Watching and Recently Added, just above the libraries (`PhoneDownloadsRow`); when the server can't be reached (`errorOffline`: an
    IOException), the phone shows a banner with Retry and that row instead of the error (`PhoneOfflineScreen`).
    Download ahead (Settings › Downloads, off by default; `DownloadAhead.kt`): two minutes into an episode (or at the
    end of a shorter one) the phone asks `GET /api/media/upcoming?path=&count=` for the next episodes and queues
    those not downloaded (`DownloadItem.ahead`), as many as download in about an hour at `DownloadStore.speed`
    (measured by the worker), 3 to 5, in the quality of the show's episodes downloaded by hand (else
    Compatibility), leaving 500 MB or 5% of the phone free. Ahead downloads before the episode playing are removed;
    ones made by hand never are.
  - Photos: `PhoneImageViewer.kt` and `ImageViewer.kt` (TV) loop a motion photo's clip
    (`LivePhotoUtil`): one inside the photo (Samsung, Google), or an Apple Live Photo's .MOV. The
    server pairs those in folder listings (`media.PairLivePhotos`: a .MOV of the photo's name whose
    moov box has `com.apple.quicktime.content.identifier`), giving the photo `live_video` and
    leaving the .MOV out; on SMB shares `SmbRepository` does the same. The web plays it in the image
    viewer (`startLivePhoto`, as Compatibility (Auto) for iPhones' HEVC).
  - `ui/components/UiUtils.kt`: transcode modes, `VideoDecoders` (hardware decoder limits),
    `subtitlesOffByDefault`, `subtitleLabels`, `SubtitleMemory`.
- `iOS/`: the iPhone app (SwiftUI, iOS 17, SwiftData), a port of the Android phone app for
  sideloading with AltStore (no paid entitlements: no push, iCloud or App Groups). `iOS/README.md`
  says what's ported and what isn't. Under `iOS/WeirdStreamer/WeirdStreamer/`: `Session/AppModel.swift`
  (MediaViewModel's port), `Networking/ServerClient.swift` (the API), `Networking/HLSRelay.swift`
  (AVPlayer can't play a transcode's fragmented MP4 of unknown length: a server on 127.0.0.1 relays
  it as HLS), `Views/PlayerView.swift`, `Storage/Downloads.swift` and `DownloadExtras.swift`.
  Build for the simulator signed ad hoc (`CODE_SIGN_IDENTITY=-`): unsigned builds can't use the
  Keychain, where sign-ins are kept. Run `iOS/native/build-libass.sh` once first: it builds the ASS
  subtitle renderer (`iOS/native/assrender.c`, libass and its libraries, as Android's) into
  `iOS/native/build/` (git-ignored), which the project links (`Views/AssSubtitleView.swift`). The e2e server has an account for the simulator (see `Server/e2e/NOTES.md`).
  Unit tests: `WeirdStreamerTests` (Swift Testing; `xcodebuild test`, `iOS/README.md`), the logic
  and the JSON contract with the server; a new request body or reply model gets a test there.
  From Linux, `iOS/remote/mac-ios test|build|ipa|check` has the Mac compile and test the Linux
  checkout as it is (origin plus a patch of everything not pushed or committed), over SSH, in a
  clone of its own (`iOS/remote/ios-remote.sh`); `ipa` brings back an unsigned IPA for AltStore.
  Use it to check iPhone changes made on Linux before committing them.

## Live server

- Runs from `Server/` as `./bin/server`, under `Server/scripts/live.sh` in the user's "WeirdStreamer live"
  terminal tab. Each start logs to its own file in the folder `server.logsPath` names (the last 20 are kept). The server
  also writes `auth.log` (sign-ins, refused tokens once an hour per address, user and password
  changes) and `access.log` (a combined-format line per request, with the user and milliseconds)
  there, each moved to `.1`…`.5` at 10 MB (`internal/logs`). ffmpeg's progress isn't logged:
  transcodes log `[TRANSCODE] Started`, `Finished`, `Stopped` or `Failed` (with ffmpeg's last lines).
  Behind nginx, `server.trustedProxies` (addresses or networks) lets `X-Forwarded-For` / `X-Real-IP`
  name the client (`internal/clientip`) for the logs, sign-in limits and Streams page; from anyone
  else those headers are ignored.
  Every response carries a Content-Security-Policy (`securityHeaders` in `cmd/server/main.go`): the
  web UI runs only the server's own script files (no inline scripts or `on…=` attributes: attach
  handlers in code), WebAssembly is allowed, and only `/vendor/jassub/` may evaluate code (its worker
  needs `new Function`). Also nosniff, `Referrer-Policy: same-origin` and no framing.
  Port 9000, HTTPS. The certificate's expiry is checked at startup and daily (`internal/tlscert`):
  from 30 days before, a `[TLS] Warning` in the log and a banner in admins' Settings
  (`GET /api/admin/certificate`), an error once expired, and a note when certFile holds a newer one
  than the server is using (a restart serves it). Config is `Server/config.yml`
  (git-ignored; `config.example.yml` explains every setting). The server saves it too (Settings, Jobs,
  `cli config add-path`) through `internal/config/file.go`, which changes only the values that changed
  and keeps comments, blank lines, order and unknown keys. `cli config init <template>` writes a new
  one from JSON values with the template's comments (`create-config.sh`). Scripts read it with PyYAML.
  Which files are videos, images or documents is the config's `supportedVideoFormats` (and image and
  doc formats), through `config.MediaType` / `config.IsVideo`; a `.ts`, `.m2ts` or `.mts` file counts
  as a video only if it starts with MPEG-TS packets (`looksLikeTransportStream`, so TypeScript's `.d.ts`
  files don't), when given a path rather than a bare name; MIME types are `media.MimeType`. The
  Android app takes the server's `type` for its files and keeps its own list only for SMB shares.
  Data is in the folders `config.yml` names (databases, thumbnails, trickplay, artwork, download
  encodes). Short-lived work files (HEIC photos converted to JPEG, a trickplay check frame) go in
  `media.tempPath` (default `tmp` beside the thumbnails folder, `media.TempDir`), never `/tmp`;
  their names start `weirdstreamer-`, and startup removes only those. The server's data is
  readable by its account only: it runs with umask 077, and each start makes the databases and the
  TLS key 0600 and its data folders 0700 (`cmd/server/private.go`; never a folder holding a library).
- To deploy server code: `Server/scripts/restart-live.sh`. It checks for active streams, builds, stops
  the live server and waits for `live.sh` to start the new one (`--no-build` restarts the binary
  as it is). It reads the streams from `/local/streams`, which answers only this machine with the key
  the server writes to `local.key` beside config.yml at each start (`X-Local-Key`; 0600, so a proxy
  on this machine can't). It refuses if the server wasn't started by `live.sh`; then the user has to stop it
  and run `scripts/live.sh` in that tab.
- `cli server restart|stop [--force]` (no root needed: the server's own account) asks the server
  running with this config to restart or stop, through `POST /local/restart|stop` (loopback and
  `local.key`, like `/local/streams`; refused with the streams, exit 2, while anyone is watching
  unless `--force`). A restart shuts down cleanly and exits with status 3
  (`handlers.RestartExitCode`): systemd's `Restart=on-failure` (setup.sh's unit also has
  `SuccessExitStatus=3` and `RestartForceExitStatus=3`), launchd's `KeepAlive` and `live.sh` start it
  again; a stop exits 0, which none of them restart.
- Server tests: `cd Server/src && go vet ./internal/... ./cmd/... && go test ./internal/...`.
  Don't use `./...`: stray `main` files in `Server/src/` (`test_auth.go`, `test_login.go`) break it.
  The Go module is in `Server/src/`; binaries build into `Server/bin/` (`make build`, `scripts/build.sh`,
  `scripts/restart-live.sh`).

## Test environment

- `Server/scripts/run-e2e.sh` builds the server and runs a test server from `Server/e2e/` (git-ignored)
  on localhost port 9100; Android emulators reach the host at `10.0.2.2`. The test server's accounts,
  sample library, emulators and helper scripts are described in `Server/e2e/NOTES.md` (git-ignored):
  read it before testing. Its passwords are in `Server/e2e/pw`; never print them.
- On macOS, Homebrew's ffmpeg lacks the DVD reader, the chromaprint muxer and libass:
  `Server/scripts/build-ffmpeg.sh` builds the server's own into `Server/ffmpeg/bin` (git-ignored),
  which `run-e2e.sh` and `live.sh` put first on the PATH. `scripts/check-ffmpeg.sh` lists what's needed.
- Windows: a test VM inside this machine (KVM, Windows 11, SSH into PowerShell). How to reach it, its
  account and password are in `Server/windows.local` (git-ignored; never print the password), and
  `~/vms/windows11/scripts/windows-vm start|stop|status|ssh` drives it. Stop it when done.
- Emulators run the TV and phone apps; the tests use the e2e server only, never the live server or a
  real device. `Android/tools/start-emulators.sh` starts them (details in `Server/e2e/NOTES.md`).
  Install debug builds on emulators only.
- Typical Android test loop:
  1. `cd Android && ./gradlew :app:assembleDebug`.
  2. Install the debug APK on the emulator with `adb`, then `adb shell am force-stop` and
     `am start` for the app's main activity.
  3. Navigate and screenshot.
- Web UI: use the browser pane at http://localhost:9100 and sign in as an e2e account (the password
  is in `Server/e2e/pw`; type it yourself, don't print it). `window.api` and the app.js functions are
  available from the page's JavaScript for setting up state, such as `api.reportPlayback(path,
  seconds, true)` to create a resume point.
- To create test media, use ffmpeg test patterns. For example, `testsrc` plus `sine` with extra
  `-i subs.srt` inputs and `-metadata:s:s:N language=…` builds an MKV with many subtitle tracks.
  Put new files under `Server/e2e/sample/` (see `Server/e2e/NOTES.md` for the layout).

## Release builds

`./release.sh` (repo root, on the build machine: it has the signing key) checks the tools, runs the
server tests, then builds (the Windows server and the Mac's server and IPA in the background, `lane`, while
this machine tests and builds its own; each lane's log is shown when it's done, `wait_lanes`) `dist/<version>/`, the assets for a GitHub Release: a server package per
platform (`weirdstreamer-server-<version>-<platform>.tar.gz`, `--platforms` picks: `linux-amd64`;
`linux-arm64`, cross-compiled with `aarch64-linux-gnu-gcc` for SQLite's cgo; `darwin-arm64`, built on
the Mac in `iOS/remote/remote.local` over SSH from `Server/src` as it is here, `build_darwin`;
`windows-amd64`, built the same way on the Windows machine `Server/windows.local` describes
(`VM_HOST`, `VM_USER`, `VM_KEY`, `VM_KNOWN_HOSTS`; Go, Git and MinGW-w64 gcc there; with `VM_START` and
`VM_STOP` commands it starts a machine that doesn't answer and stops it after, only one it started), `build_windows`,
packaged as a `.zip` with `bin\server.exe`, `bin\cli.exe`, `setup.cmd` and `setup.ps1` instead of
setup.sh and check-ffmpeg.sh. Each
has `bin/server`, `bin/cli`, the web UI with this APK to download, `setup.sh`, `config.example.yml`,
LICENSE and VERSION, the Mac's also `build-ffmpeg.sh`; needs only ffmpeg where it runs), the APK, the
iPhone app's IPA (`mac-ios ipa --ref` the release commit, which must be on origin; `--no-ipa` skips it;
its version must equal Android's) with `altstore-source.json` (`iOS/altstore/make-source.py`, from the
IPA's Info.plist: AltStore and SideStore users add
`https://github.com/vargonsoft/WeirdStreamer/releases/latest/download/altstore-source.json`) and
`SHA256SUMS`. The server and cli put `ffmpeg/bin` beside their `bin` folder first on the PATH, and on
macOS add Homebrew's folders (`media.UseBundledTools`): a package's own ffmpeg, Server/ffmpeg/bin in
the repo. It also publishes the APK to the web download links (`--no-web`
doesn't). The version is the app's (`0.0.1.<commit count>`). It refuses uncommitted changes
(`--allow-dirty` builds them as `<version>-dirty`) and installs on no device. Artifacts stay out of git.
`--publish` then tags the commit `v<version>`, pushes the tag to the `github` remote and creates the
GitHub Release with the files (`gh`, signed in; checked before building; it never replaces a
release, nor moves a tag that's on another commit).

The package's `setup.sh` (`Server/package/setup.sh`) asks only for the name, port and HTTPS and
writes config.yml through `cli config init`, can create the admin user and install a systemd service
(Windows: `setup.ps1`, see More server platforms) (`setup.sh service`, `setup.sh remove-service`: `bin/server` as the user who ran setup, from the
package's folder, after network mounts), then prints the web UI's address and offers to start the
server and open it. On macOS (`mac=1`) it offers to build ffmpeg into the package
(`FFMPEG_PREFIX=<package>/ffmpeg ./build-ffmpeg.sh`) when there's none with what check-ffmpeg.sh
asks for, gives Homebrew install hints, and installs a launchd daemon
(`/Library/LaunchDaemons/com.weirdstreamer.server.plist`: the user, the package folder, started at
boot and again after an error, output in `var/server.out`) instead of systemd; it says a library in
Desktop, Documents, Downloads or on removable or network volumes needs Full Disk Access for
`bin/server`. Its GPU step (`gpu_setup`; alone as `setup.sh gpu [--check]`) finds graphics by PCI
vendor (no driver needed) and lists what hardware encoding needs: Intel and AMD the `video` and
`render` groups and VA-API drivers (Intel also `libmfx-gen1.2` for Quick Sync), NVIDIA a loaded
driver (or `linux-modules-nvidia-<branch>-<kernel>` when the module wasn't built for the running
kernel) and `libnvidia-encode`; on apt systems it installs them and adds the groups after asking.
AMD's are assumed, untested on a card; a missing ffmpeg is reported and installed with them.
Before it, `packages_setup` (apt systems) offers ffmpeg (required) and then, in one question, what's
missing of libheif-examples, fonttools and fonts-noto-cjk; elsewhere it prints notes. `apt_install`
updates the package lists and tries once more when an install fails.
`PCI_DEVICES` points it at a fake device folder for testing. Its Guide step (`guide_setup`; alone as
`setup.sh guide [--check]`, after the admin user in a full setup; Windows `setup.ps1 guide [-Check]`,
`Step-Guide`) offers the Guide's voice-overs and music as two questions, from `guide-extras.tsv` (shipped
beside setup: each voice's and track's address and SHA-256; a file that doesn't match is thrown away):
Piper (`piper-tts`, pinned `piper_version`) in its own Python environment in `var/piper` (Python 3.9+ with
venv: apt's `python3-venv` offered; on Windows a real Python found by its `sys.executable`, else winget's
Python 3.12, and the Visual C++ runtime onnxruntime needs, `Microsoft.VCRedist.2015+.x64`), Ryan's and HFC Female's voices (pinned to a piper-voices commit) in `var/piper-voices`, and the
20 tracks with their credit lines in `var/guide-music`; then `cli config set-piper` and `set-guide-music`
write config.yml. Run alone, it restarts a service of this folder's server (asking while someone watches).
`setup.sh upgrade [--check]` installs the latest
GitHub release over the package (`release_repo`, `WEIRDSTREAMER_REPO`): the tag from where `/releases/latest`
redirects, this platform's tarball checked against SHA256SUMS, unpacked in `.upgrade/` and moved into place
by renames (a running program, setup.sh included, keeps its old file), what it replaced kept in `.previous/`;
config.yml, `var/`, `ffmpeg/` and the service stay. A service of this folder is restarted, asking first while
`/local/streams` (with local.key) says someone is watching. Libraries and the TMDB token are set in Settings: while there are no libraries,
an admin's sign-in opens Settings at Media Paths (`openLibrarySettings` in `app.js`). Its Browse… button picks a library's folder on the server (`openFolderPicker`, from `GET /api/config/folders?path=`, admins only: folders without hidden ones, and how many videos are in it).

## Deploying the Android app

`Android/deploy-release.sh` builds the R8 release APK and installs it only on the destinations
given: `--emulators` (the running emulators, with a launch-and-crash check; emulators go first and
a crash stops the rest), one flag per real device in `Android/devices.local` (git-ignored, one line
each: `flag|display name|model prefix`; `--help` lists them), and `--web`: copies it to `Server/web/downloads/WeirdStreamer.apk`
(git-ignored), which the web UI's Settings (My Account) links to for downloading (the sign-in page
links to GitHub's latest release instead, `.app-download-github`);
the link hides when there's no APK. `/downloads/` is for signed-in accounts only (a 20 MB file anyone could
fetch over and over), so the apps' `AppUpdater` asks each server with its token (`updateServers`: only
saved servers with an account signed in). When none offers it (or none is signed in), it takes GitHub's
latest release instead (`latestOnGitHub`: the API's `releases/latest`, the tag's last part as the build, the
release's `.apk`), installed the same way; only if neither answers does Settings' update row open GitHub's
release page (`RELEASES_PAGE`; a TV without a browser shows the address). With none it builds nothing. `--no-build` reuses the built APK. A
destination that isn't connected, or a failed install, makes it exit non-zero.

- Versions: `versionCode` (the build number) is the git commit count, and `versionName` is a
  hand-set version plus the build number (`0.0.1.254`), in `app/build.gradle.kts`. `--web` also
  writes `WeirdStreamer.json` (version, build, size, date), which the download links show.
- Real devices must get release builds. Debug builds are visibly sluggish on the TV box.
- R8 needs keep rules (`Android/app/proguard-rules.pro`) for anything Gson stores by field name
  or created by reflection. Missing ones once crashed release builds at startup (WorkManager's
  Room database) and would have scrambled the saved downloads list.

## How things work (where it matters for the TODOs)

- **Client paths** (`handlers/access.go`): every response gives a file's `path` as
  `<library ID>/<path below the library>`, as folders are browsed (`clientPath`, applied last, by
  `clientItems`, `clientFiles`, `clientContinue`, `clientItem`, `clientJob`: handlers work with disk
  paths until then). Every request's `path` is that in base64, turned back by `diskPath`
  (`decodeMediaPath` / `filePath`, which also check the user's group), which refuses anything else:
  a disk path, or one that climbs out of its library. Text made for people (ffmpeg's arguments on
  the Streams page, a download's error) goes through `withoutDiskPath`. A new endpoint must do the
  same. Cache names (thumbnails, trickplay, ASS tracks, downloaded subtitles, subtitle keys) are
  `config.CacheKey`: the SHA-256 of the library's ID and the path inside it, so moving a library's
  folder keeps them. Clients treat `path` as a name to hand back, and store it
  (downloads' `sourcePath`, offline progress, remembered track choices).
  Stored user data is keyed by client path too, never a disk path (`config/clientpath.go`:
  `ClientPath` / `DiskPath`; `database/keys.go` refuses a disk path): `playback.path`,
  `segments.path`, `dvd_discs.path`, `subtitle_offsets.path`, `metadata_folders.folder_path`,
  `episode_overrides.file_path`. Only the scanner's own records keep disk paths: `media_items` and
  `media_added`, which describe the files on disk. A library's folder can move without touching the
  stored data. A file outside every library has no client path, so nothing is stored for it.
- **Streaming:** `GET /api/media/stream?path=<base64 client path>`. Query options:
  - `transcode=1` forces an ffmpeg transcode: libx264 ultrafast, AAC 192k, fragmented MP4,
    8-bit 4:2:0.
  - `audiocodec=` picks the audio of a transcode or a copy: `copy` keeps it (AAC, AC-3, E-AC-3,
    MP3, Opus, FLAC and DTS travel in the stream; TrueHD is only experimental in MP4), `ac3`,
    `eac3` and `aac` convert it to 5.1, and `aac` with `channels=2` to stereo.
  - `start=<s>`, `bitrate=` (for example `80%`) and `resolution=` (a height, never upscaled)
    also apply.
  - `auto-transcode=0` turns off the server's extension-based auto-transcode (`needsTranscoding`
    in `streamer.go`: `.mkv`, `.avi`, `.wmv` and similar). The web UI always sends it, so the
    browser gets originals unless the user presses "Transcode to MP4".
  - Discs (`.iso`) are always transcoded, per title (`title`, `angle`, `audio` params); their
    subtitles come beside the stream (below). An unpacked DVD, a `VIDEO_TS` folder holding
    `VIDEO_TS.IFO` (`media.DVDFolder`; `config.IsDVDFolder` by name, which `config.MediaType` takes for a
    video and `media.IsDiscImage` for a disc), is one too: the scanner and folder listings show it as one
    video named after its parent folder (`DiscName`), sized as its files (`DiscSize`), and never go into
    it; its IFOs are read from the folder (`folderDisc`), ffmpeg's dvdvideo opens the folder, and the
    clients take a path ending in `VIDEO_TS` for a disc as they do `.iso`. No VOB/IFO extensions in
    config.yml: the folder is found by its structure.
- **Web player** (`app.js`): `playVideo(item, forceTranscode, startAt)` plays in the
  `<video id="video-player">` of `#player-modal`, with the player's own controls
  (`#player-controls`, `setupPlayerControls`; the native `controls` are off). Discs (`playDisc`)
  use the same controls plus title, chapter and angle.
  - Before playing, it reads `GET /api/media/codecs` (container, duration, video codec, audio and
    subtitle tracks). The Transcode menu has Android's modes (`TRANSCODE_MODES`): Original (the
    file as it is), Compatibility (Auto) (`transcode=audio`: the server copies browser-friendly
    H.264, else transcodes; the audio is kept if the browser plays it, `audioSteps`),
    Compatibility (Safe) (H.264 and stereo AAC, always), and the quality and size ones (full
    transcodes). The player picks Compatibility (Auto) itself (`chooseCompatibility`) for audio
    the browser can't decode, a non-default audio track or picture subtitles, and after a
    `<video>` error on Original; a copied video that errors gets a full transcode. Original
    picked by hand plays the file untouched, even if it's silent.
  - Video and container are checked before playing too (`originalProblem`): the codecs report
    becomes an RFC 6381 codec string (`videoCodecString`: `avc1…`, `hvc1…`, `av01…`, `vp09…`)
    for `MediaSource.isTypeSupported` / `canPlayType`, codecs no browser decodes (MPEG-4 Part 2,
    MPEG-2, VC-1…) are known, and so are containers browsers don't open (MPEG-TS, AVI, WMV…),
    whose H.264 Compatibility (Auto) copies into MP4.
  - A copied video starts at the keyframe ffmpeg's seek lands on (`-noaccurate_seek`, so the audio
    starts there too instead of after seconds of silence). `GET /api/media/seekpoint` (ffprobe's
    seek, which lands on the same keyframe) tells the player that start, for its offset. A jump
    forward that keyframe would undo (keyframes far apart, a short jump) asks `next=1` instead: the
    next keyframe and the `ask` start that lands on it (ffmpeg's seek goes by decode times, so it
    needs a bit past it). TV and phone `StreamChoice.jump`, web `jumpStart`.
  - A transcoded stream restarts at an offset when seeking past what has arrived (`seekTo`,
    `player.offset`, `disc.offset`), as on Android, so the player tracks the absolute position
    itself (`playerPosition`). Seeks within the buffer don't restart it.
  - Text subtitles come from `GET /api/media/subtitles` (WebVTT, 2-minute windows, fetched ahead
    by `keepSubtitlesAhead`) as cues on a `TextTrack`. PGS comes the same way with `format=sup`
    (raw .sup data, which also carries when each line clears): `js/pgs.js` decodes it and a canvas
    draws it over the video, as `PgsSubtitles.kt` and `SubtitleOverlay` do on Android, so it needs
    no transcode. Pictures decode only when shown (a film's worth would take hundreds of MB). PGS
    draws for the disc's 16:9 frame: it's centered on the video at the video's scale, not
    stretched to a cropped video. A file's DVD subtitles (`dvd_subtitle`, VobSub) come the same way: for
    `format=sup` the server decodes their subpictures and writes them as PGS (`media/dvdsub.go`,
    `DVDSubtitlesAsPGS`: packets from ffprobe, the palette and canvas from the track's .idx header,
    pictures widened to square pixels by the video's sample aspect, a clearing set at each end), so
    every player draws them with its PGS code and downloads save them as .sup. Only the rare other
    picture formats (DVB, XSUB) are burned in (`subtitle=N`). `forced=1` gives only a DVD track's
    lines marked forced: with no track chosen, every player loads those of the DVD track in the
    playing audio's language (files' and discs', `forcedLinesTrack` on the web, `forcedTrack` in
    the apps' ServerSubtitles).
  - A disc image's subtitles come the same way (`media/dvddiscsub.go`; `format=sup` with `title=`,
    `angle=` and `from=`, where the player's stream started), drawn over the transcode, so choosing
    them doesn't restart it. The demuxer lands a seek near its target and labels what follows as the
    target (the stream started at 3000 s may show 3002 s as 3000 s, up to 15 s off), and the labels
    depend only on the target: so a reader per stream start (`discReaderFor`) seeks where the stream
    did and reads on, every track at once, about 160 times faster than playback, and the windows
    wait for it. A title of chained program chains or read from a later chapter isn't sought (the
    transcode trims), so its reader reads from the start in the title's own time. The palette is the
    demuxer's `.idx` extradata; the canvas and pixel shape the title's. An open interval with
    `-show_data` (`3000%`) makes ffprobe read nothing, so it's given an end.
  - ASS/SSA tracks and files (codec `ass` or `ssa`) are drawn styled by libass: JASSUB 1.8.8, vendored in
    `Server/web/vendor/jassub` (loaded on first use). `format=ass` gives each 2-minute window as a
    whole script (header, styles and its Dialogue lines, at the file's times); `loadAssWindow` keeps
    the first header, adds each window's lines once and hands libass the script so far. The
    video's embedded fonts (the codecs report's `fonts`) come from `GET /api/media/font?path=&font=N`,
    fetched through the API (JASSUB's own fetches wouldn't be signed in). JASSUB's time is the
    video's plus `timeOffset` (the stream's offset less the track's timing correction), and its
    canvas sits in `player-ass-box` over the picture; while paused, `redrawAss` draws changes.
  - **CJK subtitle fonts** (`media/cjkfont.go`, `POST /api/media/subtitle-font` with `{"text"}`):
    libass has no font for Chinese, Japanese or Korean characters in the web player or on Android,
    and a complete CJK font is 20 MB or more. So the server makes a subset of
    `media.cjkFontPath` (default: Noto Sans CJK where installed) with `media.subsetCommand`
    (fontTools' `pyftsubset`) holding the text's CJK characters plus basic and Latin-1 Latin, cached
    in `<subtitlePath>/cjk` per character set (OpenType, a few KB). Its family is "Noto Sans CJK JP",
    and the players use that as the renderer's default font, since libass finds a glyph's font by
    the family it asks for (no font provider): JASSUB's `fallbackFont` (`CJK_FAMILY` in `app.js`) and
    Android's `ass_set_fonts` default family (`assrender.c`). The web and Android fetch a subset
    with each ASS track (`addCjkFonts`, Android's `ServerSubtitles.fetchCjkFont`). Downloads save it as
    `font-cjk.ttf`. The iPhone fetches the same subsets (`ServerSubtitles.fetchCjkFont`,
    `ServerClient.subtitleFont`, default family in `iOS/native/assrender.c`) and saves them with its
    downloads, tested on the simulator only. Without a font or
    the subsetter the endpoint answers 503 and those lines show as boxes. Set `cjkFontPath` and
    `subsetCommand` in config.example.yml; the checks are in `scripts/check-ffmpeg.sh`.
  - **ASS cache** (`media/asscache.go`): a window read from the video can't see a line that started
    before it (a long sign on screen when playback starts or jumps). The "Styled subtitles" job
    (after each scan and every 6 hours, like trickplay, waiting while the file's disk is busy)
    probes every library video that changed since it last looked and copies each ASS/SSA track
    out whole (one ffmpeg read per video) into `<subtitlePath>/ass/<hash[:2]>/<hash>-<track>.ass`,
    with `index.json` keyed by path hash (size and mtime, its ASS tracks). `format=ass` windows
    are then cut from the copy (`AssWindow`: the header and every line on screen in the window);
    until a video's turn they're read from the video as before. `.ass`/`.ssa` files beside a video
    are read whole and cut the same way. `duration=0` gives the whole track (downloads).
    Track choices are kept per video in `localStorage`. Subtitles are off unless
    the file has forced ones, as on Android.
  - The seek bar shows trickplay frames (made by `cmd/cli trickplay`) on hover.
  - `startPlaybackReports` sends progress to `POST /api/streams/progress`. Resume and
    "Play from start" go through `playVideo`'s `startAt`.
  - Icons come from `MATERIAL_ICONS`, `iconSvg(name)` and `decorateIconButtons`.
- **Hardware encoding** (`media/hardware.go`): at startup (`DetectHardware`, in the background)
  the server lists GPUs (DRM render nodes' PCI vendor and lspci name; `nvidia-smi` for NVIDIA's
  names) and tries a short test encode with each fitting encoder ffmpeg has: NVENC for NVIDIA,
  Quick Sync and VA-API for Intel, VA-API for AMD (each render node), and on macOS VideoToolbox
  (no device; the GPU is named from `sysctl machdep.cpu.brand_string`). `GET /api/admin/hardware`
  reports the GPUs, the encoders that worked and why others didn't. Only render nodes with a device file
  in `/dev/dri` count (a container sees the host's `/sys` but only the nodes passed into it), and an
  SR-IOV virtual function (a `physfn` link: an Intel GPU shared with VMs) only when its GPU isn't
  there itself (`usableRenderNodes`): a host lists the GPU once, a container or VM given a virtual
  function keeps it. An ffmpeg that doesn't run is reported as that, not as missing
  encoders. The startup log has a
  `[HARDWARE]` line per GPU (`gpuName`; its kernel driver, NVIDIA's driver version and memory, a
  Mac's shared memory, the render node), ffmpeg's version, each encoder that works or doesn't and
  why, and the setting in effect (`HardwareSetting`: the encoder and its limit, off though one works,
  or chosen but not working here); an admin's change in Settings logs it again. Settings (web Server › Streaming,
  phone and iPhone Server tab, TV Settings › Hardware encoding, admins only) shows that and, when
  something works, the choice (`streaming.hardwareEncoder` nvenc/qsv/vaapi, `hardwareDevice` the
  render node) and `hardwareMaxTranscodes` (0 no limit, streams and downloads together). At startup
  NVENC's sessions are counted (`countSessions`: 32 short encodes at once, the driver refuses those
  past its limit; GeForce driver 580 allowed 8, 615 allowed 12; Intel's had no limit at 16), and the
  limit used is never more than that (`hardwareLimit`); the encoder's `sessions` is in the report,
  and Settings' note names it. Streaming
  transcodes (files and discs) and download encodes use it: `hardwareArgs` turns the CPU's ffmpeg
  arguments into the encoder's (VA-API uploads frames with `format=nv12,hwupload` after the CPU's
  filters), so decoding, scaling, burned-in subtitles and 10-bit to 8-bit stay on the CPU. x264's
  CRF becomes `-cq`/`-global_quality`/`-qp` plus `qualityOffset` (NVENC +5, Quick Sync +1, VA-API
  +2: the same size and look as x264 veryfast; 1:1 made files 1.3 to 2 times the size), and
  VideoToolbox's `-q:v` (`videoToolboxQuality`: 75 for CRF 18, 2.5 per CRF step; Apple Silicon only,
  Intel Macs fail the test encode). In the
  transcode cache (`attach`) a new transcode goes to the GPU while under the limit; its spool keeps
  the CPU arguments' name and runs them instead if ffmpeg fails before making anything (then the GPU
  is skipped for 5 minutes, `hardwareFailed`). The Streams page names the encoder used
  (`SetVideoEncoder`). A download (`DownloadManager.encode`) takes the GPU while there's room
  (`hardwareForDownload`, counted in `downloadsOnGPU`), else x264; one the GPU fails is encoded again
  by the CPU. The log's `Ready` line names the encoder.
  Measured in 2026-11 on Tears of Steel 1080p (GTX 1060 and RTX 4070 Ti SUPER on i7-14700Ks,
  Meteor Lake through a VM's passthrough): at the same size every GPU matched x264 veryfast, by
  SSIM and by eye, in both download modes; the slower presets (NVENC p7, two-pass, x264 medium)
  gained nothing worth their time. On the 14700K the GPU is no faster than x264 but uses a ninth of
  the CPU; on a Core Ultra 5 125U it's 2.7 times faster at a seventh of the CPU. Quick Sync needs
  Intel's GPU runtime (Ubuntu's `libmfx-gen1.2`), and the server's account the `video` and
  `render` groups. VideoToolbox on an M1 is weaker per bit: about 20% bigger at full quality and
  0.9 dB lower SSIM at a Compressed size, though it looked the same by eye; 1.2 times x264's speed
  at a tenth of the CPU. Detection and both download modes were tried on the M1 (not as a running
  server: there's no Mac package yet). Streaming on Quick Sync and VA-API is still untested on real hardware.
- **Resumable transcodes** (`media/transcache.go`): browsers drop and reopen media connections
  with `Range: bytes=N-`, and ffmpeg can't seek its output, so every transcode goes through
  64 MB chunks in `media.transcodeCachePath` (default `transcodes` beside the thumbnails folder).
  A reconnect joins the same ffmpeg and is served from the chunks (206 with an unknown total).
  - Each viewer (user, client and file, the stream's key) has their own transcode and chunks.
  - Chunk names are a hash of the viewer, path and ffmpeg's arguments, never the video's name.
  - ffmpeg runs at most 96 MB ahead of the furthest read, and 512 MB is kept behind it.
  - With no client for 60 seconds a transcode is stopped and its chunks removed (`reapIdle`, its
    own timer). Startup clears the folder.
- **Unreadable files:** the stream and codecs endpoints first open and read the file
  (`media.CheckReadable`). If they can't (gone: 404; refused by the disk or share, or an I/O
  error: 503), they log `[STREAM] Can't read …` and answer with the reason and
  `X-Stream-Error: unreadable`. Players then skip every fallback and show the reason under "Can't
  play this video" (`unreadableReason` on Android; the web checks codecs again on a `<video>`
  error). A read that fails partway through an original logs `Stopped serving …`.
- **Without ffmpeg:** background jobs (trickplay, intros and credits, styled subtitles, damaged
  videos) wait while ffmpeg or ffprobe isn't on the PATH (`ffmpegMissing`, checked in `busyFor`; the
  Jobs page says which), and store nothing about a file read then; the scanner leaves its format
  unread for the next scan. `backgroundCommand` runs a missing tool without nice, so the error is
  exec.ErrNotFound and not nice's exit status 127, which read like the file being turned down (a
  server started before ffmpeg was installed once marked every video done without frames).
- **When jobs wait for the disks** (`media/load.go`, `disks.go`): above 50% busy on the disk someone is
  playing from, 90% on one nobody is (these and the shares' 60 and 500 ms are config.yml's `jobs.waitDiskPercent`,
  `waitDiskIdlePercent`, `waitShareMs`, `waitShareIdleMs`, 0 the default, `config.CurrentWaitLimits`; the Jobs page's
  "When jobs wait" fields on web, phone, TV and iPhone, `POST /api/admin/jobs/limits`, used at once; a job already
  reading stops further up, `stopDisk` and `stopShareMs`); a file whose disk can't be told counts the busiest disk at 50%.
  Libraries on network shares (SMB, NFS) and FUSE mounts (a mergerfs pool, sshfs, rclone: no block
  device, and a container's `/proc/diskstats` lists the host's disks) are judged by timing 64 KB reads
  from a big file in each instead (`isTimedFS`, `IsTimedMount`; above 60 ms where someone is playing,
  500 ms otherwise), shown as Read times on the Jobs page. On Windows the readings come from Windows'
  own counters (see "Windows x64" under More server platforms).
- **Trickplay on the GPU** (`media/trickplaygpu.go`): a trickplay frame costs the CPU a decode of every
  frame from the keyframe before its time (up to a keyframe interval: on a 4K film about 1.6 CPU-seconds a
  frame here, 7 on a Core Ultra 5). With a hardware encoder chosen in Settings (`hardwareChoice`), batches
  decode on that GPU instead: VA-API for Quick Sync and VA-API (the render node), CUDA for NVENC;
  VideoToolbox isn't tried. One device for the batch (`-init_hw_device`, a context per input was slower than
  the CPU), frames scaled to 320 px on the GPU (`scale_vaapi`, `scale_cuda`) and only those copied back
  (copying every decoded 4K frame back cost more than the CPU's decode). Measured: a tenth of the CPU time,
  the same pictures (only the scaler differs, a little sharper). A GPU batch that makes nothing is tried
  with one frame: if that works the GPU ran out of memory (a 6 GB GTX 1060 took 10 4K decoders, not 15), so
  batches keep to half the pixels from then on (`tooBig`, per GPU, logged once); if not, the GPU can't decode
  that video and its batches go back to the CPU (three such videos in a row rest the GPU for 10 minutes).
  `cli trickplay` waits for the GPU check first (`WaitForHardware`). The Jobs page can keep trickplay on the CPU
  anyway ("Decode on the GPU", shown when there's a GPU to decode on; `jobs.trickplayCPUOnly`,
  `config.TrickplayCPUOnly`; the jobs reply's `gpu` names it and `use_gpu` says whether it's used; saved at once,
  `{"use_gpu"}`, apart from the fields): measured on babylon5 (i7-14700K, 28 threads, GTX 1060) with a 31 Mb/s 4K
  HEVC 10-bit clip, keyframes 10-20 s apart, the CPU at 2 × 30 made 2.0 frames/s (13.7 GB), at 1 × 30 1.7 (7 GB), and the GPU 1.2-1.3
  (1 × 6 or 2 × 3, a few hundred MB: its decoder was the limit, and the desktop's 2.3 GB of the card left room for
  only 6 4K decoders). The CPU's decodes skip the deblocking filters
  (`-skip_loop_filter all`, `skipLoopFilter`): 3-9% faster, the same memory, nothing to see at 320 px. Keyframes-only decoding (`-skip_frame
  nokey`) was 11-15 times faster but shows the keyframe before each time, up to ~10 s early: not used.
- **Trickplay memory** (`media/memory.go`): each frame of a trickplay batch is an ffmpeg input with a
  decoder of its own, so a batch's memory grows with the picture: about 60 MB a process plus 228 MB a
  frame for 4K HEVC 10-bit, 84 MB for 1080p HEVC 10-bit, 45 MB for 1080p H.264 (measured, ffmpeg 8).
  On the GPU it's more: VA-API (Intel, measured on a Core Ultra 5) about 40 MB plus 370 MB a 4K frame,
  its decoders' pictures taken from the server's memory (not in ffmpeg's RSS, only its cgroup's); CUDA
  (a GTX 1060) 175 MB plus 124 MB of the server's memory and 60 MB plus 540 MB of the card's. The jobs
  reply's `trickplay.memory` carries the figures for the decoder in use (`decoder`, and `card_…` for CUDA).
  Frames per runner are set by the video's size (`config.TrickplayTiers`, `media.TrickplayTier` from
  `resolutionLabel`): 4K (1440p and up, and unknown sizes) in config.yml's `trickplayFramesPerRunner`, 1 to 50 (100 at
  once), then `trickplayFrames1080p` 1 to 100 (200), `trickplayFrames720p` 1 to 150 (300), `trickplayFramesSD` 1 to 200
  (400); each runner's batch takes its own video's tier. A tier's memory per frame is 4K's times `TrickplayTierScale`
  (1080p 0.37, measured on the CPU with HEVC 10-bit; 720p 0.18, SD 0.09 by their pixels with room to spare).
  The defaults (config.yml's 0, an empty field on the Jobs page; `media.TrickplayRunners`,
  `TrickplayFramesPerRunner(tier)`) are automatic: a runner per disk the libraries are on (`LibraryDisks`: whole
  disks, each network or FUSE mount, Windows drives), and as many frames as fill half the server's memory
  with videos of that size for those runners (and fit in 90% of an NVIDIA card's own memory, which is trickplay's to
  use) (`AutoTrickplayFrames`, within the tier's limits). The jobs reply gives what's in effect, `auto_runners`, and
  per tier (`tiers`: `frames`, `auto`, `max_frames`, `max_product`, `scale`, `auto_frames_for[r-1]`) for the fields'
  placeholders and the note; saving sends `frames` by tier ID (`4k`, `1080p`, `720p`, `sd`).
  Numbers set are used as they are (the user sizes them, swap included); the Jobs page's trickplay
  table shows what they take with each size against the server's memory and swap, and an NVIDIA card's (a
  row per size and per size on the card: runners × frames, needs, has, fits or not; the jobs reply's `trickplay.memory`:
  `SystemMemory`, MemTotal or the cgroup's limit when lower, `hw.memsize` on macOS; `SystemSwap`; web `trickplayMemoryTable`, Android
  `trickplayMemoryTable` in `AdminSettings.kt` drawn by `TrickplayMemoryTable.kt`, iPhone in `ServerModels.swift`). A batch the kernel kills is made again a frame at a time, and setup.sh's
  systemd unit has `OOMPolicy=continue`: by default an ffmpeg killed for memory made systemd stop and
  restart the whole server.
- **Streams with track options:** file transcodes take `audio=N` (index among the audio tracks)
  and `subtitle=N` (burn in that subtitle track, scaled to the video), in `TrackOptions`
  (`media/codecs.go`).
- **TV player** (`VideoPlayer.kt`, ExoPlayer):
  - Transcode modes come from `transcodeModes` in `UiUtils.kt`, picked in a D-pad dialog in
    `PlayerDialogs.kt`. `transcodeQuery()` caps the height at what the hardware decodes.
  - Any playback error on the original switches to Compatibility (Auto), and so does a track
    with no decoder (no error, just silence), unless Original was picked by hand.
    `StreamChoice.kt` decides the video and the audio apart, as the web does:
    - Video: copied if it's 8-bit 4:2:0 H.264 (`transcode=audio`; `/api/media/seekpoint` gives the
      keyframe it really starts at), unless subtitles are burned in or it's too big to decode.
    - Audio (`audioLadder`): the track as it is if the device plays it (`audiocodec=copy`), else
      the best 5.1 its output takes (AC-3, E-AC-3, AAC, `AudioOutput`), else stereo AAC.
    - What failed steps down: a copied video gets a transcode, audio the next on its ladder, an
      error that doesn't say which does the video first. At the end the player says it can't
      play the video.
    - Compatibility (Safe) keeps nothing: H.264 and stereo AAC, whatever the device says.
  - ASS/SSA subtitles (TV and phone) are drawn by libass: `AssRenderer.kt` over `libassrender.so`
    (`app/src/main/cpp/assrender.c`, JNI). `ServerSubtitles` loads `format=ass` windows into one
    script (`assScript`, `assVersion`) and the video's fonts (`videoFonts`), and `AssSubtitle` draws
    it: each frame the position is checked, libass draws on its own thread into the back of two
    bitmaps, shown when the subtitles changed. It sits in the video's own box with the video's
    `graphicsLayer` transform (`videoTransform`: zoom, pan, fill, aspect, rotation), so the lines
    stay on the picture; `SubtitleOverlay(drawsStyled = false)` leaves it to the player. Fonts the
    script names that aren't the video's fall back to the system's Roboto. Downloads save ASS tracks
    whole (`<n>.ass`, `format=ass&duration=0`) with the video's fonts (`font-<index>-<name>`), and
    play them styled offline; one saved before as WebVTT shows plain lines until the worker saves it
    again.
  - Native build (needs the NDK r29, cmake, make, pkg-config, curl; `release.sh` checks them): `Android/native/build-libass.sh` builds libass 0.17.5, HarfBuzz 15.0.0, FriBiDi
    1.0.17 and FreeType 2.15.0 (pinned, checksummed sources) as static libraries once, and
    `libassrender.so` for arm64-v8a, armeabi-v7a, x86_64 and x86 (about 1.7 MB each) into
    `Android/native/build/jniLibs` (git-ignored), which the app packages. Gradle runs it before every
    build (`buildAssRenderer`). It needs the Android NDK r29 (`~/Android/Sdk/ndk/29.0.14206865`, or
    `ANDROID_NDK_HOME`), cmake, make and pkg-config, on Linux x86-64.
  - Subtitles: `onTracksChanged` collects `C.TRACK_TYPE_TEXT` formats and labels them
    (`subtitleLabels`). `selectSubtitleTrack(index)` sets a `TrackSelectionOverride`, and
    `SubtitleMemory` remembers the choice per video.
  - Audio tracks (files from the server with more than one; a disc's are in its dialog): the
    toolbar's Audio button (id 9, after Subtitles) opens `AudioDialog`, the server's tracks named
    by `audioLabels`. The choice is `StreamChoice.audioTrack` (the server's index, -1 the default),
    kept per video in `AudioMemory` (`TrackMemory`, like `SubtitleMemory`). The original plays a
    track the device decodes itself (`selectAudioTrack`: a `TrackSelectionOverride` on the
    player's audio groups, which are in the server's order; if their number differs, the server
    plays it); one it can't decode switches to Compatibility (Auto). Server streams carry
    `audio=N` (`trackQuery`), and the audio ladder works from that track.
- **Phone player** (`PhoneVideoPlayer.kt`, ExoPlayer):
  - Same transcode menu (a dropdown) and fallback to Compatibility (Auto) as the TV.
  - Touch (`PhoneGestures.kt`, `phonePlayerGestures`, one detector on the root `Box`, then
    `clickable` to show or hide the controls): two fingers pinch-zoom and pan; one finger pans a
    zoomed picture; on an unzoomed one a sideways swipe seeks (the screen's width is 90 s, a
    "+0:30 / 12:34" indicator, applied on release through `seekPlayer`), and up or down sets the
    brightness on the left half (`window.attributes.screenBrightness`, given back to the system
    when the player closes) or the media volume on the right (`AudioManager`), with a level
    indicator. Drags the seek bar or trickplay row take are left to them.
  - It has a subtitle menu like the TV's (tracks, online search, timing) and an Audio menu (files
    with more than one track) that works as the TV's.
  - Picture-in-picture (`PhonePip.kt`): going home while a video plays shrinks it into a window
    (auto-enter from Android 12, `onUserLeaveHint` in `MainActivity` before), with a play/pause
    button; the player hides its controls there (`viewModel.inPictureInPicture`). Closing the
    window ends the viewing like Back. The manifest's `configChanges` keep the activity (and the
    player) alive through the window's size changes.
- **Themes:** Default is Ember (the logo's aubergine, gold and cream) on web and Android, and the
  two should match: a dark background (#1C0F1B) with aubergine boxes (#321B2F) for the header and
  content, plum buttons (#6A3A62) with cream labels, gold (#F6C445) with aubergine text for a
  focused (TV) or hovered (web) button, and gold headings and progress lines (with the watched
  check). Progress has its own color in each theme: `--progress-color` / `--progress-label` on
  the web, `progressColor` / `progressLabel` in the Android theme spec. Web: `:root[data-theme="default"]` in
  `style.css` (`--button-text`, `--button-focus`, `--heading-color`…; other themes fall back to
  their old colors). Android: every theme is a `ThemePalette` (`Color.kt`) built from its
  background (boxes, buttons, text and one accent for focus, headings and icons; `panelAlpha` makes
  Cinema Blue's boxes see-through over its film reel). The TV and phone color schemes and the
  theme spec come from it; `Panel`, `tvButtonColors()` and `phoneButtonColors()` in
  `ThemedControls.kt` draw with it. Quieter buttons (Cancel, Keep, Close, Back) are secondary: each theme's `--button-secondary-bg` / `-text` on the web (`.btn-secondary`), and `ThemePalette.buttonSecondary` / `buttonSecondaryLabel` on Android (worked out from the box and button colors), outlined in the button color: `TvSecondaryButton` on the TV, `PhoneSecondaryButton` beside `PhoneButton` (the filled main action; `destructive` for deleting or removing) in phone dialogs, and the phone's tonal buttons. On the phone `surface` is transparent, so lists and bars show
  their box. Both also have Teal, Orange, Brown, Purple, Gray, Yellow and Green, and six of three colors
  (Harbor Lights, Moss and Rose, Desert Bloom, Arctic Berry, Copper Sea, Neon Arcade), and
  Terminal Green and Terminal Amber (phosphor on black, all text monospace: `ThemePalette.monospace`), dark themes built like
  Ember with the same colors on each (a `ThemePalette` and a `:root[data-theme=…]` block). Android also has
  Light and Auto Light/Dark (`AppTheme`). Themes are picked from a grid of tiles drawn in each theme's colors (its background, box, accent and button), applied and kept at once: the web's Settings, Themes tab (`renderThemePicker`, colors read from the theme's CSS block), and the Themes tab of the TV and phone Settings (`ThemeTiles.kt`: `TvThemePicker`, 3 rows of 7, and `PhoneThemePicker`). The TV's Settings has tabs under its title (`SettingsScreen.kt`, rows drawn by `TvSettingsRow`): General (this TV's options and the app update), Account (with a server open: Languages, Change password, Sign in on a TV), Sources (saved servers and shares), Themes, and for an admin's account Server, Users, Streams and Jobs, as the phone's and the web's. Server administration's state and requests are shared (`AdminSettings.kt`: `ServerSettingsModel`, `UsersModel`, `PasswordChangeModel`, `StreamsModel`, `JobsModel`, each with a `remember…`), and each draws its own screens on them: the phone in `PhoneServerSettings.kt` (a Save button), the TV in `TvAdminSettings.kt` (rows that open dialogs; each change saves at once, `saveSoon` for Left/Right steps, and the server's answer is a toast). A change to these settings goes in the model and both screens.
- **Damaged videos** (`media/repair.go`): a video whose seek index is broken (often FLV, some named
  .avi) can't be jumped in: transcodes started past 0 read nothing, thumbnails and trickplay fail. The
  "Damaged videos" job (after each scan and every 6 hours, like the ASS cache: `repairPath/index.json`
  by `config.CacheKey`, size and mtime) checks each changed video (`checkSeek`: ffprobe drops the index
  as invalid, or a jump to the middle finds no video or video far off, by the packet's pts, else its dts (DivX in AVI has none);
  landing well before it is fine when no keyframe lies between, `keyframeBetween`: a video with gaps, as an unfinished encode;
  MPEG-TS packet lines have an extra field; `repairChecks` has those an older check listed looked at again) and lists a damaged one (state `damaged`); only an admin's Fix (`POST /api/admin/damaged/fix`,
  `RepairJob.Fix`: in the background, one at a time, shown as `fixing`, refused unless about the video's size
  plus 2 GB is free) copies its streams untouched into an indexed Matroska file in
  `media.repairPath` (default `repairs` beside the thumbnails; `fixVideo`, checked to seek). `media.Playable`
  gives that copy in the video's place to everything that reads it: the stream, codecs, seek-point,
  subtitle and font handlers (`h.playable`: not with `original=1`, or for an admin who chose Play original,
  `POST /api/admin/damaged/play`, for an hour), thumbnails, trickplay, intros and credits, the ASS cache,
  downloads and the scanner's length. Fixing one removes its trickplay frames and thumbnail for remaking and
  stores its real length (the damaged header's is often wrong). Settings › Server lists them for admins
  (`GET /api/admin/damaged`, with each one's thumbnail when the library has one; web, phone, iPhone, TV) with Play original, Play fixed copy and Replace with
  fixed copy (`POST /api/admin/damaged/replace`, `media.ReplaceWithFixed`: the copy goes in as `<name>.mkv`
  with the original's time on disk, refused while someone else plays it; `db.MoveFile` moves its progress,
  segments, subtitle timing and episode matches to the new client path; the original goes to the trash,
  `media.trashPath`, default `trash` beside the thumbnails, emptied after 30 days by the job or with Empty
  the trash, `POST /api/admin/trash/empty`). Each trashed file has its own folder (`<unix>-<n>/`, its ID)
  with a note (`.trash.json`: its disk and client paths, its replacement's, the reason and lengths) and a
  copy of its thumbnail (`.thumbnail.jpg`: the Cleanup job deletes a gone file's). Settings lists the trash
  (`GET /api/admin/trash`: name, library and folder, size, when it went in and is deleted, the thumbnail from
  `/api/admin/trash/thumbnail?id=`, admins only) with Restore (`POST /api/admin/trash/restore {"id"}`,
  `media.RestoreFromTrash`), which undoes a replace: the original back in its place, the `.mkv` back to the
  repairs folder as its fixed copy (listed as fixed again), the thumbnail back if it's gone, and `db.MoveFile`
  the other way; refused while a file is in its place, the replacement has changed, or someone else plays it.
  The job keeps a restored video's index entry until a scan lists it. Both folders are on the Jobs page's storage list.
- **People** (a title page's cast: a tap, a click or OK opens a person's window): `GET /api/library/person?lib=&id=`
  (`metadata/people.go`) gives a larger photo (`artwork/person/<id>large.jpg`, h632), their TMDB details
  (biography, birth and death, place of birth, known for), the library's titles they're credited in (as its
  titles, with progress and their `role`) and their whole TMDB filmography (cast and crew merged per title,
  newest first, announced ones at the top; `poster` a TMDB image URL, `in_library` those here, which open as
  title pages). Fetched from TMDB (`/person/<id>?append_to_response=combined_credits`) when first opened and kept
  in `person_details` (refreshed after 30 days, the stored one used when TMDB can't be reached); only people a
  stored title credits can be opened. The web's policy allows images from `https://image.tmdb.org` for these
  and the Fix dialog's search results. Clients: web `openPersonDialog`, phone `PhonePersonDialog` and TV
  `TvPersonDialog` (`PersonDialogs.kt`, `TvPersonDialog.kt`), iPhone `PersonSheet`.
- **Collections** (movie libraries, admins make them; `database/collections.go`): a name, an optional description
  and the library's titles (by kind and TMDB ID, `user_collections` / `user_collection_titles` in the media
  database), seen by everyone who sees the library and changed only by admins. `GET /api/library` gives
  `user_collections` (each one's titles still in the library, `count`, and the posters of its first four by release
  date for its collage); `GET /api/library/usercollection?lib=&id=` is its page (titles by release date with
  progress); `POST|PUT|DELETE /api/library/usercollections` create (`lib`, `name`, `description`, `titles`), change
  (`id`, `name`?, `description`?, `add`?, `remove`?) and delete one; only the library's titles go in. Clients: the
  web's Collections (`showCollections`, `showCollection`, `openCollectionEditor`, a title page's Add to collection,
  `openAddToCollection`; × takes a movie out), the apps' view button (posters → collections → TMDB collections,
  `isUserCollectionsView`, `openUserCollection`): phone `PhoneUserCollections.kt` (editor, long press takes a movie
  out), TV `TvUserCollections.kt` (New collection and Edit collection tiles, the movie picker, hold OK takes one out),
  iPhone `UserCollections.swift`. The collage: one poster fills it, two side by side, three one tall and two
  stacked, four 2×2.
- **TMDB Collections view** (movie libraries, after Collections): TMDB's collections (film series). A movie's
  details name its collection (`belongs_to_collection`, stored as `movies.collection_id`: 0 none, -1 not looked
  up yet), and `ensureCollection` stores the collection (`collections`: name, overview, poster, backdrop) and
  its films (`collection_parts`, each poster downloaded) under `artwork/collection/<id>/`, refreshed every 30
  days like the titles; movies stored before ask only `client.MovieCollection` (`ensureMovieCollection`,
  `needsWork` counts them due), and unused collections go with the artwork cleanup. `GET /api/library` gives
  each movie's `collection_id` and the library's `collections` (name, poster, `count` here of `total`);
  `GET /api/library/collection?lib=&id=` is a collection's page: every film by release date, the library's as
  its titles (with progress) and the rest `missing`. Collection artwork is served to whoever can see one of its
  movies (`canAccessCollection`). Clients: the web's switch (Folders | Library | Collections | TMDB Collections,
  `showTmdbCollections`, `showTmdbCollection`, `?view=tmdbcollections|tmdbcollection`), the apps' view button (list → grid → library →
  collections → TMDB collections → list; `isCollectionsView`, `openCollection`; Back goes from a title to its collection, then the
  collections): phone `PhoneCollectionsScreen`/`PhoneCollectionScreen`, TV `TvCollectionsScreen`/
  `TvCollectionScreen`, iPhone `CollectionsScreen`/`CollectionScreen`. Missing films are dimmed and open nothing.
- **Fixing TMDB matches** (admins, movie and TV libraries; `handlers/match.go`, `/api/media/match…`): the web's Fix
  dialog (`openMatchDialog`) and the apps' (Android `MatchModel.kt` with `PhoneMatchSheet.kt` and `TvMatchDialog.kt`,
  iPhone `MatchSheet.swift`) show the current match, search TMDB (Movies or TV shows where the library allows
  both) and match a result, mark no match or reset, pick a poster, and map an episode file to another TMDB episode
  for that file or its whole season. A file is sent by its base64 path, a folder standing for one title (the browse
  response's `fixable`) by its client path (`dir`). The apps open it from a title page (Fix match: the whole title,
  through its first file, `wholeTitle`, without the episode part; a file or episode row's pencil), a Library view
  Unmatched file and a folder view video or `fixable` folder (long press on the phone and iPhone, holding OK on
  the TV, `onHoldOk`; the TV dialog takes OK only after a fresh press, `okAfterFreshPress`); after a change the
  screen behind reloads (`closeMatch` / `matchChanged`; a title page matched to another title closes). The TV
  starts the search from the title the server took from the file's name, so the keyboard is rarely needed.
- **3D videos marked twice** (`media/stereo.go`): an H.264 side-by-side (or other 3D) video whose
  layout is marked in both the container (Matroska's StereoMode) and the stream's SEI can't be decoded
  by ffmpeg 8: each frame gets the marker twice ("File exists" from the filter graph), so no thumbnail,
  trickplay or transcode, though its original plays. Such a video (`stereoMarked`: H.264 with the
  container's Stereo 3D side data, remembered until it changes) is read with its SEI left out by an
  input bitstream filter (`inputFilters`: `-bsf:v filter_units=remove_types=6` before `-i`, chained after
  the matrix filter where there is one): thumbnails, trickplay, transcodes, download encodes and credits
  on black. The file isn't touched, and it isn't a damaged video (no fixed copy).
- **Format lines** (`media/format.go`): each video's resolution, video codec, HDR and main audio track, as
  "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1" or "1080p · H.264 · AAC 2.0" (resolution by width as well as
  height, so a cropped 1920×800 film is 1080p; Dolby Vision from the stream's DOVI configuration record, else HDR10
  or HLG from its transfer; the default audio track, DTS by its profile, Atmos from the profile). The scanner
  builds it from its ffprobe and stores it in `media_items.format` ("" not read yet, "?" none, "DVD" for discs);
  videos scanned before are read once by the next scan (`SetMediaFormat`, the item's ID kept). Browse items and
  title pages' files (`LibraryFile.format`) carry it ("?" sent as nothing). Every client shows it between a
  video's name and its size and length: folder-view list rows (the web's tiles too) and title pages' file and
  episode rows (each distinct format of an episode's files).
- **Theme music** (`media/themes.go`, `GET /api/library/theme?lib=&kind=&id=`; `TitleDetail.theme` says there is
  one): a title page plays its show's or movie's theme once, quietly, and fades it out when the page closes or a
  video plays. The theme is a `theme.<ext>` file in the show's folder or the movie's own (flac, wav, m4a, aac, mp3
  sent as they are; opus, ogg, oga, wma converted to AAC once; one folder with several plays the best, in that
  order), else cut from the videos: a show's intro (the segments table's, its first episode that has one, specials
  last), a movie's music its first ten minutes and its credits share (`MovieThemeSpan`: Chromaprint, as intros;
  20 s or more, remembered per file), else the first minute of its credits. Cuts and conversions are AAC in
  `media.themePath` (default `themes` beside the thumbnails, on the Jobs page's storage list), named by a hash of
  the file, its size and time and the part cut, faded in and out, up to 2 minutes; the first request makes one
  (a few seconds for a movie's search). A title with neither has none. Settings (each device's, on by default):
  "Play theme music on title pages" and its volume, Low, Medium or High (15%, 30%, 50% of the device's volume;
  the TV has one picker row for both, Off | Low | Medium | High, `TvSettingsRow`'s `stepValue`/`onStep`):
  web My Account › Playback (`localStorage` `themeMusic`, `themeVolume`; `playThemeMusic`, `stopThemeMusic`),
  Android `ThemeMusic.kt` (`TitleThemeMusic` on the phone's and TV's title pages; downloaded through the app's
  client, then `MediaPlayer`), iPhone `ThemeMusic.swift` (`AVAudioPlayer`, the ambient session: it mixes and
  keeps to the silent switch).
- **Video thumbnails** (`media/thumbnail.go`): a frame a minute in, and while it's blank (`frameBlank`:
  mean brightness under 0.07 or over 0.93, little spread in brightness and color, or 80% or more of it
  one color, as a white card with a small logo: a fade, a title card, one color) 30 s further, up to 3½ minutes in; the first that isn't wins, else the least blank. A video
  shorter than a minute tries 30 s, 10 s and 1 s; a disc a tenth, a fifth and three tenths into its main
  title. Thumbnails under 1 KB count as broken, so a video black throughout still gets none. Existing
  thumbnails stay until they're regenerated.
- **The home screen's libraries** (web `renderMedia`, Android `HomeTiles`, iPhone `FileBrowserScreen`): the TV
  Shows and Movies libraries come first, in config.yml's order (each client keeps the order the server listed them in; Settings' library list moves them: arrows on the web, phone and iPhone, the TV library dialog's Place in the list, `movePath`; `config/file.go` keeps the comment above the first library at the top), each with its kind's icon on its folder (Material movie and
  TV; in a corner badge over a folder picture; the browse response's `library_type`), then the Guide's tile,
  then the other libraries from the next row, also in config.yml's order (the TV's grid fills the row with empty cells, as it
  moves focus by index; the phone's grid spans a break; the iPhone draws two grids).
- **Folder pictures:** in folder view every folder shows a picture inside its folder icon (posters are only the Library view's) (web
  `folderPreview`, Android `FolderPreview` in `FolderThumbnails.kt`, iPhone `FolderPreview`): the
  browse response's `thumbnail_url` on each folder (`folderImage`, `database.FolderPreview`): its own
  `thumbnail.png` if the media database has it, else the thumbnail nearest the folder among the first 200
  under it in path order (its own files before its subfolders'), else the TMDB poster of a matched
  folder with nothing thumbnailed yet. Thumbnail URLs carry the thumbnail file's time (`?v=`,
  `versionedThumbnailURL`; `ThumbnailFile` strips it for file names), so a remade one is fetched
  anew everywhere it's used. Regenerate Thumbnails also remakes the picture each subfolder's icon
  shows (`folderPictureFile`) and sends a thumbnail event with the folder's path, which the web,
  Android and iPhone apply to the folder's icon. A library (the root listing) shows
  only the `thumbnail.png` directly in its folder. In folder views (only there) videos have filmstrip
  edges, telling them from photos (`.filmstrip` on the web, `Modifier.filmstrip()`, `Filmstrip`).
- **Backups** (`internal/backup`): one `weirdstreamer-backup-<time>.tar.gz` holds `users.db` and
  `media.db` (consistent copies made while the server runs: `database.CopyTo`, SQLite's `VACUUM
  INTO`), `config.yml` and `manifest.json` (version, time), owner-only (password hashes, secrets).
  Where is config.yml's `database.backupPath` (Settings shows it, never changes it, as with the other
  data folders; default `BackupDir`: `backups` beside a `data` folder holding the databases, else
  inside their folder). When is `backupSchedule` (off, daily, weekly; default daily), `backupTime`
  (04:00), `backupDay` (weekly's; sunday), and `backupKeep` (7) are kept, the oldest pruned; Settings
  (web Server tab, phone and iPhone Server tab) changes these, and its Back up now is `POST
  /api/admin/backups` (`GET` lists them, with the folder and the next time). The schedule runs as the
  Backups job (the Jobs page, Run now): the next time after the newest backup, or with none after
  the server started; two in one second get `-2`. `cli backup now | list | restore <file>
  [--with-config]` (restore refuses while a server answers on the config's port; the files it
  replaces go to `before-restore-<time>/` beside the users database). `setup.sh restore` picks one,
  stopping and restarting the service around it, and `setup.sh upgrade` backs up first. The backup
  folder is on the Jobs page's storage list.
- **Recently Added:** `showNewAdditions` (`config.yml` or Settings) is `on`, `libraryOnly` or `off`
  (the default): the last 10 videos show under Continue Watching on the library's own Library and
  top folder pages ("Recently Added", `GET /api/library/new?lib=`), and with `on` also on the home
  screen, a row per library ("Recently Added in Movies", `?by=library`, the one with the newest
  arrival first; without it, the blended
  list older apps show). On the TV, Up and Down go row by row (`TvContinueRow`). The `media_added` table dates each file by its time on
  disk: when it's saved (`SaveMediaItem`), and every known file again at startup (`DateMediaAdded`).
  Only files in the media database are listed, so a scan runs every hour by itself (`StartScans`,
  the first 10 minutes after start) as well as on Scan Media. The newest day comes first, and a day's videos in viewing order (show,
  season, episode). `media_items`' `created_at` is no use for this: every save replaces the
  row. Clients turn `added_at` into "Added 2 days ago" on their own calendar.
- **Update banners:** the apps run `AppUpdater`'s check when they open, on another server and every
  6 hours (`UpdateChecks`), and `UpdateBanner` points to Settings: on the TV only to read, gone
  after 15 seconds; on the phone a tap opens Settings and X closes it for that version. The web
  has no version number: `checkForNewWeb` compares its files' dates and sizes with those at page
  load, every 5 minutes and on returning to the tab, so saving a file in `Server/web` shows the
  banner in open pages. A click reloads.
- **Sign-ins:** every sign-in (password or TV code) is a row in the users DB's `sessions` table
  (`database/sessions.go`), named by its token's ID (`jti`). The middleware refuses a token whose
  session is gone and reads the user, admin rights and group from the database on every request
  (the token's `is_admin` is only for the apps' menus). Signing out (`POST /api/auth/logout` with
  the token; the apps send it when an account is signed out or a server removed) ends that session;
  a new password ends the user's others (`cli user change-password`: all of them); deleting a user ends all.
  Usernames are limited to an email address's characters (`database.ValidUsername`). The server
  won't start with a blank `auth.jwtSecret` or config.example.yml's placeholder, and `/api/config`
  never sends the secret. Settings' save (`PUT /api/config`) changes only the fields its screens show
  (`settingsUpdate` in `handlers/config.go`: server name, port and host, streaming switches, TMDB,
  OpenSubtitles, language, libraries); the rest of what it's sent is ignored, and a new field Settings
  shows must be added there. The Guide's settings are the Guide tab's own save (`PUT /api/admin/guide`,
  `handlers/guideadmin.go`), which `/api/config`'s keeps as they are. The web UI never sees its token: its sign-in sends `web: true`, so the
  token goes only into the HttpOnly cookie (`Secure` over HTTPS, including behind a trusted proxy
  that sends `X-Forwarded-Proto: https`), and its requests send `X-WeirdStreamer-Web: 1`, without
  which the cookie works for GET only (other sites can't add the header).
- **Account PINs (apps):** a server account saved on a device can have a PIN there, so others using
  the device can't open it from the source list (`chooseServerAccount` / iPhone `chooseAccount` ask
  first: `pinRequest`). Each device keeps its own (Android `AccountPins`, iPhone `PinStore`: salted
  PBKDF2, never sent anywhere), set in Settings › Account: 4 or more digits on a phone (`PhonePinSettings`,
  the iPhone's `PinSettings`, asked for with a number pad: `PhonePinPrompt`, `PinDialog`), and on a TV a
  code of 4 or more D-pad arrows (`TvPinSettingsRow`, `TvPinPrompt`) of which the screen only flashes a dot
  per press; OK ends it, Back clears it, and the code is checked only at OK. Changing or removing one asks
  for the current one. After 5 wrong tries each try waits (30 s, doubling to an hour), across restarts.
  Forgotten: "Use the password instead" (the TV: hold OK) signs the account in again, and only a sign-in
  that works for that same account clears the PIN. Signing the account out (Android) or removing its
  server clears it. SMB accounts have none.
- **Server certificates (apps):** home servers often have self-signed certificates, so the apps
  trust a saved server's certificate whatever it is, but only that one: its SHA-256 is kept when
  the server is added (from the address check), or at the first connection of a server saved
  before, keyed by host and port. A different one is accepted (and kept instead) only if it passes
  the usual check, a trusted authority for the server's name (a Let's Encrypt renewal); otherwise
  it's refused with a message saying to remove and re-add the server if the certificate was replaced. Removing the server forgets it. Other hosts
  are checked as usual. Android: `api/ServerTrust.kt` (every OkHttp client goes through
  `ServerTrust.secure`; only the address check uses `probing`). iPhone: `TrustPolicy` in
  `ServerClient.swift` (also the HLS relay's and downloads' sessions, and `OriginalLoader.swift`: AVPlayer
  checks certificates itself, so an HTTPS original or Live Photo clip has a `wsoriginal://` URL
  whose byte ranges the app fetches through TrustPolicy).
- **Rewind when resuming** (Settings, off by default; TV, phone, iPhone and web): unpausing goes back up
  to 5 seconds first (`resume()` in `VideoPlayer.kt` and `PhoneVideoPlayer.kt`, `togglePlay(rewinding:)`
  on the iPhone, `resumePlayback()` in `app.js`), shown as a manual skip back is: the TV's "-5", the
  phone's and iPhone's swipe-seek indicator ("−0:05 / 12:29"), and the same text over the web player
  (`showSeekIndicator`). Every unpause must go through it. The web keeps it per browser (Settings ›
  My Account › Playback, `localStorage.rewindOnResume`).
- **Languages** (account settings, kept on the server: `users.audio_language`, `subtitle_language`
  ("" for File default, the default) and `subtitles_on` (off by default); `GET/PUT /api/account`; web
  Settings › My Account, phone and iPhone Settings › Account, the TV's Settings › Languages,
  `TvLanguagesDialog`): the codecs report carries `preferred_audio` and `preferred_subtitle`
  (`media.PreferredTracks`, -1 for the file's default): an audio track in the audio language, and,
  with subtitles on, full subtitles (not forced; PGS and DVD count) in the subtitle language, or
  the file's own pick for File default. Every player starts with those unless the video has a
  remembered choice; a language the video lacks plays as before (subtitles off but forced ones).
  Downloads (phone and iPhone) ask for audio and subtitle languages when the files have a choice
  (`DownloadLanguages`), starting with the last ones changed in the dialog, else what playing would
  pick. The item keeps the languages (Download ahead copies them from the show's hand downloads) and
  the worker turns them into tracks: encoded copies keep that audio track (the job's `audio`), an
  original keeps all and plays it (`audioTrack` of `audioTracks`), and the chosen subtitle track is
  saved and shown offline (`subtitleTrack`).
- **Progress and resume:** players report position every 5s, on pause and at the end. The
  server decides "watched": leaving at or after the credits start (the segments table's), else the
  last 2 minutes of videos 20 minutes or longer, the end of shorter ones. Credits found or moved
  later (the analyzer, or marked by hand) finish the viewings that stopped past their start
  (`finishPastCredits`). Credits on black are a stretch at least 80% black (`BlackCredits`: a white
  crawl on black is 85-90%) reaching to near the end. An episode's credits found by fingerprint give way to credits on black that start later (music the
  episodes share can come minutes before the credits). A video goes on Continue Watching once played 10 seconds in (`database.ListedPosition`)
  and then stays at any position (the playback row's `continue_state`: listed), until it's finished or
  removed; removing sets it to removed, which also keeps an up-next episode off. An up-next episode
  only opened (under 10 s) is still up next. `MediaViewModel.playVideo(path, fromStart)` sets `playbackStartMs`.

## Notes on the TODO items

### The Guide

The plan, its settled decisions and open questions are in `docs/GUIDE.md`: keep it up to date as the
work goes, and follow its decisions.

What's built (server):
- **Data from TMDB.** Shows keep their first network and US rating, and movies their US certification
  (`shows.network_id`, `network`, `rating`; `movies.rating`). A title without them (`guide_info` 0) is due
  for the TMDB lookups job.
- **Library settings.** `showInGuide` (on, libraryOnly, off: `MediaPath.InGuide`) and `guideFiller` on
  each library, set in Settings' Guide tab (below). A filler library's videos get no trickplay frames (`config.InGuideFiller`): the trickplay job
  skips them (removing any made before, logged) and only reads a missing length, which the Guide plans with;
  `cli trickplay` for a library or every video skips them too.
- **Lengths.** The Guide schedules only videos whose length is stored (`media_items.duration`). A scan reads a
  known video's missing length (one scanned before ffprobe was installed, or when it failed: `lengthsFilled`,
  logged), and each scan prompts the Guide to plan (`MediaHandler.AfterScan`; planning never changes the next
  6 hours), so network channels appear once enough of their shows have lengths. `guide.primeTime` (19 or 20) is in config.yml.
- **`internal/guide`.**
  - `Channels` groups one library's titles: a network with 4 or more shows gets a channel (named by
    `CallSign`), the other shows and the movies go into genre buckets.
  - `Plan` lays a channel out from a `State`, which `Record` builds from the stored slots, so each show
    goes on to its next episode. In prime time each weekly position's shows take turns a season each;
    the rest of the day is strips (a week's rotation per day part) or the movie aired longest ago.
    Ratings depend on the day part. Gaps get filler clips (near the program's year first, none
    repeated within 2 hours on the channel), then a bumper.
  - `Service` plans 72 hours ahead, hourly and when settings change (the Jobs page's Guide job). When
    a channel's titles change it plans again from 6 hours ahead. Slots are kept 35 days.
- **Tables:** `guide_channels` (numbers in blocks, `guide/numbers.go`: Guide 1, networks 2–19, Weather 20–29, TV
  genres 30–49, movies 50–69, each typed definition its ten from 100 (`Typed`), overflow from 200; one outside its block moves in at the next refresh, `Renumber`) and `guide_slots` (client
  paths, a program's info as JSON), in `database/guide.go`.
- **API** (`handlers/guide.go`): `GET /api/guide/available`, `GET /api/guide` (the grid),
  `GET /api/guide/now?channel=` (the file, the offset, and the program on now and next) and
  `POST /api/guide/watched?channel=`, each with an optional `lib=`. A channel is one library's, so access
  is that library's.
- **Logos** (`guide/logo.go`): every channel's own, in one color as stations' were: a symbol
  (`logosymbols.go`) or the number's holder as the mark, with the name beside or below it.
  - The number sits in a ring, square or rectangle, or is cut out of a disc, block, bar or triangle.
  - Served as SVG or PNG at any width, transparent around it, in the color asked for
    (`GET /api/guide/logo?channel=N&w=&color=rrggbb&format=svg`; the channel's `logo` in the replies).
    The web draws them in the theme's text color, white on cards.
  - The artwork is made once by `cmd/logoart` (designs in `logodesign.go`), drawn with Google Fonts'
    OFL display faces. The fonts stay on the machine that ran it (`-fonts <folder>`; the families are
    named in `logodesign.go`).
  - The outlines are saved in `guide/logoart/*.json`, which the server embeds: a logo per genre
    channel, and a pool of made-up stations that network channels take their call signs from
    (`PickStation`: none taken twice, none in another channel's font).
  - Each logo has its own font. The server only draws the channel's number, in Liberation Sans Bold
    (`logofont/`, OFL). A solid holder takes the digits reversed (`reversePath`), so they cut it.
- **Web** (`js/guide.js`, loaded after app.js and using its helpers): on the home screen the Guide is a
  folder-sized tile after the TV Shows and Movies libraries (`createGuideTile`, placed by `renderMedia`); on a
  library's own page `createGuideRow` puts a Guide button just above the folders. The Guide is a screen like Settings
  (`SCREENS.guide`). A screen's history entry keeps what it shows (`pushScreenHistory`: Settings' `tab`, kept by
  `selectSettingsTab`; the Guide's `guideLib`), so a reload or Forward opens it again as it was, over the view the URL names.
  - Top left is `#guide-video`. It tunes to the last channel watched in this browser
    (`localStorage.guideChannel`).
  - Each item from `/now` plays as is, seeked to the channel's offset, where the browser can play it
    (`originalProblem`, the default audio track). Otherwise it plays through the server's stream with
    `start=` (Compatibility (Auto)). An item the browser then fails on (a QuickTime .mov in Firefox) goes on
    through the stream, then a full transcode, from where the channel is (`playGuideItem`'s `onerror`).
  - A timer asks `/now` again when the item ends, by the server's clock (`guide.skew`).
  - Bumpers are a card drawn over the picture (`showBumper`).
  - There's no seeking, and a pause mutes instead. Typing a number tunes to that channel, and Up and
    Down change channel in full screen.
  - Settings' Guide tab (`loadGuideSettings` in `guide.js`): typed channels, the libraries' Show in Guide and Use for
    Guide Filler, prime time, the weather ZIP and the voice-over tester.
- **Android** (`Guide.kt`: `GuideModel`, `GuidePlayer`, the cards; `TvGuide.kt`, `PhoneGuide.kt`):
  - On the home screen the Guide is a tile after the TV Shows and Movies libraries (`HomeTiles`; `guideRow`
    is "" there); on a library's own page it's the last of the rows above the folders (`MediaViewModel.guideRow`,
    from `/api/guide/available`): a button in `TvContinueRow` and `PhoneContinueRow`. `openGuide` sets
    `guideOpen`, which `MainActivity` shows instead of the browser.
  - `GuideModel` follows the tuned channel through `/now` (a timer at each item's end, by the
    server's clock) and keeps the last channel per server and Guide (`SharedPreferences` "guide").
  - An item plays as is, seeked, where `OfflinePlayback` says the device plays it. Otherwise it plays
    through the server's stream from where the channel is (H.264 copied, `audioLadder`'s audio), and
    after a player error as a full transcode.
  - One ExoPlayer serves the small window and full screen.
  - TV: the D-pad grid. OK on a program on now on another channel tunes to it; OK on the picture goes
    full screen, where Up and Down change channel. There OK shows the channel banner (`TvChannelBanner`,
    `GuideModel.banner`): the channel's logo, number and name and its program on now (title, episode, times,
    description, and the time at its lower right, which stays put as the banner's height changes); with it up, Up and Down show the other channels' without tuning, OK tunes to the one shown,
    Back hides it, and it hides itself after 6 seconds untouched (`BANNER_MS`). Number keys tune, and Play/Pause mutes.
  - Phone: the grid scrolls sideways. A tap on the picture goes full screen (sideways), where swipes
    change channel; there's a mute button.
  - Settings' Guide tab (`GuideSettingsModel` in `AdminSettings.kt`; the phone's `PhoneGuideSettings.kt`, the
    TV's `TvGuideTab` in `TvAdminSettings.kt`).
- **Marking shows watched:** no button. Each device's setting "Mark Guide shows watched" (off by default; web
  My Account › Playback `localStorage.guideMarksWatched`, Android `guideMarksWatched`, iPhone
  `guide_marks_watched`): when the item after a library channel's program starts at or after its end while
  that channel is tuned, the app sends `POST /api/guide/watched?channel=&start=<the program's start>`.
- **TV:** OK on the tuned channel (its name or what's on now) goes full screen, since the picture is hard to
  reach with the arrows.
- **Voice-overs on the credits** (`guide/voices.go`, `voiceovers.go`): Piper (`guide.piperCommand`,
  `piperVoices`) says a line ("Stay tuned for {next}, next on Channel {channel}…") in the channel's announcer's
  voice (by its number), with the announcer's fixes (`match` → `say` with `[[ phonemes ]]`, optional `pace`:
  that piece said apart and joined), then ffmpeg's filter chain; kept in `guide.voiceOverPath` by a hash of the
  line and the announcer's settings. All of it is in `guide-voices.yml` beside config.yml (written from
  `voices.default.yml` when missing, read again when it changes). A job makes the next 4 hours' after each
  planning and every 10 minutes. `/now`'s `voice_over` (only once made): `at` seconds into the program's file
  (a second into known credits, never ending later than 5 s before the end; unknown credits: ending 5 s
  before it, `voiceOverTiming`), `seconds`, `squeeze_at` (known credits only). Never over filler. The players
  start it when they reach `at` by their own position in the file (an original's position; a stream's start
  plus its position: web `guide.fileStart`, Android `GuideStream.fileStartMs` via `GuideModel.playerPosition`,
  iPhone `fileStart`; the clock only until the item has loaded), so a stall doesn't put it early; they
  play it over the program (lowered to 30%), squeeze the picture beside an "Up next" card (web `guide.js`
  `scheduleVoiceOver`, Android `GuideModel.scheduleVoiceOver` / `UpNextCard`, iPhone the same names), and show
  Report voice-over while it plays (`POST /api/guide/voiceover/report`). The web's Settings › Guide
  has the tester (`/api/admin/guide/voices`, `/try` with phonemes from Piper's `--debug`, `/fix` saved keeping
  the file's comments, reports). The e2e server has Piper in `e2e/piper-venv` and the voices in `e2e/piper-voices`.
- **The grid scrolls to the end of the plan:** `/api/guide` gives `until` (72 hours ahead) and takes `from=`;
  every app loads the next 3 hours as its grid nears the end of what's loaded (`extendGrid`, web
  `extendGuideGrid`), merging a program across the boundary once, and the half-hourly reload keeps the hours
  loaded. A half hour has a fixed width (TV: 3½ hours fit; web 200 px); the TV's rows and times share one
  ScrollState, moved by focus. The corner shows the day at the left edge, midnight's label is the day, and
  the details name the day when it isn't today.
- **Channels of their own** (`guide/special.go`; no library, `LibraryID` ""; the home screen's Guide only, while
  it has a library's channel): the Guide channel (1) and Weather (20–29), made by the planner's
  refresh. Their grid half hours are `ProgramInfo` kind `channel`; the channel's `kind` says
  which screen the apps draw (`guide`, `weather`). `/now` gives `offset` (seconds into the file) and every
  client plays from `offset` plus the time since `server_time`.
  - Guide channel: `GuideChannelBlock`, 30-second blocks by the clock (preview, its promo card, preview,
    commercial), worked out per request from the viewer's channels and filler (`Service.FillerClips`); `/now`
    adds `airs` and `airs_on`. The apps draw the picture in the top 58% and the listings roll below.
  - Weather: `guide/weather.go`, the National Weather Service for `guide.weatherZip` (Settings › Guide; default
    10001), the ZIP's position from the embedded Census table (`zipcodes.txt.gz`); `GET /api/guide/weather`
    (10-minute cache), `/weather/radar` (the radar station's latest GIF), `/weather/icon?name=` (drawn like the
    logos' symbols, `weathericons.go`). Pages turn every 10 s by the server's clock.
  - Music: `guide/music.go`, the tracks in `guide.musicPath` (default `guide-music` beside the thumbnails),
    shuffled per day and played by the clock from midnight; `/now`'s `music` (url, offset, credit from
    `<track>.txt`), `GET /api/guide/music?name=`. Under the Weather channel and the Guide channel's cards, with
    the credit on screen (CC BY). The e2e server has Kevin MacLeod's tracks in `e2e/guide-music`.
  - Clients: web `guide.js` (`setChannelScreen`, `renderGuideListings`, `showPromo`, `renderWeather`, the
    `<audio id="guide-music">`), Android `GuideChannels.kt` (drawn inside `GuidePlayer`; the music is
    `GuideModel.music`, its own ExoPlayer), iPhone `Views/GuideChannels.swift` (`GuideModel.music`, an AVPlayer).
- **Typed channels** (`docs/GUIDE.md`; `config/typed.go`, `guide/typed.go`, `guide/folders.go`): `guide.typedChannels`, each
  a type (`config.TypedChannelTypes`: Late Night, late night content, airs everything in its libraries all day,
  `Channel.AnyTime`; the genre types their TMDB titles of that genre, `TypedMatch`), its libraries, up to `max` (1–4)
  channels and its block of ten numbers from 100 (`number`, given when added). Their channels' library is
  `typed:<id>` (`TypedLibraryID`), keys `typed:<id>/<group>:<part>`: the handlers show them on the home Guide only,
  to groups that can open every one of the libraries (`canSeeTyped`), and their libraries are in no regular channel
  (`InTypedChannel`). Folder libraries are read from their files (`FolderTitles`): series (SnnEnn, or names alike
  but for numbers, or Season folders), collections (fewer than 5 videos join the parent's) of long videos, and
  short clips aired in half-hour blocks (an hour for long clips, `Title.BlockLength`) that the planner fills when it
  schedules one (`planner.block`: the collection's clips aired longest ago; the first the program, the rest its
  filler, `Slot.Parts`; `ProgramInfo.block`, no voice-overs), topped up by the definition's clips not aired in the
  last 2 hours, which fill all its channels' gaps.
  `TypedChannels` splits a definition's content: a channel per kind with a day of it, the rest joining the
  largest, then splits while each part has two weeks. Each type has four channel names with logos of their own
  (`TypedChannelNames`, `typedDesigns` in `logodesign.go`, `typed-*.json`), taken in order (`TypedName`; a fifth is
  the first name and a number). A folder library's program is titled "<folder> - <the file's>" (`Title.Folder`):
  an episode's own title, else its file name cleaned (`fileTitle`, `episodeSummary`; nothing but SnnEnn: the folder's alone); a block's first clip "and
  N more". Late Night channels aren't previewed on the Guide channel. Nothing checked in names what a Late Night library holds.
- **Settings' Guide tab** (admins; web, phone, TV, iPhone): `GET/PUT /api/admin/guide` (`handlers/guideadmin.go`):
  typed channels, every library's Show in Guide (movie and TV) and Use for Guide Filler, prime time, the weather
  ZIP, the music and Piper found; saved apart from `/api/config` (the Server tab no longer shows these).
  Plan again from now (asked first; only by hand, `POST /api/admin/guide/replan`, `Service.Replan`) plans every channel
  again from its next program, the next 6 hours too; what each airs now stays.
- **iPhone** (`Session/Guide.swift`: `GuideModel`; `Views/GuideView.swift`: `GuideScreen`), as the phone's:
  - On the home screen the Guide is a tile after the TV Shows and Movies libraries (`GuideTile`, `GuideRow` in
    `FileBrowserScreen`); on a library's own page its button ends `ContinueRows` (`AppModel.guideRow`); `openGuide` sets `guideOpen`, which
    `RootView` shows as a full-screen cover. The last channel is kept per server and Guide (`UserDefaults`).
  - An item plays as is (`OriginalLoader`, seeked) where `OfflinePlayback` says the phone plays it, else
    through `HLSRelay` from where the channel is (H.264 copied, the audio kept where it plays), and after a
    failure as a full transcode. A pause from outside mutes and plays on.
  - The grid's half hours follow the programs' sideways scroll (`onGeometryChange`), and one time line
    runs over them. Full screen asks the window to turn sideways; swipes change channel.
  - Settings' Guide tab (`GuideSettingsTab.swift`), as the phone's.
  - Compiled and unit-tested (`GuideTests`) through `mac-ios`; not yet run on a simulator or phone.


### More server platforms (feasibility, 2026-11-22)

Checked by type-checking the server for each target and reading `release.sh` (which builds
`linux-$(go env GOARCH)` only, with CGO for `mattn/go-sqlite3`). Nothing was built or run on them.

- **Linux arm64:** the code compiles unchanged; `setup.sh`, systemd and the scripts work as they are.
  Only the build needs an ARM C toolchain for SQLite (none on the build machine). About an hour or
  two, mostly packaging.
- **macOS (amd64 and arm64):** the code compiles unchanged. Needs building on a Mac (or a macOS CI
  runner). `setup.sh service` is systemd: needs a launchd plist, or the step skipped. Homebrew's
  ffmpeg lacks the DVD reader, chromaprint and libass: ship or point to `scripts/build-ffmpeg.sh`'s.
  The disk-busy check (`media/disks.go`, `/sys/dev/block`) is Linux-only and already falls back to
  nothing. About half a day.
- **Windows x64** (2026-11-25): the server builds and its tests pass on the test VM (Go, MinGW gcc,
  Gyan's "full" ffmpeg 9.1, which has dvdvideo and chromaprint), and the web UI works against it.
  Platform files (`_windows.go` / `_unix.go` or `_other.go`) cover disks, free space, stopping ffmpeg,
  low priority, memory and the folder picker's drives. Disk paths are Windows paths; client paths
  stay `/`, and only `config.ClientPath` / `DiskPath` convert: `DiskPath` checks each part
  (`clientPartsOK`: no `:` streams, trailing dots or spaces, reserved names, and the name as stored, so
  8.3 short names and other cases are refused), and paths compare case-insensitively (`SamePath`,
  `InDir`). Windows won't rename or delete a file that's open unless every handle allows it, nor rename
  over one that's open: files the server opens, renames or serves go through `internal/fileio`
  (`Open`, `OpenFile`, `Create`, `ReadFile`, `Rename`, `ServeFile`, `Dir`; on Unix the `os` calls),
  which shares delete access and renames with POSIX semantics, and moves copy only across disks
  (`fileio.CrossDevice`). A file ffmpeg or another program has open still can't be moved there:
  Replace with fixed copy and Restore answer that it's in use (`fileio.InUse`). A new file
  open/rename/serve uses `fileio`, not `os`. A transcode's output can't go to fd 3 there (no
  `ExtraFiles`): `separateOutput` (`transcache_windows.go`) makes a named pipe for it
  (`\\.\pipe\weirdstreamer-<random>`, inbound, one instance, local clients) and gives ffmpeg its
  name with `-y`. The data is locked down with ACLs instead of modes (`fileio.OwnerOnly`,
  `IsOwnerOnly`: the server's account, SYSTEM and Administrators, inherited by what a folder gets
  later, since Windows has no umask): `keepDataPrivate` there also closes the databases' folders
  (their WAL files come after startup), the backups' and config.yml, and `local.key`, a saved
  config.yml and the backup folder get it as they're written. Jobs wait for busy disks there too
  (`load_windows.go`): the CPU from `GetSystemTimes`, each disk's busy time from
  `IOCTL_DISK_PERFORMANCE` on `\\.\PhysicalDriveN` (no admin rights needed; named by number,
  "Disk 0"), a file's disk from its volume (`GetVolumePathName`,
  `IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS`; `disks_windows.go` numbers volumes by a hash of their
  root), and a network share (`DRIVE_REMOTE`: UNC paths, mapped drives) is timed as SMB and NFS are
  on Linux; there's no fadvise, so share probes may read cached pages. The server runs as a Windows
  service (`cmd/server/service_windows.go`: `svc.IsWindowsService`, from the package folder, a log
  file per start in the logs folder as live.sh keeps, Windows' Stop shuts down cleanly), installed
  by `cli service install [--user DOMAIN\name] | remove | start | stop | status`
  (`cmd/cli/service_windows.go`, an administrator PowerShell): as the installing user (its password
  is checked with `LogonUser`, given to Windows and kept nowhere; the account gets "Log on as a
  service", `SeServiceLogonRight`, through the LSA), started at boot with a delay (after the
  network), restarted by its recovery actions when it stops by itself (a crash, or
  `cli server restart`'s exit status 3), and let through Windows Firewall (a rule for
  `bin\server.exe`). Tried on the VM: restart, stop, Windows' stop and a reboot. The package is
  `release.sh`'s `windows-amd64` (below), set up by `setup.cmd` / `setup.ps1` (ffmpeg through
  winget's Gyan.FFmpeg, config.yml through `cli config init`, a self-signed certificate through
  `cli config self-signed`, the admin user and the service). `UseBundledTools` finds a package's
  `ffmpeg\bin\ffmpeg.exe` and joins the PATH with `;` there.
- **Building:** either switch to pure-Go SQLite (`modernc.org/sqlite`; no C toolchains, every target
  cross-compiles from the build machine and `release.sh` loops over them; benchmark the media DB
  first, it's slower than the C one), or keep CGO and build natively (GitHub Actions matrix, or
  `zig cc` for Linux arm64 and Windows; macOS still needs a Mac runner).
- Only Linux can be tested on the build machine: Mac and Windows packages need the user's testing or
  CI runs there.
