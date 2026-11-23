# TODO

Requests saved for later, newest last.

## Playback

- [x] **Web: a 4K film with E-AC-3 audio plays with no audio.** Find out why (likely an audio codec the browser
      can't decode, such as DTS, TrueHD or E-AC-3) and fix it.
- [x] **Audio track selection on TV, phone and web.** Choose among a file's audio tracks
      (languages, commentary, formats) the way subtitles can be chosen on TV. Also on the iPhone.
- [x] **Web: transcode automatically when the browser can't play a file**, as Android does when
      the device can't decode it (check the video and audio codecs and the container against what
      the browser supports, or fall back when playback fails).
- [x] **Web: a custom player for all videos** that looks and works more like the TV's player,
      replacing the browser's built-in controls (discs already use custom controls).
- [x] **Phone player gestures**, as in popular video players:
  - [x] Swipe left or right to seek back or forward.
  - [x] Swipe up or down on the left side for brightness.
  - [x] Swipe up or down on the right side for volume.
- [x] **Styled ASS/SSA subtitles.** Drawn by libass everywhere: JASSUB on the web, native on the
      Android TV and phone and the iPhone, from the server's `format=ass` script and the video's
      embedded fonts, on originals and transcodes alike, and files beside the video too. The
      "Styled subtitles" job copies the tracks out after scans, so a start or jump shows lines
      already on screen. Phone downloads keep them styled offline.

## Transcoding

- [x] **GPU encoding for downloads.** Download encodes (Compatibility and Compressed) use the GPU
      Settings chose, sharing its limit with streams, and x264 when it's full or fails. Measured
      first: at the same size the GPUs matched x264 in both modes, once x264's CRF was translated
      per encoder (which also fixed streaming transcodes' bitrates).

## DVD discs

- [x] **Send a disc image's DVD subtitles beside the stream instead of burning them in.** Every
      player draws them as PGS over the transcode (`media/dvddiscsub.go`), switching without a
      restart, and with subtitles off shows a DVD track's forced lines (files' too). Drawn
      subtitles don't show in the iPhone's picture-in-picture or AirPlay, as for files.
- [ ] **Disc seeks land up to 15 s from where they say.** ffmpeg's dvdvideo demuxer lands a seek
      near its target and labels what follows as the target, so a disc stream started at 3000 s can
      show 3002 s (on the sample disc, between 2 and 14 s off). The subtitles follow the same labels
      (see CLAUDE.md), but the clock, chapter jumps and resume points are off by as much. Ideas:
      seek to the chapter before and trim (accurate, but decodes up to a chapter's length), or read
      the landing's true time from the disc's navigation packets.
- [ ] **Subtitles in downloads of a disc's titles.** A disc download (its main title as MP4) saves
      no subtitle tracks yet; the server now reads them (`DiscSubtitlesAsPGS` with from 0).

## Platforms

- [x] **Server packages for Linux arm64 and macOS (Apple Silicon)**, beside Linux amd64:
      `release.sh` builds all three (`--platforms`); the Mac's setup builds ffmpeg and installs a
      launchd service. Intel Macs are left out (current macOS runs only on Apple Silicon).
- [ ] **Server package for Windows x64.** Feasibility is in CLAUDE.md's notes ("More server
      platforms").
