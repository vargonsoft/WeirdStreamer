package guide

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	bgjobs "github.com/weirdstreamer/server/internal/jobs"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/metadata"
)

// How far the schedule reaches
const (
	PlanAhead    = 72 * time.Hour      // planned this far ahead
	KeepPlanned  = 6 * time.Hour       // when the libraries change, what's planned this soon stays
	KeepHistory  = 35 * 24 * time.Hour // aired slots kept this long: the planner goes on from them
	refreshEvery = time.Hour
	firstRefresh = time.Minute // after start, so the TMDB sync goes first
)

// ProgramInfo is what the guide shows of a program, stored with its slot.
type ProgramInfo struct {
	Kind            string   `json:"kind"` // movie or show
	TMDBID          int      `json:"tmdb_id"`
	Library         string   `json:"library"`
	Title           string   `json:"title"`
	Year            int      `json:"year,omitempty"`
	Rating          string   `json:"rating,omitempty"`
	Genres          []string `json:"genres,omitempty"`
	Overview        string   `json:"overview,omitempty"`
	Poster          string   `json:"poster,omitempty"`
	Season          int      `json:"season,omitempty"`
	Episode         int      `json:"episode,omitempty"`
	EpisodeTitle    string   `json:"episode_title,omitempty"`
	EpisodeOverview string   `json:"episode_overview,omitempty"`
}

// Service keeps the Guide's channels planned: at start, every hour, and when asked (settings
// changes, Run now on the Jobs page).
type Service struct {
	db   *database.DB
	meta *metadata.Service
	job  *bgjobs.Job
	mu   sync.Mutex // one refresh at a time
	kick chan struct{}

	clipsMu    sync.Mutex
	clipsByLib map[string][]Clip // the filler libraries' clips, by library ID, from the last refresh
}

// FillerClips are the filler clips of the libraries may(id) says the viewer sees (the Guide
// channel's commercials).
func (s *Service) FillerClips(may func(libraryID string) bool) []Clip {
	s.clipsMu.Lock()
	defer s.clipsMu.Unlock()
	var out []Clip
	for id, clips := range s.clipsByLib {
		if may(id) {
			out = append(out, clips...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// NewService makes the Guide's planner; Start runs it.
func NewService(db *database.DB, meta *metadata.Service) *Service {
	return &Service{db: db, meta: meta, kick: make(chan struct{}, 1),
		job: bgjobs.Register("guide", "Guide",
			"Plans the Guide's channels three days ahead from the libraries with Show in Guide on.")}
}

// Start plans in the background until ctx is done.
func (s *Service) Start(ctx context.Context) {
	s.job.Schedule("A minute after server start, every hour, and when settings change")
	s.job.OnRunNow(s.Kick)
	go func() {
		timer := time.NewTimer(firstRefresh)
		s.job.NextIn(firstRefresh)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			case <-s.kick:
				timer.Stop()
			}
			if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
				log.Printf("[GUIDE] Planning failed: %v", err)
			}
			timer.Reset(refreshEvery)
			s.job.NextIn(refreshEvery)
		}
	}()
}

// Kick plans again soon (libraries or settings changed).
func (s *Service) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Refresh brings every channel's schedule up to PlanAhead, planning again from KeepPlanned
// ahead the channels whose titles changed.
func (s *Service) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := config.GlobalConfig
	set := Settings{PrimeTime: cfg.Guide.PrimeTime, Location: time.Local}
	if set.PrimeTime == 0 {
		set.PrimeTime = config.PrimeTimeDefault
	}
	now := time.Now()

	var libs []config.MediaPath
	for _, lib := range cfg.Media.Paths {
		if (lib.InGuide(false) || lib.InGuide(true)) && (lib.Type == config.LibraryTypeMovie || lib.Type == config.LibraryTypeTV) {
			libs = append(libs, lib)
		}
	}
	s.job.Begin(len(libs))
	stored, err := s.db.GuideChannels()
	if err != nil {
		s.job.End("failed: " + err.Error())
		return err
	}
	byKey := map[string]database.GuideChannel{}
	for _, c := range stored {
		byKey[c.Key] = c
	}
	clipsByLib, err := s.fillerClips(cfg.Media.Paths)
	if err != nil {
		s.job.End("failed: " + err.Error())
		return err
	}
	s.clipsMu.Lock()
	s.clipsByLib = clipsByLib
	s.clipsMu.Unlock()

	active := map[string]bool{}
	planned := 0
	for _, lib := range libs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.job.SetDetail(lib.Name)
		titles, err := s.titles(ctx, lib)
		if err != nil {
			log.Printf("[GUIDE] %s: %v", lib.Name, err)
			s.job.Step(1)
			continue
		}
		clips := clipsFor(lib, cfg.Media.Paths, clipsByLib)
		for _, ch := range Channels(lib.ID, titles) {
			active[ch.Key] = true
			c, ok := byKey[ch.Key]
			if !ok {
				if c, err = s.addChannel(ch, byKey); err != nil {
					return err
				}
				byKey[ch.Key] = c
			} else if !c.Active {
				if err := s.db.SetGuideChannelActive(ch.Key, true); err != nil {
					return err
				}
			}
			if ch.Network && !IsStation(c.Name) { // named before the pool of stations
				c.Name = pickStation(ch.Key, byKey)
				if err := s.db.SetGuideChannelName(ch.Key, c.Name); err != nil {
					return err
				}
				byKey[ch.Key] = c
			}
			if !slices.Contains(Colors, c.Color) { // a color since taken out of the palette
				c.Color = Colors[hash(ch.Key)%uint64(len(Colors))]
				if err := s.db.SetGuideChannelColor(ch.Key, c.Color); err != nil {
					return err
				}
			}
			n, err := s.planChannel(ch, c, set, clips, now)
			if err != nil {
				return fmt.Errorf("channel %d: %w", c.Number, err)
			}
			planned += n
		}
		s.job.Step(1)
	}
	// The channels of their own, while there's a Guide
	if len(active) > 0 {
		for _, sc := range SpecialChannels {
			active[sc.Key] = true
			c, ok := byKey[sc.Key]
			if !ok {
				if c, err = s.addSpecial(sc, byKey); err != nil {
					return err
				}
				byKey[sc.Key] = c
			} else if !c.Active {
				if err := s.db.SetGuideChannelActive(sc.Key, true); err != nil {
					return err
				}
			}
		}
	}
	for _, c := range stored {
		if c.Active && !active[c.Key] {
			if err := s.db.SetGuideChannelActive(c.Key, false); err != nil {
				return err
			}
			if err := s.db.DeleteGuideSlotsFrom(c.Key, now); err != nil {
				return err
			}
		}
	}
	if err := s.db.DeleteGuideSlotsBefore(now.Add(-KeepHistory)); err != nil {
		return err
	}
	s.job.End(fmt.Sprintf("%d channels, %d slots planned", len(active), planned))
	return nil
}

// planChannel plans a channel on to PlanAhead, returning how many slots it added.
func (s *Service) planChannel(ch Channel, c database.GuideChannel, set Settings, clips []Clip, now time.Time) (int, error) {
	fp := fingerprint(ch, clips, set)
	if fp != c.Fingerprint {
		// Its titles changed: plan again from the first program KeepPlanned ahead.
		next, err := s.db.GuideSlots(ch.Key, now.Add(KeepPlanned), now.Add(PlanAhead+24*time.Hour), SlotProgram)
		if err != nil {
			return 0, err
		}
		if len(next) > 0 {
			if err := s.db.DeleteGuideSlotsFrom(ch.Key, next[0].Start); err != nil {
				return 0, err
			}
		}
		if err := s.db.SetGuideFingerprint(ch.Key, fp); err != nil {
			return 0, err
		}
	}
	from := set.AlignHalfHour(now)
	if last, err := s.db.LastGuideSlot(ch.Key); err != nil {
		return 0, err
	} else if last != nil && !last.Start.Add(last.Length).Before(from) {
		from = last.Start.Add(last.Length)
	}
	to := now.Add(PlanAhead)
	if !from.Before(to) {
		return 0, nil
	}

	st := NewState(set)
	progs, err := s.db.GuideSlots(ch.Key, now.Add(-KeepHistory), from, SlotProgram)
	if err != nil {
		return 0, err
	}
	fillers, err := s.db.GuideSlots(ch.Key, now.Add(-clipRepeatAfter), from, SlotFiller)
	if err != nil {
		return 0, err
	}
	history := append(progs, fillers...)
	sort.Slice(history, func(i, j int) bool { return history[i].Start.Before(history[j].Start) })
	for _, h := range history {
		st.Record(Slot{Channel: h.Channel, Start: h.Start, Length: h.Length, Kind: h.Kind, Title: h.Title, Item: h.Item,
			Path: h.Path, Track: h.Track})
	}

	byKey := map[string]*Title{}
	for _, t := range ch.Titles {
		byKey[t.Key] = t
	}
	slots := Plan(ch, from, to, st, clips)
	rows := make([]database.GuideSlot, len(slots))
	for i, sl := range slots {
		rows[i] = database.GuideSlot{Channel: sl.Channel, Start: sl.Start, Length: sl.Length, Kind: sl.Kind,
			Title: sl.Title, Item: sl.Item, Path: sl.Path, Track: sl.Track}
		if t := byKey[sl.Title]; sl.Kind == SlotProgram && t != nil {
			rows[i].Info = programInfo(ch.LibraryID, t, sl.Item)
		}
	}
	return len(rows), s.db.AddGuideSlots(rows)
}

func programInfo(lib string, t *Title, item int) string {
	info := ProgramInfo{Kind: t.Kind, TMDBID: t.TMDBID, Library: lib, Title: t.Name, Year: t.Year, Rating: t.Rating,
		Genres: t.Genres, Overview: t.Overview, Poster: t.Poster}
	if t.Kind == KindShow && item < len(t.Items) {
		it := t.Items[item]
		info.Season, info.Episode, info.EpisodeTitle, info.EpisodeOverview = it.Season, it.Episode, it.Name, it.Overview
	}
	b, _ := json.Marshal(info)
	return string(b)
}

// fingerprint sums up what a channel is planned from.
func fingerprint(ch Channel, clips []Clip, set Settings) string {
	h := uint64(0)
	add := func(s string) { h = h*31 + hash(s) }
	add(strconv.Itoa(set.PrimeTime))
	for _, t := range ch.Titles {
		add(t.Key + t.Rating)
		for _, it := range t.Items {
			add(it.Path + it.Duration.String())
		}
	}
	add(strconv.Itoa(len(clips)))
	return strconv.FormatUint(h, 36)
}

// addChannel stores a new channel with the next free number: network channels from 2 (the
// broadcast channels), the others from 14 (cable).
func (s *Service) addChannel(ch Channel, existing map[string]database.GuideChannel) (database.GuideChannel, error) {
	taken := map[int]bool{}
	for _, c := range existing {
		taken[c.Number] = true
	}
	n := 14
	if ch.Network {
		n = 2
	}
	for taken[n] {
		n++
	}
	name := ch.Genre
	if ch.Network {
		name = pickStation(ch.Key, existing)
	}
	c := database.GuideChannel{Key: ch.Key, LibraryID: ch.LibraryID, Number: n, Name: name, Genre: ch.Genre,
		Kind: ch.Kind, Color: Colors[hash(ch.Key)%uint64(len(Colors))], Active: true}
	return c, s.db.AddGuideChannel(c)
}

// addSpecial stores a channel of its own: the Guide channel as 1, the others with the next free
// number from 14.
func (s *Service) addSpecial(sc SpecialChannel, existing map[string]database.GuideChannel) (database.GuideChannel, error) {
	taken := map[int]bool{}
	for _, c := range existing {
		taken[c.Number] = true
	}
	n := 14
	if sc.Key == GuideChannelKey && !taken[GuideChannelNum] {
		n = GuideChannelNum
	}
	for taken[n] {
		n++
	}
	c := database.GuideChannel{Key: sc.Key, Number: n, Name: sc.Name, Genre: sc.Name, Kind: sc.Kind,
		Color: Colors[hash(sc.Key)%uint64(len(Colors))], Active: true}
	return c, s.db.AddGuideChannel(c)
}

// pickStation is a call sign from the pool for a network channel, neither taken by another channel
// nor in another channel's font.
func pickStation(key string, existing map[string]database.GuideChannel) string {
	taken, fonts := map[string]bool{}, GenreFonts()
	for k, c := range existing {
		if k == key {
			continue
		}
		taken[c.Name] = true
		if c.Active {
			if a := LogoFor(c.Name, c.Genre, IsStation(c.Name)); a != nil {
				fonts[a.Font] = true
			}
		}
	}
	return PickStation(key, taken, fonts)
}

// titles are a library's titles as the planner takes them: client paths and file lengths.
func (s *Service) titles(ctx context.Context, lib config.MediaPath) ([]*Title, error) {
	gts, err := s.meta.GuideTitles(ctx, lib)
	if err != nil {
		return nil, err
	}
	var out []*Title
	for _, g := range gts {
		t := &Title{Key: TitleKey(g.Kind, g.TMDBID), Kind: g.Kind, TMDBID: g.TMDBID, Name: g.Title, Year: g.Year,
			Genres: g.Genres, NetworkID: g.NetworkID, Rating: g.Rating, Overview: g.Overview, Poster: g.Poster}
		if g.Kind == KindMovie || len(g.Episodes) == 0 {
			t.Kind = KindMovie // a TV movie in a movie library airs as a movie
			for _, f := range g.Files {
				if it, ok := s.item(f); ok {
					t.Items = append(t.Items, it)
					break // a movie's first file
				}
			}
		} else {
			for _, e := range g.Episodes {
				if it, ok := s.item(e.Path); ok {
					it.Season, it.Episode, it.Name, it.Overview = e.Season, e.Episode, e.Title, e.Overview
					t.Items = append(t.Items, it)
				}
			}
		}
		if len(t.Items) > 0 {
			out = append(out, t)
		}
	}
	return out, nil
}

// item is a file as the planner takes it: its client path and length (known once scanned).
func (s *Service) item(path string) (Item, bool) {
	client, ok := config.ClientPath(path)
	if !ok || media.IsDiscImage(path) {
		return Item{}, false
	}
	m, err := s.db.GetMediaItem(path)
	if err != nil || m == nil || m.Duration <= 0 {
		return Item{}, false
	}
	return Item{Path: client, Duration: time.Duration(m.Duration*1000) * time.Millisecond}, true
}

var clipYear = regexp.MustCompile(`\b(19[5-9]\d|20[0-4]\d)\b`)

// fillerClips are the short videos of each Use for Guide Filler library, by library ID.
func (s *Service) fillerClips(libs []config.MediaPath) (map[string][]Clip, error) {
	out := map[string][]Clip{}
	var fillers []config.MediaPath
	for _, lib := range libs {
		if lib.GuideFiller {
			fillers = append(fillers, lib)
		}
	}
	if len(fillers) == 0 {
		return out, nil
	}
	items, err := s.db.ListMediaItems("video")
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		d := time.Duration(it.Duration*1000) * time.Millisecond
		if d <= 0 || d > MaxClip || media.IsDiscImage(it.Path) {
			continue
		}
		for _, lib := range fillers {
			if !config.InDir(it.Path, lib.Path) {
				continue
			}
			client, ok := config.ClientPath(it.Path)
			if !ok {
				continue
			}
			c := Clip{Path: client, Duration: d}
			if m := clipYear.FindString(client); m != "" {
				c.Year, _ = strconv.Atoi(m)
			}
			out[lib.ID] = append(out[lib.ID], c)
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i].Path < out[id][j].Path })
	}
	return out, nil
}

// clipsFor are the filler clips a library's channels may air: those of filler libraries every
// group seeing the library also sees (a viewer never gets a clip from a library hidden from them).
func clipsFor(lib config.MediaPath, libs []config.MediaPath, clipsByLib map[string][]Clip) []Clip {
	var out []Clip
	for _, f := range libs {
		if !f.GuideFiller {
			continue
		}
		seesAll := true
		for _, g := range lib.Groups {
			if !slices.Contains(f.Groups, g) {
				seesAll = false
				break
			}
		}
		if seesAll {
			out = append(out, clipsByLib[f.ID]...)
		}
	}
	return out
}
