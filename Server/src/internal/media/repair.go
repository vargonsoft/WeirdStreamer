package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/jobs"
)

// Damaged videos: files whose seek index is broken (often FLV, some with another extension), so
// ffmpeg can't jump into them: players can't skip ahead, transcodes that start past 0 read nothing,
// and thumbnails and trickplay fail. The "Damaged videos" job (after each scan and every 6 hours)
// checks every library video that changed since it last looked, and copies a damaged one's streams,
// untouched, into an indexed Matroska file in media.repairPath, which Playable then gives in its
// place to everything that reads it with ffmpeg (streams, codecs, thumbnails, trickplay, subtitles,
// intros and credits, downloads). The index (repairPath/index.json) is keyed by config.CacheKey.
// An admin can replace the original with its fixed copy (ReplaceWithFixed): the original goes to
// the trash (media.trashPath), emptied after 30 days.

// Repair states
const (
	RepairOK      = "ok"      // seeks fine
	RepairFixed   = "fixed"   // damaged; the fixed copy is in place
	RepairFailed  = "failed"  // damaged, and it couldn't be fixed
	RepairUnknown = "unknown" // not a video ffprobe reads: not looked at again until it changes
)

type repairEntry struct {
	Path      string  `json:"path"` // where it is on disk, for the list
	Size      int64   `json:"size"`
	ModTime   int64   `json:"mtime"`
	State     string  `json:"state"`
	Reason    string  `json:"reason,omitempty"`
	Error     string  `json:"error,omitempty"`     // why it couldn't be fixed
	FixedSize int64   `json:"fixedSize,omitempty"` // the fixed copy's size
	Duration  float64 `json:"duration,omitempty"`  // the fixed copy's real length
	Claimed   float64 `json:"claimed,omitempty"`   // the length the damaged file claims
	Checked   int64   `json:"checked"`
}

var repairIndex struct {
	mu      sync.Mutex
	loaded  bool
	entries map[string]repairEntry
}

func repairDir() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.Media.RepairPath
}

// fixedFile is where a video's fixed copy is kept.
func fixedFile(key string) string { return filepath.Join(repairDir(), key[:2], key+".mkv") }

func loadRepairIndex() {
	if repairIndex.loaded {
		return
	}
	repairIndex.loaded = true
	repairIndex.entries = map[string]repairEntry{}
	if dir := repairDir(); dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "index.json")); err == nil {
			json.Unmarshal(b, &repairIndex.entries)
		}
	}
}

// saveRepairIndex writes the index (with repairIndex.mu held), through a temporary file.
func saveRepairIndex() error {
	dir := repairDir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(repairIndex.entries)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "index.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "index.json"))
}

func repairCurrent(path string, e repairEntry, ok bool) bool {
	if !ok {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() == e.Size && info.ModTime().Unix() == e.ModTime
}

// Playable is the file to read a video from: its fixed copy when it's damaged and has one (and
// hasn't changed since), else the video itself.
func Playable(path string) string {
	if repairDir() == "" || path == "" {
		return path
	}
	key := config.CacheKey(path)
	repairIndex.mu.Lock()
	loadRepairIndex()
	e, ok := repairIndex.entries[key]
	repairIndex.mu.Unlock()
	if !ok || e.State != RepairFixed || !repairCurrent(path, e, ok) {
		return path
	}
	fixed := fixedFile(key)
	if _, err := os.Stat(fixed); err != nil {
		return path
	}
	return fixed
}

// checkSeek says whether a video's seek index is broken: ffprobe drops it as invalid, or a jump
// to the middle finds no video (or video far from there). claimed is the length the file says.
func checkSeek(ctx context.Context, path string) (damaged bool, reason string, claimed float64, err error) {
	cmd := backgroundCommand(ctx, "ffprobe", "-v", "warning", "-show_entries", "format=duration", "-of", "csv=p=0", path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return false, "", 0, fmt.Errorf("ffprobe: %v %s", err, lastLine(stderr.String()))
	}
	claimed, _ = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if strings.Contains(stderr.String(), "invalid index") {
		return true, "Its seek index is broken: players can't jump in it", claimed, nil
	}
	if claimed < 120 {
		return false, "", claimed, nil // too short to matter, and to test
	}
	mid := claimed / 2
	probe := backgroundCommand(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-read_intervals", fmt.Sprintf("%.3f%%+#1", mid), "-show_entries", "packet=pts_time", "-of", "csv=p=0", path)
	got, err := probe.Output()
	if ctx.Err() != nil {
		return false, "", claimed, ctx.Err()
	}
	if err != nil {
		return false, "", claimed, nil // a reading error, not a seek one: left alone
	}
	// The first packet's time ("150.03", or "150.03," for MPEG-TS, whose programs add a field)
	first := strings.TrimSpace(strings.SplitN(string(got), "\n", 2)[0])
	pts, perr := strconv.ParseFloat(strings.TrimSpace(strings.SplitN(first, ",", 2)[0]), 64)
	if perr != nil {
		return true, "Jumping into it finds no video", claimed, nil
	}
	if math.Abs(pts-mid) > math.Max(60, claimed*0.05) {
		return true, fmt.Sprintf("Jumping to %s lands at %s", clock(mid), clock(pts)), claimed, nil
	}
	return false, "", claimed, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func clock(seconds float64) string {
	s := int(seconds)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
}

// fixVideo copies a video's streams, untouched, into an indexed Matroska file (written beside it as
// .part, then renamed), and checks that the copy seeks. Data streams can't go into Matroska;
// subtitles that can't either are left out on a second try.
func fixVideo(ctx context.Context, path, key string) (size int64, duration float64, err error) {
	out := fixedFile(key)
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return 0, 0, err
	}
	part := out + ".part"
	defer os.Remove(part)
	tries := [][]string{
		{"-map", "0", "-dn"},
		{"-map", "0:v", "-map", "0:a?"},
	}
	var lastErr error
	for _, maps := range tries {
		args := append([]string{"-v", "error", "-y", "-fflags", "+genpts", "-i", path}, maps...)
		args = append(args, "-c", "copy", "-f", "matroska", part)
		cmd := backgroundCommand(ctx, "ffmpeg", args...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			lastErr = fmt.Errorf("ffmpeg: %s", lastLine(stderr.String()))
			if ctx.Err() != nil {
				return 0, 0, ctx.Err()
			}
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return 0, 0, lastErr
	}
	damaged, reason, length, err := checkSeek(ctx, part)
	if err != nil {
		return 0, 0, err
	}
	if damaged {
		return 0, 0, fmt.Errorf("the copy doesn't seek either: %s", reason)
	}
	if err := os.Rename(part, out); err != nil {
		return 0, 0, err
	}
	info, err := os.Stat(out)
	if err != nil {
		return 0, 0, err
	}
	return info.Size(), length, nil
}

// RepairJob keeps the fixed copies: after each scan (and every few hours) it checks every video in
// the libraries that changed since it last did, one at a time, waiting while its disk or share is
// busy with playback, and fixes the damaged ones. It also empties the trash of what's been in it
// for TrashDays.
type RepairJob struct {
	db        *database.DB
	streams   *StreamTracker
	InLibrary func(path string) bool
	wake      chan struct{}
	job       *jobs.Job
}

func NewRepairJob(db *database.DB, streams *StreamTracker, inLibrary func(string) bool) *RepairJob {
	j := &RepairJob{db: db, streams: streams, InLibrary: inLibrary, wake: make(chan struct{}, 1),
		job: jobs.Register("repairs", "Damaged videos",
			"Finds videos players can't jump in (a broken seek index) and makes fixed copies to play in their place, listed in Settings › Server. Empties the trash of replaced originals after 30 days.")}
	j.job.OnRunNow(j.Wake)
	return j
}

// Wake has the job look for new videos now (after a scan).
func (j *RepairJob) Wake() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

// Run keeps the fixed copies until ctx ends.
func (j *RepairJob) Run(ctx context.Context) {
	if repairDir() == "" {
		j.job.Schedule("Off: no folder for fixed copies (media.repairPath)")
		return
	}
	j.job.NextIn(4 * time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-j.wake:
	case <-time.After(4 * time.Minute):
	}
	for {
		result := j.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if n, err := EmptyTrash(time.Duration(TrashDays) * 24 * time.Hour); err != nil {
			log.Printf("trash: %v", err)
		} else if n > 0 {
			log.Printf("trash: deleted %d file(s) older than %d days", n, TrashDays)
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

func (j *RepairJob) runOnce(ctx context.Context) string {
	j.job.Begin(0)
	paths, err := j.db.VideoPaths()
	if err != nil {
		log.Printf("repairs: listing videos: %v", err)
	}
	type todo struct{ path, key string }
	var work []todo
	keep := map[string]bool{}
	total := 0
	repairIndex.mu.Lock()
	loadRepairIndex()
	for _, p := range paths {
		if !j.InLibrary(p) || IgnoredMacFile(p) || IsDiscImage(p) || !hasData(p) {
			continue
		}
		key := config.CacheKey(p)
		keep[key] = true
		total++
		if e, ok := repairIndex.entries[key]; repairCurrent(p, e, ok) {
			continue
		}
		work = append(work, todo{p, key})
	}
	// Videos gone from the libraries (or replaced): their fixed copies go too.
	removed := 0
	for key := range repairIndex.entries {
		if !keep[key] {
			os.Remove(fixedFile(key))
			delete(repairIndex.entries, key)
			removed++
		}
	}
	if removed > 0 {
		saveRepairIndex()
	}
	repairIndex.mu.Unlock()

	j.job.SetTotal(total)
	j.job.Step(total - len(work))
	damaged, fixed, failed := 0, 0, 0
	for i, w := range work {
		if !j.job.Wait(ctx, func() (bool, string) { return busyFor(w.path, j.streams) }) {
			break
		}
		j.job.SetRunner(0, filepath.Base(w.path))
		info, statErr := os.Stat(w.path)
		entry := repairEntry{Path: w.path, Checked: time.Now().Unix()}
		if statErr == nil {
			entry.Size, entry.ModTime = info.Size(), info.ModTime().Unix()
		}
		bad, reason, claimed, err := checkSeek(ctx, w.path)
		switch {
		case ctx.Err() != nil:
		case statErr != nil || (err != nil && CheckReadable(w.path) != nil):
			// Gone, or the disk or share failed: looked at again on the next run
			j.job.SetRunner(0, "")
			j.job.Step(1)
			continue
		case err != nil:
			entry.State = RepairUnknown
		case !bad:
			entry.State = RepairOK
			os.Remove(fixedFile(w.key)) // fine now (changed since)
		default:
			damaged++
			entry.Reason, entry.Claimed = reason, claimed
			j.job.SetRunner(0, "Fixing "+filepath.Base(w.path))
			size, length, ferr := fixVideo(ctx, w.path, w.key)
			if ctx.Err() != nil {
				break
			}
			if ferr != nil {
				failed++
				entry.State, entry.Error = RepairFailed, ferr.Error()
				log.Printf("repairs: %s: %s; couldn't fix it: %v", w.path, reason, ferr)
			} else {
				fixed++
				entry.State, entry.FixedSize, entry.Duration = RepairFixed, size, length
				j.redo(w.path, w.key, length)
				log.Printf("repairs: %s: %s; fixed (%s, %s long)", w.path, reason, formatSize(size), clock(length))
			}
		}
		j.job.SetRunner(0, "")
		j.job.Step(1)
		if ctx.Err() != nil {
			break
		}
		repairIndex.mu.Lock()
		repairIndex.entries[w.key] = entry
		if i%25 == 24 || i == len(work)-1 || entry.State != RepairOK {
			if err := saveRepairIndex(); err != nil {
				log.Printf("repairs: saving the index: %v", err)
			}
		}
		repairIndex.mu.Unlock()
	}
	repairIndex.mu.Lock()
	saveRepairIndex()
	repairIndex.mu.Unlock()
	if damaged == 0 {
		return fmt.Sprintf("%s checked, none damaged", plural(len(work), "video"))
	}
	result := fmt.Sprintf("%s checked: %d damaged, %d fixed", plural(len(work), "video"), damaged, fixed)
	if failed > 0 {
		result += fmt.Sprintf(", %d couldn't be", failed)
	}
	return result
}

// redo has what was made from a damaged video made again from its fixed copy: its trickplay
// frames and thumbnail (often few or none: they stop where seeking fails) are removed for the jobs
// and pages to make anew, and its length in the library becomes the copy's real one (the damaged
// header's is often wrong).
func (j *RepairJob) redo(path, key string, length float64) {
	if dir := config.GlobalConfig.Media.TrickplayPath; dir != "" {
		os.RemoveAll(filepath.Join(dir, key))
	}
	if dir := config.GlobalConfig.Media.ThumbnailPath; dir != "" {
		thumb, _ := GetThumbnailPath(dir, key, ".jpg")
		os.Remove(thumb)
	}
	if item, err := j.db.GetMediaItem(path); err == nil && length > 0 {
		item.Duration = length
		if err := j.db.SaveMediaItem(item); err != nil {
			log.Printf("repairs: %s: saving its length: %v", path, err)
		}
	}
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%d KB", b>>10)
}

// DamagedVideo is a damaged library video, for Settings' list.
type DamagedVideo struct {
	Path      string  // on disk
	Reason    string  // what's wrong
	State     string  // RepairFixed or RepairFailed
	Error     string  // why it couldn't be fixed
	Size      int64   // the original's
	FixedSize int64   // the fixed copy's
	Claimed   float64 // the length the original says
	Duration  float64 // the fixed copy's real length
}

// DamagedVideos lists the damaged videos still in the libraries (as the job last saw them), by path.
func DamagedVideos(inLibrary func(string) bool) []DamagedVideo {
	repairIndex.mu.Lock()
	loadRepairIndex()
	var out []DamagedVideo
	for key, e := range repairIndex.entries {
		if (e.State != RepairFixed && e.State != RepairFailed) || !inLibrary(e.Path) || !repairCurrent(e.Path, e, true) {
			continue
		}
		if e.State == RepairFixed {
			if _, err := os.Stat(fixedFile(key)); err != nil {
				continue
			}
		}
		out = append(out, DamagedVideo{Path: e.Path, Reason: e.Reason, State: e.State, Error: e.Error,
			Size: e.Size, FixedSize: e.FixedSize, Claimed: e.Claimed, Duration: e.Duration})
	}
	repairIndex.mu.Unlock()
	sort.Slice(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out
}

// ErrNotFixed: the video has no fixed copy to replace it with.
var ErrNotFixed = errors.New("this video has no fixed copy")

// ErrNameTaken: a file already has the name the fixed copy would take.
var ErrNameTaken = errors.New("a file with the new name is already there")

// ReplaceWithFixed puts a damaged video's fixed copy in its place, named as it with the .mkv
// extension, with the original's time on disk (so it keeps its place in Recently Added), and moves
// the original to the trash. It returns the new file's path.
func ReplaceWithFixed(path string) (string, error) {
	key := config.CacheKey(path)
	repairIndex.mu.Lock()
	loadRepairIndex()
	e, ok := repairIndex.entries[key]
	repairIndex.mu.Unlock()
	fixed := fixedFile(key)
	if !ok || e.State != RepairFixed || !repairCurrent(path, e, ok) {
		return "", ErrNotFixed
	}
	if _, err := os.Stat(fixed); err != nil {
		return "", ErrNotFixed
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	target := strings.TrimSuffix(path, filepath.Ext(path)) + ".mkv"
	if target != path {
		if _, err := os.Stat(target); err == nil {
			return "", ErrNameTaken
		}
	}
	// The copy goes in beside the original first (a .part name, which scans skip), so a failure
	// leaves the original as it was.
	part := target + ".part"
	if err := moveFile(fixed, part); err != nil {
		return "", fmt.Errorf("copying the fixed video into the library: %w", err)
	}
	os.Chtimes(part, info.ModTime(), info.ModTime())
	if err := MoveToTrash(path); err != nil {
		os.Rename(part, fixed) // back where it was
		return "", fmt.Errorf("moving the original to the trash: %w", err)
	}
	if err := os.Rename(part, target); err != nil {
		return "", fmt.Errorf("naming the fixed video: %w", err)
	}
	repairIndex.mu.Lock()
	delete(repairIndex.entries, key)
	saveRepairIndex()
	repairIndex.mu.Unlock()
	log.Printf("repairs: replaced %s with its fixed copy %s; the original is in the trash", path, filepath.Base(target))
	return target, nil
}

// moveFile renames a file, or copies it and removes the original when it's on another disk.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(to)
		return err
	}
	return os.Remove(from)
}

// ---------------------------------------------------------------------------------------------
// The trash: library files replaced by fixed copies, each in a folder named by when it was put
// there ("<unix time>-<n>/<its name>"), deleted after TrashDays or when an admin empties it.

// TrashDays is how long the trash keeps a file.
const TrashDays = 30

func trashDir() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.Media.TrashPath
}

var trashMu sync.Mutex

// MoveToTrash moves a file into the trash.
func MoveToTrash(path string) error {
	dir := trashDir()
	if dir == "" {
		return errors.New("no trash folder (media.trashPath)")
	}
	trashMu.Lock()
	defer trashMu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	now := time.Now().Unix()
	var slot string
	for n := 1; ; n++ {
		slot = filepath.Join(dir, fmt.Sprintf("%d-%d", now, n))
		if err := os.Mkdir(slot, 0o700); err == nil {
			break
		} else if !os.IsExist(err) {
			return err
		}
	}
	if err := moveFile(path, filepath.Join(slot, filepath.Base(path))); err != nil {
		os.Remove(slot)
		return err
	}
	return nil
}

// TrashInfo is what's in the trash: how many files, their size, and when the oldest went in.
type TrashInfo struct {
	Files  int
	Bytes  int64
	Oldest time.Time
}

func trashSlots() (map[string]time.Time, error) {
	entries, err := os.ReadDir(trashDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	slots := map[string]time.Time{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		at, err := strconv.ParseInt(strings.SplitN(e.Name(), "-", 2)[0], 10, 64)
		if err != nil {
			continue
		}
		slots[filepath.Join(trashDir(), e.Name())] = time.Unix(at, 0)
	}
	return slots, nil
}

// Trash says what's in the trash.
func Trash() (TrashInfo, error) {
	var info TrashInfo
	if trashDir() == "" {
		return info, nil
	}
	trashMu.Lock()
	defer trashMu.Unlock()
	slots, err := trashSlots()
	if err != nil {
		return info, err
	}
	for slot, at := range slots {
		files, _ := os.ReadDir(slot)
		for _, f := range files {
			if fi, err := f.Info(); err == nil && !f.IsDir() {
				info.Files++
				info.Bytes += fi.Size()
			}
		}
		if info.Oldest.IsZero() || at.Before(info.Oldest) {
			info.Oldest = at
		}
	}
	return info, nil
}

// EmptyTrash deletes what's been in the trash longer than olderThan (0: everything), returning how
// many files it deleted.
func EmptyTrash(olderThan time.Duration) (int, error) {
	if trashDir() == "" {
		return 0, nil
	}
	trashMu.Lock()
	defer trashMu.Unlock()
	slots, err := trashSlots()
	if err != nil {
		return 0, err
	}
	deleted := 0
	for slot, at := range slots {
		if olderThan > 0 && time.Since(at) < olderThan {
			continue
		}
		files, _ := os.ReadDir(slot)
		if err := os.RemoveAll(slot); err != nil {
			return deleted, err
		}
		deleted += len(files)
	}
	return deleted, nil
}
