# WeirdStreamer

A self-hosted media server (Go) with a web UI (vanilla JS) and an Android app (Kotlin/Compose)
that runs as both an Android TV app and a phone app. License: AGPL-3.0-or-later (`LICENSE`, and the
copyright notice in `NOTICE`); the history was rewritten so no version was ever MIT. Pending work is in [TODO.md](TODO.md); notes
for each TODO item are at the end of this file.

## Rules (from the user)

- **Off limits:** the `k` and `p` libraries (`/home/USER/.personal/...`) and `p2`
  (`/home/USER/.personal2/...`). Don't browse, list, play or screenshot them. The one exception
  so far: reading codec/format details (ffprobe, no frames) of a file the user names.
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
    or Compressed. Each download also saves its subtitle tracks (text as WebVTT, PGS as .sup; the
    phone's languages, forced ones and files beside the video) in `subtitles/<id>/`, which
    `ServerSubtitles.forDownload` reads offline. Older downloads get theirs on the worker's next run.
  - `ui/components/UiUtils.kt`: transcode modes, `VideoDecoders` (hardware decoder limits),
    `subtitlesOffByDefault`, `subtitleLabels`, `SubtitleMemory`.

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
  Port 9000, HTTPS. Config is `Server/config.yml`
  (git-ignored; `config.example.yml` explains every setting). The server saves it too (Settings, Jobs,
  `cli config add-path`) through `internal/config/file.go`, which changes only the values that changed
  and keeps comments, blank lines, order and unknown keys. `cli config init <template>` writes a new
  one from JSON values with the template's comments (`create-config.sh`). Scripts read it with PyYAML.
  Data is in the folders `config.yml` names (databases, thumbnails, trickplay, artwork, download
  encodes).
- To deploy server code: `Server/scripts/restart-live.sh`. It checks for active streams, builds, stops
  the live server and waits for `live.sh` to start the new one (`--no-build` restarts the binary
  as it is). It refuses if the server wasn't started by `live.sh`; then the user has to stop it
  and run `scripts/live.sh` in that tab.
- Server tests: `cd Server/src && go vet ./internal/... ./cmd/... && go test ./internal/...`.
  Don't use `./...`: stray `main` files in `Server/src/` (`test_auth.go`, `test_login.go`) break it.
  The Go module is in `Server/src/`; binaries build into `Server/bin/` (`make build`, `scripts/build.sh`,
  `scripts/restart-live.sh`).

## Test environment

- `Server/scripts/run-e2e.sh` builds the server and runs a test server from `Server/e2e/` (git-ignored)
  on **http://localhost:9100**. Android emulators reach it at `http://10.0.2.2:9100`. It has its
  own config, databases and accounts (`e2e_admin`, `e2e_adult`, `e2e_normal`; password in
  `Server/e2e/pw`), and serves `Server/web` through a symlink.
- Sample library in `Server/e2e/sample/`: Movies and TV Shows (typed libraries with TMDB
  matches). Most files are short ffmpeg test patterns named after real titles. Notable ones:
  - `Movies/Example Film/…ts`: 5 minutes of H.264 and AAC in MPEG-TS (despite its name), which
    Chrome can't open: Compatibility (Auto) copies it into MP4.
  - `Movies/Subtitle Test/Subtitle Test.mkv`: 16 subtitle tracks, several per language, the
    first marked default.
  - `Movies/Example Movie/Example Movie.iso`: a symlink to a real DVD image on the share.
  - `Movies/a 4K film/`: a symlink to the real 4K file (E-AC-3 audio, text and PGS subtitles).
  - `Movies/EAC3 Test/EAC3 Test.mkv`: 2 minutes of H.264 with two audio tracks (E-AC-3 5.1
    default, AAC "commentary" at 880 Hz) and English and Spanish subtitles every 4 seconds that
    name the second they're at. Has trickplay frames.
  - `Movies/HEVC Test`, `Hi10 Test` (10-bit H.264), `AVI Test` (MPEG-4 Part 2 and MP3),
    `VP9 Test` (WebM) and `AV1 Test`: 30-second clips for the browser codec checks. New files
    only stream once a scan has added them (`api.scanMedia()`, or Scan Media in the web UI).
  - `Movies/Sparse Keyframes`: 2 minutes of H.264 with a keyframe every 10 seconds and AC-3
    audio (so it plays as a copy): for short jumps forward in a copied video.
  - `Movies/AVI BFrame Test`: 30 seconds of H.264 with B-frames in AVI, whose packets have no
    pts (framecrc prints -9223372036854775808): `SeekPoint` reads the dts instead.
  - `TV Shows/Example Show/Season 1/`: four episodes. S01E01 is a 60-second clip; S01E02 is empty;
    S01E03 and S01E04 are 8-second clips with their episode drawn on them (for going on to the
    next episode).
- Emulators: `emulator-5554` is Android TV (AVD `Television_1080p_API_34`) and `emulator-5556`
  is a phone (AVD `Medium_Phone_API_35`). `Android/tools/start-emulators.sh` starts whichever
  isn't running and waits for boot. The adb binary is `~/Android/Sdk/platform-tools/adb` (not
  on PATH). Both emulators already have the app signed in to the test server as `e2e_admin`.
- Helper scripts in `Android/tools/` (save screenshots in your scratchpad, then view them with
  Read):
  - `tv-keys.sh OUT.png DPAD_DOWN DPAD_CENTER …` presses keys on the TV, then screenshots.
    `HOLD_CENTER` is a real 1.5s hold of OK (`input keyevent --longpress` is too short).
  - `phone.sh texts` lists what's on the phone screen. `phone.sh tapt "Settings"` taps an
    element by its text or content description. `phone.sh tap X Y`, `phone.sh swipe …` and
    `phone.sh shot OUT.png` do the rest.
- Typical Android test loop:
  1. `cd Android && ./gradlew :app:assembleDebug`.
  2. `adb -s emulator-5554 install -r app/build/outputs/apk/debug/app-debug.apk` (and 5556).
  3. `adb shell am force-stop com.vargon.weirdstreamer`, then
     `am start -n com.vargon.weirdstreamer/.MainActivity`.
  4. Navigate and screenshot.

  Install debug builds on emulators only.
- Navigation tips:
  - The app opens on the source list (WeirdStreamer servers first, then SMB shares). On the TV,
    focus starts on the first server tile, and the test server (`http://10.0.2.2:9100`) is the
    second: DPAD_RIGHT, DPAD_CENTER. On the phone, `phone.sh tapt "http://10.0.2.2:9100"`.
  - In the TV player, DPAD_DOWN (with controls hidden) opens the toolbar. Its buttons are
    Transcode, Zoom, Speed, Fill+, Fill−, Aspect, Subtitles, Rotate (and Disc), and it remembers
    the last button focused, so press DPAD_LEFT several times before counting.
  - If a screenshot shows a Google TV "Set up" screen or the launcher, the app isn't in front.
    Press BACK and `am start` again, and check
    `adb shell dumpsys activity activities | grep topResumedActivity`.
  - Long chains of key presses without screenshots go wrong easily. Screenshot after each few
    steps.
- Web UI: use the browser pane at http://localhost:9100 and sign in as `e2e_admin` (password in
  `Server/e2e/pw`; type it yourself, don't print it). `window.api` and the app.js functions are
  available from the page's JavaScript for setting up state, such as
  `api.reportPlayback(path, seconds, true)` to create a resume point.
- To create test media, use ffmpeg test patterns. For example, `testsrc` plus `sine` with extra
  `-i subs.srt` inputs and `-metadata:s:s:N language=…` builds an MKV with many subtitle tracks.
  Put new files under `Server/e2e/sample/Movies/<Folder>/`.

## Release builds

`./release.sh` (repo root, on the build machine: it has the signing key) checks the tools, runs the
server tests, then builds `dist/<version>/`, the assets for a GitHub Release: the server package
(`weirdstreamer-server-<version>-linux-<arch>.tar.gz`: `bin/server`, `bin/cli`, the web UI with this
APK to download, `setup.sh`, `config.example.yml`, LICENSE and VERSION; needs only ffmpeg where it
runs), the APK and `SHA256SUMS`. It also publishes the APK to the web download links (`--no-web`
doesn't). The version is the app's (`0.0.1.<commit count>`). It refuses uncommitted changes
(`--allow-dirty` builds them as `<version>-dirty`) and installs on no device. Artifacts stay out of git.
`--publish` then tags the commit `v<version>`, pushes the tag to the `github` remote and creates the
GitHub Release with the three files (`gh`, signed in; checked before building; it never replaces a
release, nor moves a tag that's on another commit).

The package's `setup.sh` (`Server/package/setup.sh`) asks only for the name, port and HTTPS and
writes config.yml through `cli config init`, can create the admin user and install a systemd service
(`setup.sh service`, `setup.sh remove-service`: `bin/server` as the user who ran setup, from the
package's folder, after network mounts), then prints the web UI's address and offers to start the
server and open it. Libraries and the TMDB token are set in Settings: while there are no libraries,
an admin's sign-in opens Settings at Media Paths (`openLibrarySettings` in `app.js`).

## Deploying the Android app

`Android/deploy-release.sh` builds the R8 release APK and installs it only on the destinations
given: `--emulators` (the running emulators, with a launch-and-crash check; emulators go first and
a crash stops the rest), one flag per real device in `Android/devices.local` (git-ignored, one line
each: `flag|display name|model prefix`; `--help` lists them), and `--web`: copies it to `Server/web/downloads/WeirdStreamer.apk`
(git-ignored), which the web UI's sign-in page and Settings (My Account) link to for downloading;
the links hide when there's no APK. With none it builds nothing. `--no-build` reuses the built APK. A
destination that isn't connected, or a failed install, makes it exit non-zero.

- Versions: `versionCode` (the build number) is the git commit count, and `versionName` is a
  hand-set version plus the build number (`0.0.1.254`), in `app/build.gradle.kts`. `--web` also
  writes `WeirdStreamer.json` (version, build, size, date), which the download links show.
- Real devices must get release builds. Debug builds are visibly sluggish on the TV box.
- R8 needs keep rules (`Android/app/proguard-rules.pro`) for anything Gson stores by field name
  or created by reflection. Missing ones once crashed release builds at startup (WorkManager's
  Room database) and would have scrambled the saved downloads list.

## How things work (where it matters for the TODOs)

- **Streaming:** `GET /api/media/stream?path=<base64 path>`. Query options:
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
  - Discs (`.iso`) are always transcoded, per title (`title`, `angle`, `audio`, `subtitle`
    params).
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
    stretched to a cropped video. DVD subtitles are still burned in with `subtitle=N`. Track choices are kept per video in `localStorage`. Subtitles are off unless
    the file has forced ones, as on Android.
  - The seek bar shows trickplay frames (made by `cmd/cli trickplay`) on hover.
  - `startPlaybackReports` sends progress to `POST /api/streams/progress`. Resume and
    "Play from start" go through `playVideo`'s `startAt`.
  - Icons come from `MATERIAL_ICONS`, `iconSvg(name)` and `decorateIconButtons`.
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
  - Subtitles: `onTracksChanged` collects `C.TRACK_TYPE_TEXT` formats and labels them
    (`subtitleLabels`). `selectSubtitleTrack(index)` sets a `TrackSelectionOverride`, and
    `SubtitleMemory` remembers the choice per video. **Audio track selection should copy this
    pattern** with `C.TRACK_TYPE_AUDIO`, and get its own dialog row in the player toolbar
    (`PlayerZoomToolbar` buttons 0–8).
- **Phone player** (`PhoneVideoPlayer.kt`, ExoPlayer):
  - Same transcode menu (a dropdown) and fallback to Compatibility (Auto) as the TV.
  - Touch handling today: the root `Box` has `pointerInput { detectTransformGestures }` for
    pinch-zoom and pan, and `clickable` to show or hide the controls. Swipe gestures (seek,
    brightness, volume) must not break pinch-zoom: handle single-finger drags separately.
    Brightness can go through `window.attributes.screenBrightness`, volume through
    `AudioManager`.
  - It has no subtitle or audio track UI yet.
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
  `ThemedControls.kt` draw with it. On the phone `surface` is transparent, so lists and bars show
  their box. Android also has Light, Dark and Auto Light/Dark (`AppTheme`); the web's old purple
  default is now Purple.
- **Recently Added:** `showNewAdditions` (`config.yml` or Settings) is `on`, `libraryOnly` or `off`
  (the default): the last 10 videos show under Continue Watching on the library's own Library and
  top folder pages, and with `on` they also blend into the home screen's row
  (`GET /api/library/new`, with or without `lib`). The `media_added` table dates each file once:
  files already known by their time on disk (`FillMediaAdded`, at startup), new ones when the
  server first saves them. The newest day comes first, and a day's videos in viewing order (show,
  season, episode). `media_items`' `created_at` is no use for this: every save replaces the
  row. Clients turn `added_at` into "Added 2 days ago" on their own calendar.
- **Update banners:** the apps run `AppUpdater`'s check when they open, on another server and every
  6 hours (`UpdateChecks`), and `UpdateBanner` points to Settings: on the TV only to read, gone
  after 15 seconds; on the phone a tap opens Settings and X closes it for that version. The web
  has no version number: `checkForNewWeb` compares its files' dates and sizes with those at page
  load, every 5 minutes and on returning to the tab, so saving a file in `Server/web` shows the
  banner in open pages. A click reloads.
- **Progress and resume:** players report position every 5s, on pause and at the end. The
  server decides "watched" (the last 2 minutes of videos 20 minutes or longer; the end of shorter
  ones). `MediaViewModel.playVideo(path, fromStart)` sets `playbackStartMs`.

## Notes on the TODO items

- **Audio track selection on TV, phone and web:**
  - Android: ExoPlayer exposes audio tracks like text tracks (see the TV subtitle pattern above).
  - Web: done (the player's Audio menu, `audio=N`).
- **Phone gestures:** see Phone player above. Typical behaviour:
  - A horizontal drag seeks, with a "+0:30 / 12:34" overlay, applied on release. For transcoded
    streams, seek through `seekPlayer`.
  - A vertical drag on the left half changes brightness; on the right half, volume. Each shows a
    level indicator.
  - Double-tapping the sides for ±10s is common too.
