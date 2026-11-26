package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/guide"
)

// The Guide tab of Settings (admins; docs/GUIDE.md, "The Guide tab"): the typed channels, each
// library's Guide switches and the schedule's settings, saved apart from the rest of Settings.

// SetConfigSaver gives the Guide tab the way Settings saves config.yml (validated, saved, reloaded).
func (h *GuideHandler) SetConfigSaver(save func(*config.Config) (int, error)) { h.saveConfig = save }

type guideAdminLibrary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`          // movie, tv or "" (a folder library: only typed channels and filler take it)
	Groups      []string `json:"groups"`        // who sees it
	ShowInGuide string   `json:"show_in_guide"` // on, libraryOnly, off ("" is off)
	GuideFiller bool     `json:"guide_filler"`
	Typed       bool     `json:"typed"` // a typed channel uses it: it's in no regular channel
}

type guideAdminChannel struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

type guideAdminTyped struct {
	ID        string              `json:"id"`
	Type      string              `json:"type"`
	Libraries []string            `json:"libraries"`
	Max       int                 `json:"max"`
	Number    int                 `json:"number"`   // its block's first number
	Groups    []string            `json:"groups"`   // who sees its channels
	Channels  []guideAdminChannel `json:"channels"` // the ones it has now (made at the next planning)
}

type guideAdminType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type guideAdmin struct {
	PrimeTime     int                 `json:"prime_time"`
	WeatherZip    string              `json:"weather_zip"`
	Libraries     []guideAdminLibrary `json:"libraries"`
	TypedChannels []guideAdminTyped   `json:"typed_channels"`
	Types         []guideAdminType    `json:"types"`
	MaxChannels   int                 `json:"max_channels"` // a typed channel's most
	MaxTyped      int                 `json:"max_typed"`    // the most typed channels
	MusicTracks   int                 `json:"music_tracks"` // in the music folder
	VoiceOvers    bool                `json:"voice_overs"`  // Piper is set up
}

func (h *GuideHandler) guideAdmin() guideAdmin {
	cfg := config.GlobalConfig
	out := guideAdmin{PrimeTime: cfg.Guide.PrimeTime, WeatherZip: cfg.Guide.WeatherZip, Libraries: []guideAdminLibrary{},
		TypedChannels: []guideAdminTyped{}, MaxChannels: config.TypedChannelMax,
		MaxTyped:    (config.TypedNumberLast-config.TypedNumberFirst)/config.TypedNumberStride + 1,
		MusicTracks: len(guide.MusicTracks()), VoiceOvers: h.planner != nil && h.planner.Voices().Enabled()}
	for _, t := range config.TypedChannelTypes {
		out.Types = append(out.Types, guideAdminType{ID: t, Name: guide.TypedTypeName(t)})
	}
	for _, p := range cfg.Media.Paths {
		out.Libraries = append(out.Libraries, guideAdminLibrary{ID: p.ID, Name: p.Name, Type: p.Type,
			Groups: append([]string{}, p.Groups...), ShowInGuide: p.ShowInGuide, GuideFiller: p.GuideFiller,
			Typed: cfg.InTypedChannel(p.ID)})
	}
	chans, _ := h.db.GuideChannels()
	for _, t := range cfg.Guide.TypedChannels {
		a := guideAdminTyped{ID: t.ID, Type: t.Type, Libraries: append([]string{}, t.Libraries...), Max: t.Max,
			Number: t.Number, Groups: cfg.TypedChannelGroups(t), Channels: []guideAdminChannel{}}
		if a.Groups == nil {
			a.Groups = []string{}
		}
		for _, c := range chans {
			if c.Active && c.LibraryID == guide.TypedLibraryID(t.ID) {
				a.Channels = append(a.Channels, guideAdminChannel{Number: c.Number, Name: c.Name})
			}
		}
		slices.SortFunc(a.Channels, func(x, y guideAdminChannel) int { return x.Number - y.Number })
		out.TypedChannels = append(out.TypedChannels, a)
	}
	return out
}

// GuideSettings is the Guide tab: GET /api/admin/guide.
func (h *GuideHandler) GuideSettings(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	respondJSON(w, http.StatusOK, h.guideAdmin())
}

// SaveGuideSettings saves the Guide tab: PUT /api/admin/guide {"prime_time", "weather_zip",
// "libraries": [{"id", "show_in_guide", "guide_filler"}], "typed_channels": [{"id", "type",
// "libraries", "max"}]}. A typed channel without an id is new (it gets one, and a block of
// numbers); one left out is removed. Libraries left out keep their switches. It answers with the
// tab as saved, and the Guide plans again.
func (h *GuideHandler) SaveGuideSettings(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	var body struct {
		PrimeTime  int    `json:"prime_time"`
		WeatherZip string `json:"weather_zip"`
		Libraries  []struct {
			ID          string `json:"id"`
			ShowInGuide string `json:"show_in_guide"`
			GuideFiller bool   `json:"guide_filler"`
		} `json:"libraries"`
		TypedChannels []struct {
			ID        string   `json:"id"`
			Type      string   `json:"type"`
			Libraries []string `json:"libraries"`
			Max       int      `json:"max"`
		} `json:"typed_channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if h.saveConfig == nil {
		respondError(w, http.StatusInternalServerError, "Settings can't be saved")
		return
	}
	c := currentConfig()
	c.Guide.PrimeTime = body.PrimeTime
	c.Guide.WeatherZip = strings.TrimSpace(body.WeatherZip)
	for _, l := range body.Libraries {
		for i := range c.Media.Paths {
			if c.Media.Paths[i].ID == l.ID {
				c.Media.Paths[i].ShowInGuide, c.Media.Paths[i].GuideFiller = l.ShowInGuide, l.GuideFiller
			}
		}
	}
	old := c.Guide.TypedChannels
	c.Guide.TypedChannels = []config.TypedChannel{}
	for _, t := range body.TypedChannels {
		n := config.TypedChannel{ID: t.ID, Type: t.Type, Libraries: append([]string{}, t.Libraries...), Max: t.Max}
		if i := slices.IndexFunc(old, func(o config.TypedChannel) bool { return o.ID == t.ID && t.ID != "" }); i >= 0 {
			n.Number = old[i].Number // its block stays
		} else {
			n.ID = "" // new: the server gives its ID
		}
		c.Guide.TypedChannels = append(c.Guide.TypedChannels, n)
	}
	if status, err := h.saveConfig(&c); err != nil {
		respondError(w, status, err.Error())
		return
	}
	if h.planner != nil {
		h.planner.Kick()
	}
	respondJSON(w, http.StatusOK, h.guideAdmin())
}

// ReplanGuide plans every channel again from its next program, the next 6 hours too (what's on now
// stays): POST /api/admin/guide/replan. Destructive, so only by hand: the Guide tab asks first.
func (h *GuideHandler) ReplanGuide(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	if h.planner == nil {
		respondError(w, http.StatusServiceUnavailable, "The Guide isn't running")
		return
	}
	if err := h.planner.Replan(context.WithoutCancel(r.Context())); err != nil {
		respondError(w, http.StatusInternalServerError, "Planning failed: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Planned again from each channel's next program."})
}
