# WeirdStreamer for iPhone

The iPhone app for a WeirdStreamer server, built to work like the Android phone app (its screens,
words and behaviour are ported from `Android/…/ui/components/Phone*.kt` and `MediaViewModel.kt`).
It is **not** for the App Store: it's sideloaded with AltStore and refreshed through Remote AltStore
or LocalDevVPN.

## What's there (as on Android)

- **Source list:** each server in its own box (its card, then its accounts as round buttons and
  Add user). + adds a server by address alone: without a scheme or port it tries 9000, 443, 80 and
  8080, https before http, and saves the first WeirdStreamer server that answers (its `/api/info`
  name shows on the card). Add user signs an account in; tapping an account opens the server as it,
  holding one signs it out (it stays, marked Signed out); an expired sign-in asks again.
- **Browsing:** the bar (Home, the folder's title, list → grid → library view, ⋮ with Show and
  Settings) under the wordmark and "Server · account"; Continue Watching and Recently Added on the
  server's front page and each library's own page; folder rows and tiles with thumbnails, size,
  length and time left; the A–Z strip; Resume or Play from start for a partly watched video; hold a
  video for Play from start and Mark watched. A swipe in from the left edge is Android's Back.
- **Library view:** posters with progress, Unmatched files, title pages (backdrop, poster, details,
  cast, files with Resume / Play / watched; seasons with their rings, watched buttons for a season
  and the whole show, Auto Skip, episode rows and their details).
- **Player:** Android's controls (Back and title, Transcode menu with its nine modes and what the
  stream does, Rotate; −10 s / play / +30 s and frame steps; trickplay frames, aspect ratio, speed,
  seek bar), pinch to zoom, Skip Intro / Skip Credits, the next episode at the end, progress every
  5 seconds, picture in picture when you go home. It picks Compatibility (Auto) itself for files
  iOS can't play (MKV, AVI, other codecs) and steps down on errors as Android does. DVD images
  (.iso) are always transcoded, a title at a time; the Disc button (Android's disc sheet) picks
  chapters, angles, audio and subtitle tracks and other titles (picture subtitles burned in by the
  server), as a panel in the player; a title the server can't convert says so.
- **Audio:** files with more than one audio track get an Audio menu (the choice remembered per
  video). AVPlayer plays only the original's default track, so another track comes from the server
  (Compatibility (Auto) with `audio=N`, as on the web).
- **Subtitles:** the CC menu lists the file's tracks (Off unless it has forced ones, the choice
  remembered per video). Text tracks come from the server as WebVTT in 2-minute windows, fetched
  ahead, and PGS as .sup decoded on the phone and drawn over the video at the disc's scale; other
  picture subtitles are burned in by a transcode. ASS/SSA tracks and .ass/.ssa files are drawn
  styled by libass (`Views/AssSubtitleView.swift`, `iOS/native/assrender.c`), as on the web and
  Android: the server's `format=ass` windows (the first one's header, every window's lines once),
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
  Compressed (sizes and resolution as on Android; the last two are server encodes the app waits
  for), "Download this folder" in ⋮, and Settings › Downloads (Wi-Fi only, Download ahead, storage,
  play or remove). Downloads use a background `URLSession`, so they continue while the app is
  suspended. Progress played from a download is kept locally and sent when the server is reachable.
  Compatibility keeps what the iPhone plays: an MKV of H.264 and AAC is copied into MP4 (the server
  takes `keep_video` and `keep_audio` together for a file that isn't MP4 already).
  Each download's extras are saved in `Documents/DownloadExtras/<id>/` for playing offline: its
  subtitles (text as WebVTT, ASS/SSA whole as .ass with the video's fonts, PGS as .sup; this phone's languages, forced ones and files beside
  the video), intro and credits (Skip Credits goes on to the next downloaded episode), trickplay
  frames, codecs report and thumbnail; again every 12 hours and when a subtitle is added. With the
  server unreachable, its error has a Downloads button. Download ahead (as Android's): two minutes
  into an episode, the next 3 to 5 (as many as download in about an hour at the speed downloads
  have come at), leaving 500 MB or 5% of the phone free; those before the episode playing go.
- **Settings:** App (show hidden files, clear the image cache, backup, the saved servers with their
  users and Remove), Themes (all 21 of Android's, as a grid of tiles), Downloads, Account (change
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

## Constraints (free Apple ID, no developer program)

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
(FreeType, FriBiDi, HarfBuzz; the versions and checksums Android pins) into `iOS/native/build/`
(git-ignored), which the project links: once, and again after changing `iOS/native/assrender.c`.
It needs cmake and pkg-config (Homebrew). Simulators are Apple silicon only.

```bash
iOS/native/build-libass.sh
```

From the command line (simulator):

```bash
xcodebuild -project WeirdStreamer.xcodeproj -scheme WeirdStreamer \
  -destination 'generic/platform=iOS Simulator' CODE_SIGN_IDENTITY=- build
```

Build with signing (ad hoc is enough): an unsigned build (`CODE_SIGNING_ALLOWED=NO`) can't use the
Keychain, so its sign-ins don't survive a relaunch. AltStore signs the app it installs.
The deployment target is iOS 17 (SwiftData's minimum).

The e2e server (`Server/e2e`, http://localhost:9100) has an `e2e_ios` account for the simulator.

## Certificates

Like the Android app, a server you add is trusted with whatever certificate it has (home servers
often use self-signed ones), for the app's own requests and for transcodes (which go through the
relay). An **original** file streams straight to AVPlayer, which checks the certificate itself: a
self-signed server needs its certificate installed on the phone (Settings › General › About ›
Certificate Trust Settings), or plain HTTP. iOS refuses self-signed certificates valid for more
than 825 days.

## Not yet ported from Android

- SMB shares: iOS has no SMB client, so this needs a library (AMSMB2, libsmb2) or one written
  for the app.
- Phone gestures beyond pinch (Android doesn't have them yet either) and the update banner
  (AltStore handles updates).
- Tried against a Linux e2e server (whose ffmpeg reads DVDs): the Library view and title pages,
  and a real DVD image (titles, chapters, audio tracks, burned-in subtitles, resume). Not yet tried:
  Compressed downloads, Download ahead, PGS subtitles, backup and
  restore, the Light theme, and anything on a real iPhone.
