package handlers

import (
	"encoding/json"
	"hash/crc32"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/guide"
)

// The Guide (docs/GUIDE.md): live channels planned by internal/guide. Each channel is made from
// one library, so a user sees the channels of the libraries their group may see: the home
// screen's Guide those with Show in Guide on, a library's own (lib=) its channels where it's on
// or Library Only.

type GuideHandler struct {
	db      *database.DB
	planner *guide.Service
}

func NewGuideHandler(db *database.DB, planner *guide.Service) *GuideHandler {
	return &GuideHandler{db: db, planner: planner}
}

// guideChannel is a channel as clients see it.
type guideChannel struct {
	Number   int            `json:"number"`
	Name     string         `json:"name"`
	Genre    string         `json:"genre"`
	Color    string         `json:"color"`
	Logo     string         `json:"logo"` // its logo (GET /api/guide/logo: PNG, w= its width, format=svg)
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
	return guideChannel{Number: c.Number, Name: c.Name, Genre: c.Genre, Color: c.Color, Logo: logo}
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
	for _, c := range all {
		lib, ok := config.GlobalConfig.LibraryByID(c.LibraryID)
		if !c.Active || !ok || !groupCanAccess(userGroup(r), lib) {
			continue
		}
		if libID != "" && c.LibraryID != libID || libID == "" && !lib.InGuide(false) {
			continue
		}
		out = append(out, c)
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
	Kind    string       `json:"kind"`           // program, filler, bumper
	Path    string       `json:"path,omitempty"` // program and filler: the file
	Offset  float64      `json:"offset"`         // seconds into it (or into the bumper) now
	Start   int64        `json:"start"`          // Unix milliseconds
	End     int64        `json:"end"`
	// The program on now (the one whose time this is, for filler and bumpers), and the next one
	Program    *guide.ProgramInfo `json:"program,omitempty"`
	Next       *guide.ProgramInfo `json:"next,omitempty"`
	NextStart  int64              `json:"next_start,omitempty"`
	ServerTime int64              `json:"server_time"`
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

// Watched marks the program a channel is airing (or just aired, in its filler) watched for the
// user: POST /api/guide/watched?channel=N[&lib=]. Watching a channel records nothing else.
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
