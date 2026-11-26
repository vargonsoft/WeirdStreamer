# The Guide: live pseudo-channels

What we're working toward: a cable or satellite style TV guide made from the libraries. Its channels play
the libraries' shows and movies on a schedule, and you can watch only what's on now. The programming
works the way 80s and 90s TV did, not the way modern TV does.

Status: on the `guide` branch, phase 1 (the server: channels, the planner, filler, the API), phase 2
(the web: `js/guide.js`), phases 3 and 4 (Android TV and phone) and phase 5 (the iPhone; compiled and
unit-tested, not yet tried on a simulator or phone) are built. Voice-overs are next. Decisions are marked as settled; open questions are at the end.

## Decisions (settled)

- **Where it opens.** A new row above the folders, only on the Libraries screen, holding (for now) one
  button: **Guide**. If no library is in the Guide, there's no row and no button.
- **Library setting "Show in Guide":** On (Home Screen and Library), Library Only, or Off, like Continue
  Watching and Recently Added. **Off by default.** Library Only puts a Guide button on that library's own
  page, showing only its channels.
- **Library setting "Use for Guide Filler"** (off by default): the library's short videos can fill the gaps
  between programs. The library stays a normal library and its videos show everywhere as usual. The Guide
  only borrows from it, picking what suits the time of day.
- **Channels have our own names and logos.** Never the real networks' names or logos. TMDB's network is
  used only to group similar shows.
- **Live only.** A channel plays what's on now, from the current second. There's no seeking and **no
  pause**: the player's pause control mutes instead. Leaving the Guide (or the full-screen channel) stops it.
- **Progress isn't recorded.** Watching a channel adds no resume point and nothing on Continue Watching.
  You can still **mark the current program watched** from its info.
- **Credits stay in.** Programs play whole. Filler goes after the credits.
- **Prime time is a server setting:** 7–10 pm or 8–11 pm, with the news hour after.
- **News, for now:** no news source. The news hour is a late-night block (reruns or a late movie). A real
  source (an antenna tuner such as an HDHomeRun, or a stream address an admin enters) can come later.
- **The 80s and 90s feel is about how programming works.** It's not about which titles air, and not a
  visual theme. See the next section.

## How programming works (the 80s and 90s way)

- **No binge blocks.** A channel doesn't run the same show for hours. A show airs once in a day part, at
  most twice (an episode in the afternoon and again at night counts as two). Back-to-back episodes of one
  show are the exception, not the rule.
- **Prime time airs weekly.** Each show in prime time has a night and a time slot, and its next episode
  airs once a week: *Thursday 8:30* is that show's time. Episodes air in order.
  - **A show keeps its slot for one season** (its season's episodes, one a week). Then another series takes
    the slot for one of its seasons, like a mid-season replacement. After that the first show returns
    with its next season. Shows sharing a slot take turns this way, season by season.
  - A channel without enough shows to fill a week of prime time falls back to the next point, then to a
    movie.
  - Optional later touch: summer reruns (June to August) replay the season's episodes.
- **Daytime and late night are syndication.** These are reruns stripped across weekdays: the same show
  every weekday at 5:00, its next episode each day. Several such shows rotate through the afternoon.
- **Weekends differ.** Saturday morning is for kids (by rating), the afternoons get movies and
  long-form shows, and Sunday night has a movie.
- **Movie night.** Movie channels have a weekly feature slot in prime time. Other movies fill
  afternoons and late nights.
- **Times of day follow the ratings.** Kids' ratings (TV-Y, TV-G, G) run mornings and afternoons. TV-MA
  and R run only from the news hour on. Everything else runs anywhere.

## Channels

- Built from the libraries with Show in Guide on. Each user sees only the channels made from libraries
  their group can see. A channel mixing libraries shows a user only what they can see, and a channel
  left empty for them is hidden.
- **TV shows are grouped by network** (TMDB's): a network with enough shows (for example four or more,
  or enough episodes for a week without repeats) gets a channel of its own, under our own name.
  Shows from smaller networks go into **genre channels**: sitcoms, drama, sci-fi and fantasy,
  animation, kids, documentary and reality.
- **Movies get genre and decade channels:** action, family, horror, comedy, 80s movies, and so on.
- **Channel numbers stay put.** They're stored in a `guide_channels` table. Rebuilding the lineup
  reuses a channel's number, and new channels take the next free one. Later, an admin may rename,
  renumber or hide channels, and give them their own logos.
- **Names and logos.**
  - Genre channels are named for their genre. Network channels take the call sign of one of a pool of
    made-up stations (WKBT, KZXM…).
  - Every channel has its own logo in its own font, in one color as real stations' were: a symbol
    (a star, a bolt, a ringed planet, a bat…) or the number's holder, with the name beside or below
    it. The number is in a ring, square or rectangle, or cut out of a disc, block, bar or triangle.
  - The logos are artwork made once (`cmd/logoart`) with display fonts that aren't shipped. The server
    stamps each channel's number into its logo, and serves it as SVG or as PNG at any size, for the
    grid, the cards and filler.

## The schedule

- **Everyone sees the same schedule, and it's stored.** The server plans each channel a few days ahead
  into `guide_slots` (channel, start, end, the item, what kind it is: program, filler or bumper). So the
  guide doesn't shift on reload, and two people on one channel see the same frame. Planning continues
  from where it left off: each show remembers its next episode (per show and per slot type: weekly or
  strip).
- **Programs snap to the half hour.** A 22-minute episode takes a half-hour slot, a 44-minute one an
  hour, a 1h50 movie two hours. The rest of the slot is filler. File durations come from
  `media_items.duration`.
- **Rebuilding.** When libraries change (new episodes, a library turned on or off), only the time from a
  few hours ahead onward is re-planned. What's on now and soon never changes under a viewer.

## Filler

The gap after each program (after its credits), packed to the second, in this order:

1. **Filler libraries' short videos** (Use for Guide Filler): shorts, cartoons, music videos, trailers,
   a folder of commercials. Clips are chosen whose lengths add up to the gap, suited to the time of day
   (by rating), not repeated soon on the same channel. The last one may be cut short.
2. **Bumpers:** "Up next on Channel 7: *Title* at 8:30" with the poster, the channel's badge and a
   clock, or "We'll be right back". They fill whatever is left, so a gap is never empty. A bumper is a
   slot with no file: the player draws the card from `/api/guide/now`'s `next` (sharp at any size, no
   encoding). The voice-overs may make some of them videos later.
3. **Commercials are the admin's to add**, as a filler library. Two sources: the Internet Archive's
   *Classic TV Commercials* (about 8,500 user uploads, mostly 50s to 80s) and the *Prelinger Archives*.
   They're labeled public domain by their uploaders, but not checked, so the server neither bundles nor
   downloads them. A helper script that fetches a chosen set into a folder may come later.

Other free sources for filler libraries. All of them are downloaded into a folder once, not streamed
live, so a gap never waits on someone else's site:

- **NASA's videos:** US government work, public domain (except NASA's logos used as endorsements).
- **Blender's open films** (*Big Buck Bunny*, *Sintel*, *Tears of Steel*, *Spring*, *Charge*): CC BY,
  shown with their credit.
- **Wikimedia Commons:** public domain and CC videos, each file with its own license.
- **Public domain cartoons and newsreels** on the Internet Archive. Check each one, as with the
  commercials.
- **Stock footage** from Pexels or Pixabay (their own free licenses), for scenic interludes.
- **Music for bumpers:** Kevin MacLeod's (incompetech, CC BY), or the Free Music Archive's tracks by
  their licenses.
- **Generated segments**, made by the server and needing no source:
  - a "local weather" interlude from the US National Weather Service's free API (`api.weather.gov`,
    public domain), very 80s cable;
  - a "tonight on Channel 7" promo card.

## Voice-overs on the credits (later phase)

As old shows did, while the credits roll: "Stay tuned for *Title*, next on Channel 7. And at 9, *Title*."

- The text comes from the schedule: what's next and what's later tonight.
- **Piper** (free, local, offline) makes the audio. The server runs it as a command, like
  `subsetCommand`, so another synthesizer could stand in later.
- **Each channel has its own announcer:** one voice from a pool, picked by the channel's number and
  kept with it, so a channel always sounds the same. The pool, chosen by listening to samples:
  - `en_US-ryan-high`: male, deep and steady.
  - `en_US-hfc_female-medium`: female, lively.
  - More can be added later (`en_US-libritts_r-medium` has 904 speakers to pick from).
- **Pronunciation:** Piper's words come out right, but its stress can drift with the sentence ("next on
  Channel 7" put the stress on the wrong syllable of "Channel", where "Only on Channel 7" got it right).
  The script's lines are written and tested to read well, and Piper's `[[ phonemes ]]` syntax can fix a
  word where wording doesn't.
  - Each voice has its own fixes, with the phonemes picked by listening. For "Channel" in "next on
    Channel 7", the "a" is lengthened:
    - Ryan: `[[ tʃˈæːnəl ]]`;
    - HFC Female: `[[ tʃˈæːːnəl ]]`.
  - A line that ends "Only on Channel 7" reads right as plain text in both voices.
- **Changing inflection must stay easy.** Fixes will come from hearing real voice-overs, so none of this is
  in code:
  - **A file of its own.** Voices and their fixes live in `guide-voices.yml` beside config.yml,
    commented like config.example.yml. Each voice has:
    - its Piper model;
    - its pace, variation and pauses (`length_scale`, `noise_scale`, `noise_w`, `sentence_silence`);
    - its audio processing;
    - its word fixes.
  - **Fixes can be narrow or broad.** A fix can be a single word ("Channel"), or a phrase, so it applies
    only in that spot ("next on Channel"). Either way it gives phonemes, or a slower pace for that piece,
    spoken on its own and joined back in as in sample I. Edits are read again without a restart.
  - **A tester in Settings** (admins, the Guide's settings):
    - type a line or pick one of the real templates, pick a voice, and hear it, with and without the fixes;
    - try phonemes and pace, then save them as a fix.
    - It shows how Piper spelled the line (its phonemes), so you can see what to change.
  - **Fixes reach what's already made.** The cached voice-overs are keyed by the line, the voice and a hash
    of its settings and fixes, so changing a fix remakes only the ones it touches, before they air.
  - **Report a bad one while watching.** While a voice-over plays, the channel's info has "Report
    voice-over". It saves the line, the voice and the time to a list in the tester, so a bad one heard
    on air can be found and fixed later.
- **Sounding like broadcast:** a little slower than Piper's default (`--length-scale` about 1.1), then
  ffmpeg's compressor, a high-pass filter and a slight presence boost, so the voice sits over the music
  like a 1980s announce booth.
- ffmpeg mixes it over the last part of the credits, lowering the credits' music under it. Optional:
  squeeze the credits into part of the picture beside an "Up next" card. Credit starts come from the
  segments table (the intros and credits analyzer); a program without known credits gets it over its
  filler instead.
- It's made ahead with the schedule and cached per slot, so it plays like the rest of the program.

## Playing a channel

- `GET /api/guide?from=&hours=` gives the channels (number, name, badge) and their slots from the
  current half hour to +3 hours, with each program's info: title, episode (S2E5 and its name), times,
  rating, genres, description and poster.
- `GET /api/guide/now?channel=` gives what's on now and how far into it, as a client path plus a start
  offset. The player plays it through the usual stream URL with `start=`, so transcode modes, tracks and
  subtitles all work. When the item ends, the player asks again for the next one (filler, a bumper, the
  next program).
- `POST /api/guide/watched` marks the current program watched (the only thing a channel writes).
- Disc images (`.iso`, `VIDEO_TS`) are left out at first.

## The Guide screen

- **Top left:** the channel being watched, **with sound**. Going back to the guide from full screen keeps
  it playing in this window until a new channel is chosen. When the guide first opens it tunes to the
  last channel watched (kept on the device), or else the first channel. OK or a click on the window
  makes it full screen. Back returns to the guide.
- **Top right:** the selected program's info: title, episode, time slot, rating, genres, description.
- **Below:** the grid. Rows are channels (number, badge, name). Columns are half hours from the
  current half hour to +3 hours, with a line at the current time. Cells are as wide as their programs.
  Moving onto a cell shows its info. Only a cell on now can be watched; later ones show info only.
- TV: D-pad over the grid, as cable boxes do. Phone: the same layout, scrolling. Web: mouse and keys.

## Phases

1. **Server:**
   - TMDB networks and ratings stored (below);
   - library settings;
   - channels, the planner and the slots table;
   - filler packing and generated bumpers;
   - the API;
   - tests.
2. **Web:** the Guide row, the Guide screen, the channel player and the library settings.
3. **Android TV.**
4. **Android phone.**
5. **iPhone** (follows Android).
6. **Voice-overs on the credits.**
7. Later: admin channel editing (names, numbers, logos, hiding), summer reruns, a news source (antenna
   tuner or stream address), the commercials fetch script.

## Data from TMDB

- **Networks:** `/tv/{id}` already returns `networks`. We'll keep the first one's ID and name in a new
  `shows.network` column, used for grouping only; network logos aren't kept.
- **Ratings:** `content_ratings` (TV) and `release_dates` (movies) are added to the existing requests'
  `append_to_response`. We'll keep the US rating: TV-Y…TV-MA, G…NC-17.
- **No new button:** the TMDB lookups job on the Jobs page refreshes a title after 30 days, and at server
  start, on Scan Media and on Run now. A title stored without the new fields counts as due, the way
  movies without a looked-up collection do, so the first run after the update fills them all in.

## Open questions

None at the moment.
