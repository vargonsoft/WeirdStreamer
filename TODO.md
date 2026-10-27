# TODO

Requests saved for later, newest last.

## Playback

- [x] **Web: a 4K film plays with no audio.** Find out why (likely an audio codec the browser
      can't decode, such as DTS, TrueHD or E-AC-3) and fix it.
- [ ] **Audio track selection on TV, phone and web.** Choose among a file's audio tracks
      (languages, commentary, formats) the way subtitles can be chosen on TV. (Web: done.)
- [x] **Web: transcode automatically when the browser can't play a file**, as Android does when
      the device can't decode it (check the video and audio codecs and the container against what
      the browser supports, or fall back when playback fails).
- [x] **Web: a custom player for all videos** that looks and works more like the TV's player,
      replacing the browser's built-in controls (discs already use custom controls).
- [ ] **Phone player gestures**, as in popular video players:
  - [ ] Swipe left or right to seek back or forward.
  - [ ] Swipe up or down on the left side for brightness.
  - [ ] Swipe up or down on the right side for volume.
- [ ] **Styled ASS/SSA subtitles.** Subtitles now come from the server as WebVTT, which keeps only
      italics, bold and underline, so ASS/SSA tracks lose their positions, colours and sizes. On
      the TV and phone, when playing the original file, let the player draw a video's own ASS/SSA
      tracks itself (styled) as the default for those tracks, falling back to the server's plain
      version on transcodes (which carry no subtitles). Open questions:
  - Also offer a plain version of each ASS track in the Subtitles box?
  - Style ASS files beside the video (`Movie.en.ass`) too, by loading them alongside the video?
  - Web: draw ASS styling with a JavaScript renderer (such as JASSUB, libass for the browser)?
