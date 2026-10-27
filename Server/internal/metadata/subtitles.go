package metadata

import (
	"path/filepath"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/subtitles"
)

// SubtitleQuery is what to search OpenSubtitles for to find a file's subtitles: its TMDB match
// (a movie, or an episode by its show, season and episode, after any episode fixes), else the
// title and year from its name.
func (s *Service) SubtitleQuery(lib config.MediaPath, path string) subtitles.Query {
	q := FileSubtitleQuery(path)
	if lib.Type == "" {
		return q
	}
	ref, ok := ResolveFile(lib, path)
	if !ok || (ref.Kind == config.LibraryTypeTV && !ref.IsEpisode) {
		if ref, ok = ExtraRef(lib, path); !ok {
			return q
		}
	}
	if ref.Title != "" {
		q.Title, q.Year = ref.Title, ref.Year
	}
	if ref.Kind == config.LibraryTypeTV && ref.IsEpisode {
		q.Season, q.Episode, _ = s.loadEpisodeMappings(ref.LibraryID).apply(ref, path)
	}
	l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key)
	if err != nil || l == nil || l.Status != "matched" || l.TMDBID == 0 {
		return q
	}
	switch {
	case ref.Kind == config.LibraryTypeTV && ref.IsEpisode:
		q.ParentTMDBID = l.TMDBID
	case l.MatchKind() == config.LibraryTypeMovie:
		q.TMDBID = l.TMDBID
	}
	return q
}

// FileSubtitleQuery is the search for a file outside a typed library: the title and year in its
// name, and the season and episode when it names one.
func FileSubtitleQuery(path string) subtitles.Query {
	name := filepath.Base(path)
	var q subtitles.Query
	if season, episode, ok := ParseEpisode(name); ok {
		q.Season, q.Episode = season, episode
		if title, year, _, ok := ParseEpisodeShow(name); ok && title != "" {
			q.Title, q.Year = title, year
			return q
		}
	}
	q.Title, q.Year = ParseTitleYear(name)
	return q
}
