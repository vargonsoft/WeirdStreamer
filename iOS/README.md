# WeirdStreamer for iPhone

The iPhone and iPad app for a WeirdStreamer server. It isn't on the App Store: install it with
AltStore or SideStore (below), or build it from the source with your own Apple developer account.

## Installing with AltStore or SideStore

Each GitHub release has the app as an unsigned IPA and an AltStore source listing it. In AltStore (or
SideStore), add this source once (Browse › Sources, +):

```
https://github.com/vargonsoft/WeirdStreamer/releases/latest/download/altstore-source.json
```

Then install Weird Streamer from it; each new release shows as an update. AltStore signs the app with
your Apple ID. With a free one, it must refresh the app every 7 days (AltServer on your computer, or
SideStore on the phone) and at most 3 sideloaded apps are active. The app needs nothing a free Apple
ID can't give (see Constraints). Keep the app and the server at the same release: they change
together.

## What's there

- **Source list:** each server in its own box (its card, then its accounts as round buttons and
  Add user). + adds a server by address alone: without a scheme or port it tries 9000, 443, 80 and
  8080, https before http, and saves the first WeirdStreamer server that answers (its `/api/info`
  name shows on the card). Add user signs an account in; tapping an account opens the server as it,
  holding one signs it out (it stays, marked Signed out); an expired sign-in asks again.
- **Browsing:** the bar (Home, the folder's title, list → grid → library view, ⋮ with Show and
  Settings) under the wordmark and "Server · account"; Continue Watching and Recently Added on the
  server's front page and each library's own page; folder rows and tiles with thumbnails, size,
  length and time left; the A–Z strip; Resume or Play from start for a partly watched video; hold a
  video for Play from start and Mark watched. A swipe in from the left edge goes back. In a folder,
  an administrator's ⋮ also has Regenerate thumbnails (`POST /api/media/regenerate-thumbnails`,
  after a confirmation): the server makes them in the background and reports each one on
  `/api/media/thumbnail-updates`, which the app listens to while a server is open, so tiles change
  as they're made.
- **Library view:** posters with progress, Unmatched files, title pages (backdrop, poster, details,
  cast, files with Resume / Play / watched; seasons with their rings, watched buttons for a season
  and the whole show, Auto Skip, episode rows and their details). For admins, Fix match (`MatchSheet.swift`):
  from a title page, a file or episode row, an Unmatched file, or holding a video or a title's folder.
  Collections (movie libraries, after Library view): collections admins make (a name, a description, movies by
  release date, a collage of the first four posters; admins create, edit, delete and add from a movie's page), then
  TMDB Collections: TMDB's collections, then a collection's films, missing ones dimmed.
- **Player:** controls (Back and title, Transcode menu with its nine modes and what the
  stream does, Rotate; −10 s / play / +30 s and frame steps; trickplay frames, aspect ratio, speed,
  seek bar), pinch to zoom, Skip Intro / Skip Credits, the next episode at the end, progress every
  5 seconds, picture in picture when you go home. It picks Compatibility (Auto) itself for files
  iOS can't play (MKV, AVI, other codecs) and steps down on errors. DVD images
  (.iso) are always transcoded, a title at a time; the Disc button picks
  chapters, angles, audio and subtitle tracks and other titles (picture subtitles burned in by the
  server), as a panel in the player; a title the server can't convert says so.
- **Audio:** files with more than one audio track get an Audio menu (the choice remembered per
  video). AVPlayer plays only the original's default track, so another track comes from the server
  (Compatibility (Auto) with `audio=N`, as on the web).
- **Subtitles:** the CC menu lists the file's tracks (Off unless it has forced ones, the choice
  remembered per video). Text tracks come from the server as WebVTT in 2-minute windows, fetched
  ahead, and PGS as .sup decoded on the phone and drawn over the video at the disc's scale; other
  picture subtitles are burned in by a transcode. ASS/SSA tracks and .ass/.ssa files are drawn
  styled by libass (`Views/AssSubtitleView.swift`, `iOS/native/assrender.c`), as on the web: the server's `format=ass` windows (the first one's header, every window's lines once),
  the video's embedded fonts (`/api/media/font`), the system's through CoreText, and the server's CJK
  subset (`POST /api/media/subtitle-font`) for the Chinese, Japanese and Korean characters a script
  has, since the system's simplified Chinese font can't be read by libass. Tested on the simulator
  (streamed and offline) but not yet on a real iPhone. On the picture
  through zoom and rotation, redrawn only when libass reports a change (also while paused).
  Downloads save them whole (`<track>.ass`) with the video's fonts (`font-<index>-<name>`) and draw
  them the same way offline; one saved before as WebVTT shows plain lines until its next refresh. Timing ("Already said" / "Not said yet", saved to
  the server) and Search online (OpenSubtitles through the server) are there too. Settings › App ›
  Subtitle style: the iPhone's own caption style (Accessibility › Subtitles & Captioning) or a
  custom one (box or outline, colors and opacities, font, size), with a preview.
- **Photos and PDFs:** the image viewer pages through a folder's photos, with pinch zoom; motion
  photos (Samsung, Google) and Apple Live Photos loop their clip. PDFs open in PDFKit with
  Previous / page / Next and a page grid.
- **Downloads:** the download button on videos and episodes, with Original, Compatibility or
  Compressed (sizes and resolutions as the server
  sets them; the last two are server encodes the app waits
  for), "Download this folder" in ⋮, and Settings › Downloads (Wi-Fi only, Download ahead, storage,
  play or remove). Downloads use a background `URLSession`, so they continue while the app is
  suspended. Progress played from a download is kept locally and sent when the server is reachable.
  Compatibility keeps what the iPhone plays: an MKV of H.264 and AAC is copied into MP4 (the server
  takes `keep_video` and `keep_audio` together for a file that isn't MP4 already).
  Each download's extras are saved in `Documents/DownloadExtras/<id>/` for playing offline: its
  subtitles (text as WebVTT, ASS/SSA whole as .ass with the video's fonts, PGS as .sup; this phone's languages, forced ones and files beside
  the video), intro and credits (Skip Credits goes on to the next downloaded episode), trickplay
  frames, codecs report and thumbnail; again every 12 hours and when a subtitle is added. With the
  server unreachable, its error has a Downloads button. Download ahead: two minutes
  into an episode, the next 3 to 5 (as many as download in about an hour at the speed downloads
  have come at), leaving 500 MB or 5% of the phone free; those before the episode playing go.
- **Settings:** App (show hidden files, rewind 5 seconds when resuming, clear the image cache, backup, the saved servers with their
  users and Remove), Themes (all 21 themes, as a grid of tiles), Downloads, Account (change
  password), and for admins Server (groups, media paths, server settings, Scan media), Users,
  Streams and Jobs.

## Transcodes on iOS: the HLS relay

The server sends a transcode as fragmented MP4 of unknown length. ExoPlayer plays that as it comes;
AVPlayer can't (a progressive stream needs a known length). `Networking/HLSRelay.swift` relays it
as HLS from a small HTTP server on 127.0.0.1: it reads the stream, cuts it at its fragments (each
starts on a keyframe), rewrites each fragment's offsets to its own start (AVPlayer refuses ffmpeg's
file offsets in HLS), keeps only the video and audio tracks (AVPlayer refuses the whole stream
when it carries the chapters track ffmpeg writes for a DVD or an MKV with chapters), and serves a
growing EVENT playlist. Segments go to a temporary folder in
Caches, deleted when the player closes and at the next launch. Originals still stream directly.

## Constraints (no paid entitlements)

- **No paid entitlements:** no push notifications, iCloud or CloudKit, Apple Pay, App Groups, or
  Sign in with Apple.
- **Local-first data:** servers, accounts, view choices and progress are a SwiftData store on the
  phone; sign-in tokens are in the Keychain. Settings › Backup exports and imports them as a JSON
  file through the Files app (never passwords or tokens).
- **Local notifications only:** the app schedules none yet; any must use `UserNotifications`.
- **Expiry resilience:** if the store can't be opened (a damaged file, a write cut short, an older
  store that won't migrate after the app sat expired), it's moved into `Recovery-<time>/` (the last
  3 are kept) and a fresh one starts; servers and accounts come back from the Keychain. Progress
  that couldn't reach the server is queued and sent at the next chance (`/api/playback/sync`).
- **Sign-in** is the server's username and password over HTTP(S).

## Building and testing

Open `WeirdStreamer.xcodeproj` in Xcode 26 or later. First build libass and what it needs
(FreeType, FriBiDi, HarfBuzz; their pinned versions and checksums) into `iOS/native/build/`
(git-ignored), which the project links: once, and again after changing `iOS/native/assrender.c`.
It needs cmake and pkg-config (Homebrew). Simulators are Apple silicon only.

```bash
iOS/native/build-libass.sh
```

From the command line (simulator):

```bash
xcodebuild -project iOS/WeirdStreamer/WeirdStreamer.xcodeproj -scheme WeirdStreamer \
  -destination 'generic/platform=iOS Simulator' CODE_SIGN_IDENTITY='-' build
```

Build with signing (ad hoc is enough): an unsigned build (`CODE_SIGNING_ALLOWED=NO`) can't use the
Keychain, so its sign-ins don't survive a relaunch.
For a device build in Xcode, put your team in `WeirdStreamer/Signing.local.xcconfig`
(`DEVELOPMENT_TEAM = <team ID>`, git-ignored), which `Signing.xcconfig` includes; it isn't needed
for the simulator.
The deployment target is iOS 17 (SwiftData's minimum).

The e2e server (`Server/e2e`, http://localhost:9100) has an `e2e_ios` account for the simulator.

### Unit tests

`WeirdStreamerTests` (Swift Testing, run by the shared `WeirdStreamer` scheme) covers the logic
that doesn't need a server or a screen: the JSON the app sends and reads against the Go server's
field names (a body with the wrong names is accepted and silently ignored, so it's caught here),
the thumbnail-updates event stream, addresses and certificate keys, download languages and tracks,
what plays offline, hardware encoder choices, WebVTT, subtitle and audio labels, CJK characters, PGS
decoding (a hand-made .sup), `OriginalLoader`'s byte ranges, Up next, the A–Z strip, motion photos
and the text helpers. New files in `WeirdStreamerTests/` join the target by themselves.

```bash
xcodebuild test -project iOS/WeirdStreamer/WeirdStreamer.xcodeproj -scheme WeirdStreamer \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' -parallel-testing-enabled NO CODE_SIGN_IDENTITY='-'
```

### Building from another machine (Linux)

`iOS/remote/mac-ios`, run in a checkout on another machine, has the Mac build or test that
checkout as it is, over SSH: what's on origin plus everything there that isn't (commits not
pushed, changes not committed, new files) as one patch. So iPhone changes made on Linux can be
compiled and tested before they're committed. On the Mac, `iOS/remote/ios-remote.sh` does the work in
a clone of its own (`~/Library/Caches/WeirdStreamer-remote`, `$IOS_REMOTE_DIR`), never in a
checkout anyone edits, one run at a time, testing on a simulator of its own ("WeirdStreamer Remote").

```bash
iOS/remote/mac-ios test            # the unit tests
iOS/remote/mac-ios build           # does it compile (simulator)?
iOS/remote/mac-ios ipa             # WeirdStreamer.ipa here, for AltStore (-o to name it)
iOS/remote/mac-ios check           # test, then ipa
iOS/remote/mac-ios log             # the Mac's last full xcodebuild log
```

It needs SSH from that machine to the Mac without a password (Remote Login on, the machine's key
in the Mac's `~/.ssh/authorized_keys`), and `iOS/remote/remote.local` (git-ignored) there:

```bash
MAC_HOST=user@mac.local
MAC_SCRIPT=/path/to/WeirdStreamer/iOS/remote/ios-remote.sh
```

The IPA is unsigned: AltStore signs it with the Apple ID installing it. Its version is Android's
(`MARKETING_VERSION`, 0.0.1) and its build number the commit count, as Android's: Settings shows
`0.0.1.<build>`. A build with a patch is named `WeirdStreamer-<count>-patched.ipa` on the Mac.
`release.sh` builds a release's IPA from the release commit (`mac-ios ipa --ref`, so it must be on
origin) and writes its AltStore source with `iOS/altstore/make-source.py`, which reads the version,
build, minimum iOS and privacy strings from the IPA itself (AltStore refuses an IPA that doesn't
match its source).
The Mac runs its own checkout's `ios-remote.sh`, so a change to that script reaches it when that
checkout is pulled.

## Certificates

A server you add is trusted with the certificate it has when it's added (home servers often use
self-signed ones), and only that one: its SHA-256 is kept (`TrustPolicy` in `ServerClient.swift`;
a server saved before this keeps the one it shows at its next connection), and a different
certificate is refused with a message to remove the server and add it again. That covers the app's
own requests, transcodes (the relay's session), downloads, and originals and Live Photo clips:
AVPlayer checks a certificate itself, so those are given a made-up `wsoriginal://` address and
`OriginalLoader` fetches each byte range AVPlayer asks for through the app's session. Plain-HTTP
servers need none of this, and nothing has to be installed on the phone.

## Not yet included

- SMB shares: iOS has no SMB client, so this needs a library (AMSMB2, libsmb2) or one written
  for the app.
- The update banner (you rebuild the app to update it).
- Tried against a Linux e2e server (whose ffmpeg reads DVDs): the Library view and title pages,
  and a real DVD image (titles, chapters, audio tracks, burned-in subtitles, resume). Not yet tried:
  Compressed downloads, Download ahead, PGS subtitles, backup and
  restore, the Light theme, and anything on a real iPhone.
