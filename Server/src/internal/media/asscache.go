package media

// Styled (ASS/SSA) subtitle tracks, extracted from their videos ahead of time. A track's lines are
// spread through the whole file, so a window read from the middle (format=ass, ffmpeg seeking to
// it) can't see a line that started earlier and is still on screen (a sign, a title). The ASS job
// reads each video with ASS tracks once, after a scan, and keeps every track as a small .ass file
// in <subtitlePath>/ass: windows are then cut from that, with every line on screen during them,
// and a download takes the whole track from it. Until a video's turn comes, windows are read
// from the video as before.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/jobs"
)

// assEntry is what the cache knows of a video: its size and time on disk when it was read (a
// change means reading it again) and its ASS/SSA tracks (indexes among its subtitle tracks;
// none: a video without any). Failed is a file ffprobe or ffmpeg couldn't read (not a video, or a
// broken one): it's tried again only when it changes.
type assEntry struct {
	Size    int64 `json:"size"`
	ModTime int64 `json:"mtime"`
	Tracks  []int `json:"tracks,omitempty"`
	Failed  bool  `json:"failed,omitempty"`
}

var assIndex struct {
	mu      sync.Mutex
	loaded  bool
	entries map[string]assEntry // by the video's path hash
}

// assDir is the cache's folder, or "" with no subtitle folder configured.
func assDir() string {
	if root := config.GlobalConfig.Media.SubtitlePath; root != "" {
		return filepath.Join(root, "ass")
	}
	return ""
}

func assTrackFile(key string, track int) string {
	return filepath.Join(assDir(), key[:2], fmt.Sprintf("%s-%d.ass", key, track))
}

func loadAssIndex() {
	if assIndex.loaded {
		return
	}
	assIndex.loaded = true
	assIndex.entries = map[string]assEntry{}
	if dir := assDir(); dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "index.json")); err == nil {
			json.Unmarshal(b, &assIndex.entries)
		}
	}
}

// saveAssIndex writes the index (with assIndex.mu held), through a temporary file.
func saveAssIndex() error {
	dir := assDir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(assIndex.entries)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "index.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "index.json"))
}

// assCurrent reports whether the cache's entry for path is for the file as it is now.
func assCurrent(path string, e assEntry, ok bool) bool {
	if !ok {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() == e.Size && info.ModTime().Unix() == e.ModTime
}

// CachedAss is the cached file of a video's ASS track (index among its subtitle tracks), or ""
// when it isn't cached (yet) or the video changed since.
func CachedAss(path string, track int) string {
	if assDir() == "" {
		return ""
	}
	key := config.CacheKey(path)
	assIndex.mu.Lock()
	loadAssIndex()
	e, ok := assIndex.entries[key]
	assIndex.mu.Unlock()
	if !assCurrent(path, e, ok) {
		return ""
	}
	for _, t := range e.Tracks {
		if t == track {
			f := assTrackFile(key, track)
			if _, err := os.Stat(f); err == nil {
				return f
			}
		}
	}
	return ""
}

// assTracks lists a video's ASS/SSA tracks (indexes among its subtitle tracks), from its header.
func assTracks(ctx context.Context, path string) ([]int, error) {
	out, err := backgroundCommand(ctx, "ffprobe", "-v", "error", "-select_streams", "s",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", path).Output()
	if err != nil {
		return nil, err
	}
	var tracks []int
	for i, codec := range strings.Fields(string(out)) {
		if IsAssSubtitle(codec) {
			tracks = append(tracks, i)
		}
	}
	return tracks, nil
}

// extractAss reads a video once, writing each of its ASS tracks to the cache, at the file's
// times as the windows give them (-copyts -start_at_zero).
func extractAss(ctx context.Context, path, key string, tracks []int) error {
	args := []string{"-v", "error", "-y", "-i", path, "-copyts", "-start_at_zero"}
	var tmps []string
	for _, t := range tracks {
		f := assTrackFile(key, t)
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			return err
		}
		tmp := f + ".tmp"
		tmps = append(tmps, tmp)
		args = append(args, "-map", "0:s:"+strconv.Itoa(t), "-c:s", "ass", "-f", "ass", tmp)
	}
	var stderr bytes.Buffer
	cmd := backgroundCommand(ctx, "ffmpeg", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		for _, tmp := range tmps {
			os.Remove(tmp)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%v: %s", err, lastLines(stderr.String(), 3))
	}
	for i, t := range tracks {
		if err := os.Rename(tmps[i], assTrackFile(key, t)); err != nil {
			return err
		}
	}
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

// assTime reads an ASS time (H:MM:SS.cc) in seconds.
func assTime(s string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.ParseFloat(parts[0], 64)
	m, err2 := strconv.ParseFloat(parts[1], 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

// AssWindow cuts a window from a whole ASS script: its header (to and with the [Events] Format
// line) and the Dialogue lines on screen at some moment from start to start+duration (duration
// 0: to the end), including those that started before it.
func AssWindow(script []byte, start, duration float64) []byte {
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(script))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	events := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if !events {
			out.WriteString(line + "\n")
			if strings.TrimSpace(line) == "[Events]" {
				events = true
			}
			continue
		}
		if !strings.HasPrefix(line, "Dialogue:") {
			if strings.HasPrefix(line, "Format:") {
				out.WriteString(line + "\n")
			}
			continue
		}
		// Dialogue: Layer, Start, End, …
		fields := strings.SplitN(strings.TrimPrefix(line, "Dialogue:"), ",", 4)
		if len(fields) < 4 {
			continue
		}
		from, ok1 := assTime(fields[1])
		to, ok2 := assTime(fields[2])
		if !ok1 || !ok2 || to <= start || (duration > 0 && from >= start+duration) {
			continue
		}
		out.WriteString(line + "\n")
	}
	return out.Bytes()
}

// ServeAssWindow answers a format=ass request from a whole script (a cached track, or an
// .ass/.ssa file read whole).
func ServeAssWindow(w http.ResponseWriter, script []byte, start, duration float64) {
	w.Header().Set("Content-Type", "text/x-ssa; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(AssWindow(script, start, duration))
}

// AssFileScript reads an .ass/.ssa file whole as an ASS script (ffmpeg turns SSA into ASS and
// reads it in charset): it's small, so every window is cut from all of it.
func AssFileScript(ctx context.Context, path, charset string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-sub_charenc", charset, "-i", path,
		"-map", "0:s:0", "-c:s", "ass", "-f", "ass", "pipe:1").Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AssJob keeps the cache: after each scan (and every few hours) it looks at every video in the
// libraries that changed since it last did, and reads those with ASS tracks, one at a time,
// waiting while their disk or share is busy with playback.
type AssJob struct {
	db        *database.DB
	streams   *StreamTracker
	InLibrary func(path string) bool
	wake      chan struct{}
	job       *jobs.Job
}

func NewAssJob(db *database.DB, streams *StreamTracker, inLibrary func(string) bool) *AssJob {
	j := &AssJob{db: db, streams: streams, InLibrary: inLibrary, wake: make(chan struct{}, 1),
		job: jobs.Register("ass", "Styled subtitles", "Copies the styled (ASS/SSA) subtitle tracks out of videos, so players show every line on screen when they start or jump, and downloads keep them styled.")}
	j.job.OnRunNow(j.Wake)
	return j
}

// Wake has the job look for new videos now (after a scan).
func (j *AssJob) Wake() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

// Run keeps the cache until ctx ends.
func (j *AssJob) Run(ctx context.Context) {
	if assDir() == "" {
		j.job.Schedule("Off: no subtitle folder (media.subtitlePath)")
		return
	}
	j.job.NextIn(3 * time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-j.wake:
	case <-time.After(3 * time.Minute):
	}
	for {
		result := j.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		j.job.End(result)
		j.job.NextIn(6 * time.Hour)
		select {
		case <-ctx.Done():
			return
		case <-j.wake:
		case <-time.After(6 * time.Hour):
		}
	}
}

func (j *AssJob) runOnce(ctx context.Context) string {
	j.job.Begin(0)
	paths, err := j.db.VideoPaths()
	if err != nil {
		log.Printf("ass: listing videos: %v", err)
	}
	type todo struct{ path, key string }
	var work []todo
	keep := map[string]bool{}
	total := 0
	assIndex.mu.Lock()
	loadAssIndex()
	for _, p := range paths {
		if !j.InLibrary(p) || IgnoredMacFile(p) || IsDiscImage(p) || !hasData(p) {
			continue
		}
		key := config.CacheKey(p)
		keep[key] = true
		total++
		if e, ok := assIndex.entries[key]; assCurrent(p, e, ok) {
			continue
		}
		work = append(work, todo{p, key})
	}
	// Videos gone from the libraries: their tracks go too.
	removed := 0
	for key, e := range assIndex.entries {
		if keep[key] {
			continue
		}
		for _, t := range e.Tracks {
			os.Remove(assTrackFile(key, t))
		}
		delete(assIndex.entries, key)
		removed++
	}
	if removed > 0 {
		saveAssIndex()
	}
	assIndex.mu.Unlock()

	j.job.SetTotal(total)
	j.job.Step(total - len(work))
	videos, tracks, skipped := 0, 0, 0
	for i, w := range work {
		if !j.job.Wait(ctx, func() (bool, string) { return busyFor(w.path, j.streams) }) {
			break
		}
		j.job.SetRunner(0, filepath.Base(w.path))
		info, statErr := os.Stat(w.path)
		src := Playable(w.path) // a damaged video's fixed copy
		found, err := assTracks(ctx, src)
		if err == nil && statErr == nil && len(found) > 0 {
			err = extractAss(ctx, src, w.key, found)
		}
		j.job.SetRunner(0, "")
		j.job.Step(1)
		if ctx.Err() != nil {
			break
		}
		if err != nil || statErr != nil {
			skipped++
			if statErr != nil || CheckReadable(w.path) != nil {
				continue // gone, or the disk or share failed: looked at again on the next run
			}
			// The file reads, but isn't a video ffmpeg understands.
			log.Printf("ass: %s: %v (not tried again until it changes)", w.path, err)
			assIndex.mu.Lock()
			for _, t := range assIndex.entries[w.key].Tracks {
				os.Remove(assTrackFile(w.key, t))
			}
			assIndex.entries[w.key] = assEntry{Size: info.Size(), ModTime: info.ModTime().Unix(), Failed: true}
			assIndex.mu.Unlock()
			continue
		}
		assIndex.mu.Lock()
		if old, ok := assIndex.entries[w.key]; ok {
			for _, t := range old.Tracks {
				if !containsInt(found, t) {
					os.Remove(assTrackFile(w.key, t))
				}
			}
		}
		assIndex.entries[w.key] = assEntry{Size: info.Size(), ModTime: info.ModTime().Unix(), Tracks: found}
		if i%25 == 24 || i == len(work)-1 {
			if err := saveAssIndex(); err != nil {
				log.Printf("ass: saving the index: %v", err)
			}
		}
		assIndex.mu.Unlock()
		if len(found) > 0 {
			videos++
			tracks += len(found)
		}
	}
	assIndex.mu.Lock()
	saveAssIndex()
	assIndex.mu.Unlock()
	if videos > 0 {
		log.Printf("ass: copied %d track(s) out of %d video(s)", tracks, videos)
	}
	result := fmt.Sprintf("%s copied out of %s", plural(tracks, "track"), plural(videos, "video"))
	if skipped > 0 {
		result += fmt.Sprintf(", %d unreadable", skipped)
	}
	return result
}

func containsInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}
