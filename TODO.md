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

- [ ] **GPU encoding for downloads.** Streaming transcodes can use the GPU (Settings › Streaming ›
      Hardware encoding); download encodes (Compatibility and Compressed) still use x264 on the
      CPU. Compatibility aims for full quality (CRF 18), where older NVENC falls short of x264 at the
      same size: maybe a setting, or GPU for Compressed only.

## DVD discs

- [ ] **Send a disc image's DVD subtitles beside the stream instead of burning them in.** Files'
      DVD subtitles already go to the players as PGS (`media/dvdsub.go`); discs (.iso) still burn
      theirs into the transcode. Notes from weighing it:
  - The hard part: a disc title can be several chained parts, trimmed and joined, with angles, so
    the server would have to pull the subtitle track out for exactly that timeline
    (`DiscTranscodeArgs`' segments and trims) and keep it in sync. Discs are always transcoded
    anyway, so burn-in costs them nothing extra.
  - The palette is in the disc's IFO tables (per program chain), not in an .idx header: ffmpeg's
    dvdvideo demuxer gives it as the stream's extradata.
  - Lost by drawing instead of burning in: the iPhone's picture-in-picture and AirPlay show only the
    video, so drawn subtitles disappear there (as text and PGS already do); burned-in ones show.
  - Gained: switching subtitles without restarting the stream, forced-only lines (DVD marks them one
    by one), and subtitles in downloads of a disc's titles if those ever exist.

## Platforms

- [ ] **Server packages for more platforms:** Linux arm64, macOS (Intel and Apple Silicon) and
      Windows x64, beside today's Linux amd64. Feasibility is in CLAUDE.md's notes ("More
      server platforms").
