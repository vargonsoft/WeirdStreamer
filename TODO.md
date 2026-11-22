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

## Platforms

- [ ] **Server packages for more platforms:** Linux arm64, macOS (Intel and Apple Silicon) and
      Windows x64, beside today's Linux amd64. Feasibility is in CLAUDE.md's notes ("More
      server platforms").
