package guide

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// The music under the Weather channel and the Guide channel's promo cards (docs/GUIDE.md): the
// tracks in guide.musicPath, shuffled each day and played by the clock from midnight, so every
// screen hears the same one, like a station. A <track>.txt beside a track holds its credit line.

var musicExts = []string{".mp3", ".m4a", ".aac", ".ogg", ".oga", ".opus", ".flac", ".wav"}

// MusicTrack is a track of the Guide's music.
type MusicTrack struct {
	Name     string        // its file's name in the folder
	Title    string        // its title tag, else the file's name
	Credit   string        // its .txt's first line, else "Title · Artist"
	Duration time.Duration // its length
}

// MusicDir is the folder the Guide's music is in.
func MusicDir() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.Guide.MusicPath
}

var musicCache struct {
	sync.Mutex
	probed map[string]MusicTrack // by name, size and time
}

// MusicTracks are the folder's tracks by name, each probed once (until it changes).
func MusicTracks() []MusicTrack {
	dir := MusicDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	musicCache.Lock()
	defer musicCache.Unlock()
	if musicCache.probed == nil {
		musicCache.probed = map[string]MusicTrack{}
	}
	var out []MusicTrack
	for _, e := range entries {
		if e.IsDir() || !slices.Contains(musicExts, strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		key := e.Name() + "|" + strconv.FormatInt(info.Size(), 10) + "|" + strconv.FormatInt(info.ModTime().Unix(), 10)
		t, ok := musicCache.probed[key]
		if !ok {
			t, ok = probeTrack(filepath.Join(dir, e.Name()))
			if !ok {
				continue // ffprobe missing or the file unreadable: tried again next time
			}
			musicCache.probed[key] = t
		}
		out = append(out, t)
	}
	return out
}

func probeTrack(path string) (MusicTrack, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration:format_tags=title,artist",
		"-of", "json", path).Output()
	if err != nil {
		return MusicTrack{}, false
	}
	var r struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if json.Unmarshal(b, &r) != nil {
		return MusicTrack{}, false
	}
	secs, err := strconv.ParseFloat(r.Format.Duration, 64)
	if err != nil || secs < 5 {
		return MusicTrack{}, false
	}
	tag := func(name string) string {
		for k, v := range r.Format.Tags {
			if strings.EqualFold(k, name) {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	name := filepath.Base(path)
	t := MusicTrack{Name: name, Title: strings.TrimSuffix(name, filepath.Ext(name)),
		Duration: time.Duration(secs * float64(time.Second))}
	if v := tag("title"); v != "" {
		t.Title = v
	}
	t.Credit = t.Title
	if a := tag("artist"); a != "" {
		t.Credit += " · " + a
	}
	if b, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".txt"); err == nil {
		if line, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n"); line != "" {
			t.Credit = strings.TrimSpace(line)
		}
	}
	return t, true
}

// MusicAt is the track playing at t and when it started: the day's shuffle of tracks, played in
// turn from midnight (a track still playing at midnight stops there).
func MusicAt(t time.Time) (MusicTrack, time.Time, time.Time, bool) {
	return musicAt(MusicTracks(), t)
}

func musicAt(tracks []MusicTrack, t time.Time) (MusicTrack, time.Time, time.Time, bool) {
	if len(tracks) == 0 {
		return MusicTrack{}, time.Time{}, time.Time{}, false
	}
	order := slices.Clone(tracks)
	slices.SortFunc(order, func(a, b MusicTrack) int { return strings.Compare(a.Name, b.Name) })
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	seed := uint64(day.Year()*10000 + int(day.Month())*100 + day.Day())
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	var total time.Duration
	for _, tr := range order {
		total += tr.Duration
	}
	since := t.Sub(day) % total // the playlist loops through the day
	loopStart := t.Add(-since)
	at := loopStart
	for _, tr := range order {
		end := at.Add(tr.Duration)
		if t.Before(end) {
			if next := day.AddDate(0, 0, 1); end.After(next) {
				end = next
			}
			return tr, at, end, true
		}
		at = end
	}
	return order[0], t, t.Add(order[0].Duration), true
}
