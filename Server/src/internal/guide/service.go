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
	"github.com/weirdstreamer/server/internal/models"
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
	Block           bool     `json:"block,omitempty"` // a block of short clips (its filler is its other clips)
}

// Service keeps the Guide's channels planned: at start, every hour, and when asked (settings
// changes, Run now on the Jobs page).
type Service struct {
	db   *database.DB
	meta *metadata.Service
	job  *bgjobs.Job
	mu   sync.Mutex // one refresh at a time
	kick chan struct{}

	voices   *Voices
	voiceJob *bgjobs.Job
	voiceRun sync.Mutex // one run of the voice-overs at a time

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
	return &Service{db: db, meta: meta, kick: make(chan struct{}, 1), voices: NewVoices(), voiceJob: newVoiceJob(),
		job: bgjobs.Register("guide", "Guide",
			"Plans the Guide's channels three days ahead from the libraries with Show in Guide on.")}
}

// Start plans in the background until ctx is done.
func (s *Service) Start(ctx context.Context) {
	s.job.Schedule("A minute after server start, every hour, and when settings change")
	s.job.OnRunNow(s.Kick)
	s.voiceJob.Schedule("After each planning, and every 10 minutes")
	s.voiceJob.OnRunNow(func() { go s.voiceOversOnce(ctx) })
	go func() {
		t := time.NewTicker(voiceOversEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.voiceOversOnce(ctx)
			}
		}
	}()
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
			go s.voiceOversOnce(ctx)
			timer.Reset(refreshEvery)
			s.job.NextIn(refreshEvery)
		}
	}()
}

// voiceOversOnce makes the voice-overs ahead, one run at a time.
func (s *Service) voiceOversOnce(ctx context.Context) {
	if !s.voiceRun.TryLock() {
		return
	}
	defer s.voiceRun.Unlock()
	s.makeVoiceOvers(ctx)
}

// VoiceOversSoon makes the voice-overs ahead again now (a fix changed some).
func (s *Service) VoiceOversSoon() { go s.voiceOversOnce(context.Background()) }

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
		// a library a typed channel uses is in no regular channel
		if (lib.InGuide(false) || lib.InGuide(true)) && (lib.Type == config.LibraryTypeMovie || lib.Type == config.LibraryTypeTV) &&
			!cfg.InTypedChannel(lib.ID) {
			libs = append(libs, lib)
		}
	}
	typed := Typed{}
	for _, t := range cfg.Guide.TypedChannels {
		typed[t.ID] = t.Number
	}
	s.job.Begin(len(libs) + len(cfg.Guide.TypedChannels))
	stored, err := s.db.GuideChannels()
	if err != nil {
		s.job.End("failed: " + err.Error())
		return err
	}
	byKey := map[string]database.GuideChannel{}
	for _, c := range stored {
		byKey[c.Key] = c
	}
	videos := &videoList{db: s.db}
	clipsByLib, err := s.fillerClips(cfg.Media.Paths, videos)
	if err != nil {
		s.job.End("failed: " + err.Error())
		return err
	}
	s.clipsMu.Lock()
	s.clipsByLib = clipsByLib
	s.clipsMu.Unlock()

	active := map[string]bool{}
	planned := 0
	// place stores a channel (new, or back) and plans it on
	place := func(ch Channel, clips []Clip) error {
		active[ch.Key] = true
		c, ok := byKey[ch.Key]
		if !ok {
			if c, err = s.addChannel(ch, byKey, typed); err != nil {
				return err
			}
			byKey[ch.Key] = c
		} else if !c.Active {
			if err := s.db.SetGuideChannelActive(ch.Key, true); err != nil {
				return err
			}
		}
		rename := ""
		switch {
		case ch.Network && !IsStation(c.Name): // named before the pool of stations
			rename = pickStation(ch.Key, byKey)
		case ch.Type != "" && !IsTypedName(ch.Type, c.Name): // named before its type's names, or its type changed
			rename = TypedName(ch.Type, takenNames(ch.Key, byKey))
		}
		if rename != "" {
			c.Name = rename
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
		return nil
	}
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
			if err := place(ch, clips); err != nil {
				return err
			}
		}
		s.job.Step(1)
	}
	for _, def := range cfg.Guide.TypedChannels {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.job.SetDetail(TypedTypeName(def.Type) + " (typed)")
		groups, clips, err := s.typedContent(ctx, cfg, def, videos)
		if err != nil {
			log.Printf("[GUIDE] Typed channels %s: %v", TypedTypeName(def.Type), err)
			s.job.Step(1)
			continue
		}
		for _, ch := range TypedChannels(def.ID, def.Type, def.Max, groups) {
			if err := place(ch, clipsBesides(ch, clips)); err != nil {
				return err
			}
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
	if err := s.renumber(typed); err != nil {
		return err
	}
	if err := s.db.DeleteGuideSlotsBefore(now.Add(-KeepHistory)); err != nil {
		return err
	}
	s.job.End(fmt.Sprintf("%d channels, %d slots planned", len(active), planned))
	return nil
}

// Replan plans every channel again from its next program: what each airs now stays (a viewer isn't cut
// off mid-program), and everything after it is made anew, the next 6 hours too. Only an admin asks for it
// (the Guide tab's Plan again from now); planning by itself never changes the next 6 hours.
func (s *Service) Replan(ctx context.Context) error {
	s.mu.Lock()
	chans, err := s.db.GuideChannels()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	now := time.Now()
	cleared := 0
	for _, c := range chans {
		slots, err := s.db.GuideSlots(c.Key, now, now.Add(PlanAhead+24*time.Hour), SlotProgram)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		for _, sl := range slots {
			if sl.Start.After(now) { // the first program after the one on now (programs start on half hours)
				if err := s.db.DeleteGuideSlotsFrom(c.Key, sl.Start); err != nil {
					s.mu.Unlock()
					return err
				}
				cleared++
				break
			}
		}
	}
	s.mu.Unlock()
	log.Printf("[GUIDE] Planning %d channels again from their next programs (asked by an admin)", cleared)
	err = s.Refresh(ctx)
	go s.voiceOversOnce(ctx)
	return err
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
			lib := ch.LibraryID
			if t.Library != "" {
				lib = t.Library
			}
			rows[i].Info = programInfo(lib, t, sl.Item)
		}
	}
	return len(rows), s.db.AddGuideSlots(rows)
}

func programInfo(lib string, t *Title, item int) string {
	info := ProgramInfo{Kind: t.Kind, TMDBID: t.TMDBID, Library: lib, Title: t.Name, Year: t.Year, Rating: t.Rating,
		Genres: t.Genres, Overview: t.Overview, Poster: t.Poster, Block: t.Block}
	if item < len(t.Items) {
		it := t.Items[item]
		if t.Kind == KindShow {
			info.Season, info.Episode, info.EpisodeTitle, info.EpisodeOverview = it.Season, it.Episode, it.Name, it.Overview
		}
		if t.Folder && it.Name != "" {
			info.Title, info.EpisodeTitle = t.Name+" - "+it.Name, "" // a folder's program: the folder and the file
		}
		if info.Poster == "" {
			info.Poster = it.Poster
		}
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
		add(t.Key + t.Rating + t.Name)
		for _, it := range t.Items {
			add(it.Path + it.Duration.String() + it.Name) // what's stored with its slots too: a new name is planned again
		}
	}
	add(strconv.Itoa(len(clips)))
	return strconv.FormatUint(h, 36)
}

// addChannel stores a new channel with the next free number of its block (numbers.go).
func (s *Service) addChannel(ch Channel, existing map[string]database.GuideChannel, typed Typed) (database.GuideChannel, error) {
	name := ch.Genre
	switch {
	case ch.Network:
		name = pickStation(ch.Key, existing)
	case ch.Type != "":
		name = TypedName(ch.Type, takenNames(ch.Key, existing))
	}
	c := database.GuideChannel{Key: ch.Key, LibraryID: ch.LibraryID, Name: name, Genre: ch.Genre,
		Kind: ch.Kind, Color: Colors[hash(ch.Key)%uint64(len(Colors))], Active: true}
	c.Number = nextNumber(c, existing, typed)
	return c, s.db.AddGuideChannel(c)
}

// addSpecial stores a channel of its own with the next free number of its block.
func (s *Service) addSpecial(sc SpecialChannel, existing map[string]database.GuideChannel) (database.GuideChannel, error) {
	c := database.GuideChannel{Key: sc.Key, Name: sc.Name, Genre: sc.Name, Kind: sc.Kind,
		Color: Colors[hash(sc.Key)%uint64(len(Colors))], Active: true}
	c.Number = nextNumber(c, existing, nil)
	return c, s.db.AddGuideChannel(c)
}

// renumber moves the channels numbered outside their blocks into them (numbers.go): first to
// numbers no channel can have, then to their own, so no two hold one number on the way.
func (s *Service) renumber(typed Typed) error {
	all, err := s.db.GuideChannels()
	if err != nil {
		return err
	}
	moves := Renumber(all, typed)
	if len(moves) == 0 {
		return nil
	}
	i := 0
	for key := range moves {
		i++
		if err := s.db.SetGuideChannelNumber(key, -i); err != nil {
			return err
		}
	}
	for key, n := range moves {
		if err := s.db.SetGuideChannelNumber(key, n); err != nil {
			return err
		}
	}
	log.Printf("[GUIDE] Renumbered %d channels into their blocks", len(moves))
	return nil
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
func (s *Service) fillerClips(libs []config.MediaPath, videos *videoList) (map[string][]Clip, error) {
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
	items, err := videos.list()
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

// videoList is the media database's videos, read once for a planning.
type videoList struct {
	db    *database.DB
	items []models.MediaItem
	read  bool
	err   error
}

func (v *videoList) list() ([]models.MediaItem, error) {
	if !v.read {
		v.items, v.err = v.db.ListMediaItems("video")
		v.read = true
	}
	return v.items, v.err
}

// typedContent is a typed definition's titles by group, and its clips (for filler): its movie and TV
// libraries' TMDB titles of its type, and (Late Night) its folder libraries read from their files.
func (s *Service) typedContent(ctx context.Context, cfg *config.Config, def config.TypedChannel, videos *videoList) (map[string][]*Title, []Clip, error) {
	groups := map[string][]*Title{}
	var clips []Clip
	seen := map[string]bool{}
	add := func(group string, lib config.MediaPath, ts ...*Title) {
		for _, t := range ts {
			if seen[t.Key] || len(t.Items) == 0 {
				continue
			}
			seen[t.Key] = true
			t.Library = lib.ID
			groups[group] = append(groups[group], t)
		}
	}
	for _, id := range def.Libraries {
		lib, ok := cfg.LibraryByID(id)
		if !ok {
			continue
		}
		if lib.Type == config.LibraryTypeMovie || lib.Type == config.LibraryTypeTV {
			titles, err := s.titles(ctx, lib)
			if err != nil {
				return nil, nil, err
			}
			for _, t := range titles {
				if !TypedMatch(def.Type, t) {
					continue
				}
				if t.Kind == KindMovie {
					add(TypedMovies, lib, t)
				} else {
					add(TypedSeries, lib, t)
				}
			}
			continue
		}
		if def.Type != TypedLateNight {
			continue // a folder library has no genres
		}
		items, err := videos.list()
		if err != nil {
			return nil, nil, err
		}
		var files []FolderFile
		for _, it := range items {
			if it.Duration <= 0 || media.IsDiscImage(it.Path) || !config.InDir(it.Path, lib.Path) {
				continue
			}
			client, ok := config.ClientPath(it.Path)
			if !ok {
				continue
			}
			files = append(files, FolderFile{Path: client, Duration: time.Duration(it.Duration*1000) * time.Millisecond,
				Thumbnail: it.ThumbnailURL})
		}
		series, long, blocks, c := FolderTitles(lib.ID, lib.Name, files)
		add(TypedSeries, lib, series...)
		add(TypedLong, lib, long...)
		add(TypedClips, lib, blocks...)
		clips = append(clips, c...)
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].Path < clips[j].Path })
	return groups, clips, nil
}

// clipsBesides are the clips that may fill a typed channel's gaps: those not in its own blocks (they'd
// air twice close together: as filler, then in the block).
func clipsBesides(ch Channel, clips []Clip) []Clip {
	own := map[string]bool{}
	for _, t := range ch.Titles {
		for _, it := range t.Items {
			for _, p := range it.Parts {
				own[p.Path] = true
			}
		}
	}
	if len(own) == 0 {
		return clips
	}
	var out []Clip
	for _, c := range clips {
		if !own[c.Path] {
			out = append(out, c)
		}
	}
	return out
}

// takenNames are the names the active channels besides key have.
func takenNames(key string, existing map[string]database.GuideChannel) map[string]bool {
	out := map[string]bool{}
	for k, c := range existing {
		if k != key && c.Active {
			out[c.Name] = true
		}
	}
	return out
}
