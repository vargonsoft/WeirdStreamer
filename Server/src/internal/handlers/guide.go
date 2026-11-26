package handlers

import (
	"context"
	"encoding/json"
	"hash/crc32"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/fileio"
	"github.com/weirdstreamer/server/internal/guide"
)

// The Guide (docs/GUIDE.md): live channels planned by internal/guide. Each channel is made from
// one library, so a user sees the channels of the libraries their group may see: the home
// screen's Guide those with Show in Guide on, a library's own (lib=) its channels where it's on
// or Library Only.

type GuideHandler struct {
	db      *database.DB
	planner *guide.Service
	weather *guide.WeatherService
}

func NewGuideHandler(db *database.DB, planner *guide.Service) *GuideHandler {
	return &GuideHandler{db: db, planner: planner, weather: guide.NewWeatherService()}
}

// guideChannel is a channel as clients see it.
type guideChannel struct {
	Number   int            `json:"number"`
	Name     string         `json:"name"`
	Genre    string         `json:"genre"`
	Color    string         `json:"color"`
	Logo     string         `json:"logo"`           // its logo (GET /api/guide/logo: PNG, w= its width, format=svg)
	Kind     string         `json:"kind,omitempty"` // a channel of its own's: guide or weather (the app draws it)
	Listings []guideListing `json:"listings,omitempty"`
}

// channelInfo is a channel as clients see it, without listings.
func channelInfo(r *http.Request, c database.GuideChannel) guideChannel {
	logo := "/api/guide/logo?channel=" + strconv.Itoa(c.Number)
	if lib := r.URL.Query().Get("lib"); lib != "" {
		logo += "&lib=" + url.QueryEscape(lib)
	}
	// the logo changes with the channel's name and number: a new address for a new picture
	logo += "&v=" + strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(c.Name+"|"+c.Genre))), 36)
	out := guideChannel{Number: c.Number, Name: c.Name, Genre: c.Genre, Color: c.Color, Logo: logo}
	if sc, ok := guide.SpecialByKey(c.Key); ok {
		out.Kind = sc.Kind
	}
	return out
}

// guideListing is a program in the grid: from its start to the next program's (its filler and
// bumpers included).
type guideListing struct {
	Start int64              `json:"start"` // Unix milliseconds
	End   int64              `json:"end"`
	Info  *guide.ProgramInfo `json:"info,omitempty"` // none for time without a program (an empty channel)
}

// channels are the active channels the request may see, by number.
func (h *GuideHandler) channels(r *http.Request) ([]database.GuideChannel, bool) {
	libID := r.URL.Query().Get("lib")
	if libID != "" {
		lib, ok := config.GlobalConfig.LibraryByID(libID)
		if !ok || !groupCanAccess(userGroup(r), lib) || !lib.InGuide(true) {
			return nil, false
		}
	}
	all, err := h.db.GuideChannels()
	if err != nil {
		log.Printf("[GUIDE] Channels: %v", err)
		return nil, true
	}
	var out []database.GuideChannel
	libraryChannels := false
	for _, c := range all {
		if c.LibraryID == "" { // a channel of its own: on the home screen's Guide, with any other channel
			if c.Active && libID == "" {
				out = append(out, c)
			}
			continue
		}
		lib, ok := config.GlobalConfig.LibraryByID(c.LibraryID)
		if !c.Active || !ok || !groupCanAccess(userGroup(r), lib) {
			continue
		}
		if libID != "" && c.LibraryID != libID || libID == "" && !lib.InGuide(false) {
			continue
		}
		out = append(out, c)
		libraryChannels = true
	}
	if !libraryChannels {
		return nil, true
	}
	return out, true
}

// Available says whether there's a Guide to show: GET /api/guide/available[?lib=]
func (h *GuideHandler) Available(w http.ResponseWriter, r *http.Request) {
	chans, _ := h.channels(r)
	respondJSON(w, http.StatusOK, map[string]bool{"available": len(chans) > 0})
}

// Listings is the grid: GET /api/guide[?lib=][&from=<ms>][&hours=3]. From the current half hour
// (or from) for hours (at most a day), each channel's programs.
func (h *GuideHandler) Listings(w http.ResponseWriter, r *http.Request) {
	chans, ok := h.channels(r)
	if !ok {
		respondError(w, http.StatusNotFound, "No Guide for this library")
		return
	}
	now := time.Now()
	from := now.Truncate(time.Minute)
	from = from.Add(-time.Duration(from.Minute()%30) * time.Minute)
	if ms, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64); err == nil {
		from = time.UnixMilli(ms)
	}
	hours := 3.5
	if v, err := strconv.ParseFloat(r.URL.Query().Get("hours"), 64); err == nil && v > 0 {
		hours = min(v, 24)
	}
	to := from.Add(time.Duration(hours * float64(time.Hour)))
	out := []guideChannel{}
	for _, c := range chans {
		if sc, ok := guide.SpecialByKey(c.Key); ok {
			ch := channelInfo(r, c)
			ch.Listings = specialListings(sc, from, to)
			out = append(out, ch)
			continue
		}
		listings, err := h.listings(c.Key, from, to)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to read the Guide")
			return
		}
		ch := channelInfo(r, c)
		ch.Listings = listings
		out = append(out, ch)
	}
	respondJSON(w, http.StatusOK, map[string]any{"now": now.UnixMilli(), "from": from.UnixMilli(), "to": to.UnixMilli(),
		"channels": out})
}

// listings are a channel's programs overlapping from..to, each until the next one starts.
func (h *GuideHandler) listings(channel string, from, to time.Time) ([]guideListing, error) {
	// A program started before from shows from its start; one started up to a day ahead of to
	// ends its predecessor.
	slots, err := h.db.GuideSlots(channel, from.Add(-6*time.Hour), to.Add(6*time.Hour), "")
	if err != nil {
		return nil, err
	}
	var out []guideListing
	for i := 0; i < len(slots); {
		s := slots[i]
		j := i + 1
		for j < len(slots) && slots[j].Kind != guide.SlotProgram {
			j++
		}
		end := slots[j-1].Start.Add(slots[j-1].Length)
		if end.After(from) && s.Start.Before(to) {
			l := guideListing{Start: s.Start.UnixMilli(), End: end.UnixMilli()}
			if s.Kind == guide.SlotProgram {
				l.Info = programInfo(s.Info)
			}
			out = append(out, l)
		}
		i = j
	}
	return out, nil
}

// specialListings are a channel of its own's half hours, each the same program.
func specialListings(sc guide.SpecialChannel, from, to time.Time) []guideListing {
	var out []guideListing
	start := from.Truncate(time.Minute)
	start = start.Add(-time.Duration(start.Minute()%30) * time.Minute)
	for t := start; t.Before(to); t = t.Add(30 * time.Minute) {
		out = append(out, guideListing{Start: t.UnixMilli(), End: t.Add(30 * time.Minute).UnixMilli(),
			Info: &guide.ProgramInfo{Kind: guide.KindChannel, Title: sc.Title, Overview: sc.Overview}})
	}
	return out
}

func programInfo(raw string) *guide.ProgramInfo {
	if raw == "" {
		return nil
	}
	var p guide.ProgramInfo
	if json.Unmarshal([]byte(raw), &p) != nil {
		return nil
	}
	return &p
}

// channelByNumber is the channel the request names (channel=) among those it may see.
func (h *GuideHandler) channelByNumber(r *http.Request) (*database.GuideChannel, bool) {
	n, err := strconv.Atoi(r.URL.Query().Get("channel"))
	if err != nil {
		return nil, false
	}
	chans, _ := h.channels(r)
	for i := range chans {
		if chans[i].Number == n {
			return &chans[i], true
		}
	}
	return nil, false
}

// guideNow is what a channel is airing.
type guideNow struct {
	Channel guideChannel `json:"channel"`
	Kind    string       `json:"kind"`           // program, filler, bumper; the Guide channel's preview, promo, filler; weather
	Path    string       `json:"path,omitempty"` // program and filler: the file
	Offset  float64      `json:"offset"`         // seconds into it (or into the bumper) now
	Start   int64        `json:"start"`          // Unix milliseconds
	End     int64        `json:"end"`
	// The program on now (the one whose time this is, for filler and bumpers), and the next one
	Program    *guide.ProgramInfo `json:"program,omitempty"`
	Next       *guide.ProgramInfo `json:"next,omitempty"`
	NextStart  int64              `json:"next_start,omitempty"`
	ServerTime int64              `json:"server_time"`
	// The Guide channel's preview and promo blocks: when and where the program airs
	Airs   int64         `json:"airs,omitempty"`
	AirsOn *guideChannel `json:"airs_on,omitempty"`
	// What plays under the Weather channel and the Guide channel's cards
	Music *guideMusic `json:"music,omitempty"`
}

// guideMusic is the track playing under a channel's cards, from offset seconds; the next one
// starts at end.
type guideMusic struct {
	URL    string  `json:"url"`
	Title  string  `json:"title"`
	Credit string  `json:"credit"`
	Offset float64 `json:"offset"`
	Start  int64   `json:"start"`
	End    int64   `json:"end"`
}

func musicAt(now time.Time) *guideMusic {
	t, start, end, ok := guide.MusicAt(now)
	if !ok {
		return nil
	}
	return &guideMusic{URL: "/api/guide/music?name=" + url.QueryEscape(t.Name), Title: t.Title, Credit: t.Credit,
		Offset: now.Sub(start).Seconds(), Start: start.UnixMilli(), End: end.UnixMilli()}
}

// specialNow is what a channel of its own airs: the Weather channel's pages (drawn by the app,
// asked again when the music's track ends), or the Guide channel's block.
func (h *GuideHandler) specialNow(r *http.Request, c database.GuideChannel, sc guide.SpecialChannel, now time.Time) guideNow {
	out := guideNow{Channel: channelInfo(r, c), ServerTime: now.UnixMilli(),
		Program: &guide.ProgramInfo{Kind: guide.KindChannel, Title: sc.Title, Overview: sc.Overview}}
	if sc.Kind == guide.KindWeather {
		out.Kind = guide.KindWeather
		out.Music = musicAt(now)
		start := now.Truncate(10 * time.Minute)
		end := start.Add(10 * time.Minute)
		if out.Music != nil && out.Music.End < end.UnixMilli() {
			end = time.UnixMilli(out.Music.End)
		}
		out.Start, out.End = start.UnixMilli(), end.UnixMilli()
		return out
	}
	// The Guide channel: the programs ahead on the viewer's channels, and their commercials
	chans, _ := h.channels(r)
	byNumber := map[int]database.GuideChannel{}
	var upcoming []guide.Airing
	for _, ch := range chans {
		if ch.LibraryID == "" {
			continue
		}
		byNumber[ch.Number] = ch
		slots, err := h.db.GuideSlots(ch.Key, now, now.Add(6*time.Hour), guide.SlotProgram)
		if err != nil {
			continue
		}
		for _, s := range slots {
			upcoming = append(upcoming, guide.Airing{Channel: ch.Number, Start: s.Start, Length: s.Length, Path: s.Path,
				Info: programInfo(s.Info)})
		}
	}
	clips := h.planner.FillerClips(func(id string) bool {
		lib, ok := config.GlobalConfig.LibraryByID(id)
		return ok && groupCanAccess(userGroup(r), lib)
	})
	b := guide.GuideChannelBlock(now, upcoming, clips)
	out.Kind, out.Path, out.Start, out.End = b.Kind, b.Path, b.Start.UnixMilli(), b.End.UnixMilli()
	out.Offset = (b.Offset + now.Sub(b.Start)).Seconds()
	if a := b.Airing; a != nil {
		out.Program = a.Info
		out.Airs = a.Start.UnixMilli()
		if ch, ok := byNumber[a.Channel]; ok {
			info := channelInfo(r, ch)
			out.AirsOn = &info
		}
	}
	if b.Kind == guide.BlockPromo {
		out.Music = musicAt(now)
	}
	return out
}

// Now is what a channel airs now: GET /api/guide/now?channel=N[&lib=]. The player plays path from
// offset and asks again at end.
func (h *GuideHandler) Now(w http.ResponseWriter, r *http.Request) {
	c, ok := h.channelByNumber(r)
	if !ok {
		respondError(w, http.StatusNotFound, "No such channel")
		return
	}
	now := time.Now()
	if sc, ok := guide.SpecialByKey(c.Key); ok {
		respondJSON(w, http.StatusOK, h.specialNow(r, *c, sc, now))
		return
	}
	slots, err := h.db.GuideSlots(c.Key, now.Add(-6*time.Hour), now.Add(6*time.Hour), "")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to read the Guide")
		return
	}
	cur := -1
	for i, s := range slots {
		if !s.Start.After(now) && s.Start.Add(s.Length).After(now) {
			cur = i
			break
		}
	}
	if cur < 0 {
		respondError(w, http.StatusServiceUnavailable, "This channel isn't planned yet")
		return
	}
	s := slots[cur]
	out := guideNow{Channel: channelInfo(r, *c),
		Kind: s.Kind, Path: s.Path, Offset: now.Sub(s.Start).Seconds(), Start: s.Start.UnixMilli(),
		End: s.Start.Add(s.Length).UnixMilli(), ServerTime: now.UnixMilli()}
	for i := cur; i >= 0; i-- {
		if slots[i].Kind == guide.SlotProgram {
			out.Program = programInfo(slots[i].Info)
			break
		}
	}
	for i := cur + 1; i < len(slots); i++ {
		if slots[i].Kind == guide.SlotProgram {
			out.Next, out.NextStart = programInfo(slots[i].Info), slots[i].Start.UnixMilli()
			break
		}
	}
	respondJSON(w, http.StatusOK, out)
}

// Watched marks the program a channel is airing (or just aired, in its filler), or the one that
// started at start= (Unix ms, within the last 6 hours), watched for the user:
// POST /api/guide/watched?channel=N[&lib=][&start=]. The apps send it when a program plays to its
// end while its channel is on (a setting, off by default). Watching a channel records nothing else.
func (h *GuideHandler) Watched(w http.ResponseWriter, r *http.Request) {
	c, ok := h.channelByNumber(r)
	uid, signedIn := userID(r)
	if !ok || !signedIn {
		respondError(w, http.StatusNotFound, "No such channel")
		return
	}
	now := time.Now()
	slots, err := h.db.GuideSlots(c.Key, now.Add(-6*time.Hour), now.Add(time.Millisecond), guide.SlotProgram)
	if err != nil || len(slots) == 0 {
		respondError(w, http.StatusNotFound, "Nothing to mark")
		return
	}
	s := slots[len(slots)-1]
	// start= names the program (its start, Unix ms): the one that just played to its end
	if ms, err := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64); err == nil {
		found := false
		for _, sl := range slots {
			if sl.Start.UnixMilli() == ms {
				s, found = sl, true
			}
		}
		if !found {
			respondError(w, http.StatusNotFound, "Nothing to mark")
			return
		}
	}
	if err := h.db.SavePlayback(uid, s.Path, database.PlaybackUpdate{Duration: s.Length.Seconds(), Finished: true,
		UpdatedAt: now}, false); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to mark it watched")
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Logo is a channel's logo: GET /api/guide/logo?channel=N[&lib=][&w=512][&color=rrggbb][&format=svg].
// One color (white unless color= says), on a transparent background: a PNG w pixels wide (32 to
// 4096, half as high), or SVG.
func (h *GuideHandler) Logo(w http.ResponseWriter, r *http.Request) {
	c, ok := h.channelByNumber(r)
	if !ok {
		respondError(w, http.StatusNotFound, "No such channel")
		return
	}
	art := guide.LogoFor(c.Name, c.Genre, guide.IsStation(c.Name))
	if art == nil {
		respondError(w, http.StatusNotFound, "No logo")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	tint := guide.LogoColor(r.URL.Query().Get("color")) // rrggbb, white by default
	if r.URL.Query().Get("format") == "svg" {
		b, err := art.SVG(c.Number, tint)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to draw the logo")
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(b)
		return
	}
	width, err := strconv.Atoi(r.URL.Query().Get("w"))
	if err != nil {
		width = 512
	}
	b, err := art.PNG(c.Number, width, tint)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to draw the logo")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(b)
}

// Plan plans the Guide again soon (settings changed).
func (h *GuideHandler) Plan() { h.planner.Kick() }

// Weather is the Weather channel's data: GET /api/guide/weather (for the server's ZIP code).
func (h *GuideHandler) Weather(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second) // one reading serves every viewer
	defer cancel()
	respondJSON(w, http.StatusOK, h.weather.Get(ctx))
}

// WeatherRadar is the area's latest radar image: GET /api/guide/weather/radar (a GIF).
func (h *GuideHandler) WeatherRadar(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b, err := h.weather.Radar(ctx)
	if err != nil {
		respondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write(b)
}

// WeatherIcon is a condition icon: GET /api/guide/weather/icon?name=rain[&w=128][&color=rrggbb] (a
// PNG, white unless color= says, transparent around it).
func (h *GuideHandler) WeatherIcon(w http.ResponseWriter, r *http.Request) {
	size, err := strconv.Atoi(r.URL.Query().Get("w"))
	if err != nil {
		size = 128
	}
	b, ok := guide.WeatherIconPNG(r.URL.Query().Get("name"), size, guide.LogoColor(r.URL.Query().Get("color")))
	if !ok {
		respondError(w, http.StatusNotFound, "No such icon")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(b)
}

// Music is a track of the Guide's music: GET /api/guide/music?name=<its file's name>.
func (h *GuideHandler) Music(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	dir := guide.MusicDir()
	if dir == "" || name == "" || filepath.Base(name) != name || strings.HasPrefix(name, ".") ||
		!slices.ContainsFunc(guide.MusicTracks(), func(t guide.MusicTrack) bool { return t.Name == name }) {
		respondError(w, http.StatusNotFound, "No such track")
		return
	}
	fileio.ServeFile(w, r, filepath.Join(dir, name))
}
