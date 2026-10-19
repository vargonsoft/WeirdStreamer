package metadata

import (
	"context"
	"strconv"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

// episodeMappings holds one library's manual episode fixes.
type episodeMappings struct {
	seasons   map[string]database.SeasonMap // "query key|file season"
	overrides map[string][2]int             // file path -> TMDB season, episode
}

func (s *Service) loadEpisodeMappings(libraryID string) episodeMappings {
	seasons, overrides, err := s.db.EpisodeMappings(libraryID)
	if err != nil {
		return episodeMappings{}
	}
	return episodeMappings{seasons: seasons, overrides: overrides}
}

// Episode sources reported by EpisodeInfo.
const (
	MappedFromFile   = "file"   // the file name's own season/episode
	MappedBySeason   = "season" // a season map for this show
	MappedByOverride = "manual" // a per-file override
)

// apply returns the TMDB season and episode a TV ref (an episode file, or a season folder when
// path is a folder) stands for: a per-file override wins, then a season map.
func (m episodeMappings) apply(ref Ref, path string) (season, episode int, source string) {
	if se, ok := m.overrides[path]; ok && ref.IsEpisode {
		return se[0], se[1], MappedByOverride
	}
	if sm, ok := m.seasons[ref.Key+"|"+strconv.Itoa(ref.Season)]; ok {
		episode = ref.Episode
		if ref.IsEpisode {
			episode += sm.EpisodeOffset
		}
		return sm.TMDBSeason, episode, MappedBySeason
	}
	return ref.Season, ref.Episode, MappedFromFile
}

// EpisodeInfo describes which TMDB episode a TV file is shown as, for the fix-match dialog.
type EpisodeInfo struct {
	FileSeason  int    `json:"file_season"`
	FileEpisode int    `json:"file_episode"`
	Season      int    `json:"season"`
	Episode     int    `json:"episode"`
	Source      string `json:"source"`          // "file", "season" or "manual"
	Title       string `json:"title,omitempty"` // the TMDB episode's name, once fetched
}

// EpisodeInfo returns the mapping for an episode file, or nil for other files.
func (s *Service) EpisodeInfo(lib config.MediaPath, path string) *EpisodeInfo {
	ref, ok := ResolveFile(lib, path)
	if !ok || !ref.IsEpisode {
		return nil
	}
	season, episode, source := s.loadEpisodeMappings(lib.ID).apply(ref, path)
	info := &EpisodeInfo{FileSeason: ref.Season, FileEpisode: ref.Episode, Season: season, Episode: episode, Source: source}
	if l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key); err == nil && l != nil && l.Status == "matched" {
		if e, err := s.db.GetEpisode(l.TMDBID, season, episode); err == nil && e != nil {
			info.Title = e.Name
		}
	}
	return info
}

// SetEpisodeMapping maps a TV file to a TMDB season and episode: just this file (wholeSeason
// false), or every file of its season for this show, keeping the same episode shift.
func (s *Service) SetEpisodeMapping(lib config.MediaPath, path string, season, episode int, wholeSeason bool) error {
	ref, ok := ResolveFile(lib, path)
	if !ok || !ref.IsEpisode {
		return ErrNotFixable
	}
	var err error
	if wholeSeason {
		err = s.db.SetSeasonMap(lib.ID, ref.Key, database.SeasonMap{
			FileSeason: ref.Season, TMDBSeason: season, EpisodeOffset: episode - ref.Episode})
		if err == nil {
			// The file follows its season again rather than an older pin.
			err = s.db.DeleteEpisodeOverride(lib.ID, path)
		}
	} else {
		err = s.db.SetEpisodeOverride(lib.ID, path, season, episode)
	}
	if err != nil {
		return err
	}
	s.fetchMappedSeason(ref, season, path)
	return nil
}

// ClearEpisodeMapping removes this file's override, or its season's map.
func (s *Service) ClearEpisodeMapping(lib config.MediaPath, path string, wholeSeason bool) error {
	ref, ok := ResolveFile(lib, path)
	if !ok || !ref.IsEpisode {
		return ErrNotFixable
	}
	if wholeSeason {
		return s.db.DeleteSeasonMap(lib.ID, ref.Key, ref.Season)
	}
	return s.db.DeleteEpisodeOverride(lib.ID, path)
}

// fetchMappedSeason makes sure the TMDB season a mapping points at is stored.
func (s *Service) fetchMappedSeason(ref Ref, season int, path string) {
	if l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key); err == nil && l != nil && l.Status == "matched" {
		s.enqueueSeason(l.TMDBID, season, path)
	}
}

// TMDBSeason is one season of a TMDB show, for the episode picker.
type TMDBSeason struct {
	Number       int    `json:"number"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episode_count"`
}

// TMDBEpisode is one episode of a TMDB season, for the episode picker.
type TMDBEpisode struct {
	Number  int    `json:"number"`
	Name    string `json:"name"`
	AirDate string `json:"air_date,omitempty"`
}

// ShowSeasons lists a TMDB show's seasons.
func (s *Service) ShowSeasons(ctx context.Context, showID int) ([]TMDBSeason, error) {
	client := s.tmdb()
	if client == nil {
		return nil, ErrNoToken
	}
	seasons, err := client.TVSeasons(ctx, showID)
	if err != nil {
		return nil, err
	}
	out := []TMDBSeason{}
	for _, se := range seasons {
		out = append(out, TMDBSeason{Number: se.SeasonNumber, Name: se.Name, EpisodeCount: se.EpisodeCount})
	}
	return out, nil
}

// SeasonEpisodes lists the episodes of a TMDB season.
func (s *Service) SeasonEpisodes(ctx context.Context, showID, season int) ([]TMDBEpisode, error) {
	client := s.tmdb()
	if client == nil {
		return nil, ErrNoToken
	}
	d, err := client.Season(ctx, showID, season)
	if err != nil {
		return nil, err
	}
	out := []TMDBEpisode{}
	for _, e := range d.Episodes {
		out = append(out, TMDBEpisode{Number: e.EpisodeNumber, Name: e.Name, AirDate: e.AirDate})
	}
	return out, nil
}
