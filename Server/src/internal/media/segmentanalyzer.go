package media

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/jobs"
)

// SegmentAnalysisVersion is stored with each video's segments; raising it has videos looked at
// again (after the analysis improves). Videos a version from creditsRecheckFrom on looked at are
// looked at again only when an episode or movie has no credits, or credits found on black or by
// fingerprint: version 2 finds shorter credits on black, version 3 credits a little less black (a
// crawl of white text, which only the last seconds of black were taken for), version 4 credits on
// black after an episode's fingerprint match (music the episodes share before their credits).
const (
	SegmentAnalysisVersion = 4
	creditsRecheckFrom     = 1
)

// SegmentAnalyzer finds videos' intros and credits in the background, a folder at a time (a TV
// season's episodes are compared with each other), pausing while anyone is streaming.
type SegmentAnalyzer struct {
	db      *database.DB
	streams *StreamTracker
	// LibraryKind says what library a video is in: "tv", "movie" or "" (folders only), and false
	// when it's in none (a leftover row): those are left alone.
	LibraryKind func(path string) (string, bool)
	wake        chan struct{}
	job         *jobs.Job
}

func NewSegmentAnalyzer(db *database.DB, streams *StreamTracker, kind func(string) (string, bool)) *SegmentAnalyzer {
	a := &SegmentAnalyzer{db: db, streams: streams, LibraryKind: kind, wake: make(chan struct{}, 1),
		job: jobs.Register("segments", "Intros and credits", "Finds videos' intros and credits for the Skip buttons, a folder at a time.")}
	a.job.OnRunNow(a.Wake)
	return a
}

// Wake has the analyzer look for new videos now (after a scan).
func (a *SegmentAnalyzer) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Run analyzes until ctx ends: everything not yet analyzed, then again when woken or every few hours.
func (a *SegmentAnalyzer) Run(ctx context.Context) {
	a.job.NextIn(time.Minute)
	select { // let the server settle first
	case <-ctx.Done():
		return
	case <-a.wake:
	case <-time.After(time.Minute):
	}
	for {
		a.pass(ctx)
		a.job.NextIn(6 * time.Hour)
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
		case <-time.After(6 * time.Hour):
		}
	}
}

func (a *SegmentAnalyzer) pass(ctx context.Context) {
	paths, err := a.db.VideoPaths()
	if err != nil {
		log.Printf("segments: listing videos: %v", err)
		return
	}
	done, err := a.db.AnalyzedVersions()
	if err != nil {
		log.Printf("segments: reading progress: %v", err)
		return
	}
	a.job.Begin(0)
	// Folders with a video not analyzed (by this version) yet, each analyzed whole.
	byDir := map[string][]string{}
	var dirs []string
	for _, p := range paths {
		if !analyzable(p) {
			continue
		}
		if _, inLibrary := a.LibraryKind(p); !inLibrary {
			continue
		}
		d := filepath.Dir(p)
		if _, seen := byDir[d]; !seen {
			dirs = append(dirs, d)
		}
		byDir[d] = append(byDir[d], p)
	}
	var todo []string
	for _, d := range dirs {
		for _, p := range byDir[d] {
			key, _ := config.ClientPath(p) // in a library: checked above
			if a.stale(p, done[key]) {
				todo = append(todo, d)
				break
			}
		}
	}
	a.job.SetTotal(len(todo))
	n := 0
	for _, d := range todo {
		if !a.waitIdle(ctx, byDir[d][0]) {
			a.job.End(fmt.Sprintf("Stopped after %d of %d folders", n, len(todo)))
			return
		}
		a.analyzeDir(ctx, byDir[d])
		a.job.Step(1)
		n++
	}
	if n > 0 {
		log.Printf("segments: analyzed %d folder(s)", n)
	}
	a.job.End(plural(n, "folder") + " analyzed")
}

// stale: a video this version should look at again.
func (a *SegmentAnalyzer) stale(path string, done database.Analysis) bool {
	if done.Version >= SegmentAnalysisVersion {
		return false
	}
	if done.Version < creditsRecheckFrom {
		return true
	}
	kind, _ := a.LibraryKind(path)
	return (!done.Credits || done.CreditsSource == "black" || done.CreditsSource == "fingerprint") && (kind == "tv" || kind == "movie")
}

// plural: "1 folder", "3 folders".
func plural(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

// analyzable: a video file analysis can read (not a disc image).
func analyzable(path string) bool {
	return !strings.EqualFold(filepath.Ext(path), ".iso")
}

// waitIdle waits while the disk or share path is on is busy, or the job is paused, so analysis
// doesn't compete with playback.
func (a *SegmentAnalyzer) waitIdle(ctx context.Context, path string) bool {
	return a.job.Wait(ctx, func() (bool, string) { return busyFor(path, a.streams) })
}

// Background jobs share one load monitor (set by SetLoadMonitor); without one, or without
// readings (not Linux), they pause while anyone is streaming instead.
var load *LoadMonitor

// SetLoadMonitor has background jobs run by the machine's load.
func SetLoadMonitor(m *LoadMonitor) { load = m }

// pauseWhileStreaming: the admin's failsafe (Settings → Streaming), jobs waiting while anyone streams.
func pauseWhileStreaming() bool {
	c := config.GlobalConfig
	return c != nil && c.Streaming.PauseJobsWhileStreaming
}

// streaming says whether anyone is playing or being sent a video now.
func streaming(streams *StreamTracker) bool {
	if streams == nil {
		return false
	}
	for _, s := range streams.List() {
		if s.State == "playing" || s.State == "streaming" {
			return true
		}
	}
	return false
}

// busyFor says whether a background job reading path should wait, and why: the disk or network
// share the file is on too busy to share (see loadFor: lower limits while someone plays from the
// same one), or, with no readings, anyone streaming; and while ffmpeg or ffprobe isn't installed
// (ffmpegMissing), whatever the load. The CPU isn't waited for: the jobs run at
// low priority instead (backgroundCommand), so playback gets it first.
func busyFor(path string, streams *StreamTracker) (bool, string) {
	if tool := ffmpegMissing(); tool != "" {
		return true, tool + " isn't installed (or not on the server's PATH)"
	}
	if pauseWhileStreaming() && streaming(streams) {
		return true, "Someone is streaming (paused while streaming)"
	}
	if _, _, ok := load.Readings(); ok {
		return loadFor(path, streams).waitReason()
	}
	if streaming(streams) {
		return true, "Someone is streaming"
	}
	return false, ""
}

// LoadStatus is the machine's load as background jobs judge it, for the Jobs page. Shares are
// each network share's read time (ms), slowest first. Jobs wait above MaxDisk and MaxShareMs on a
// disk or share someone is playing from, and above the Relaxed limits on one nobody is.
type LoadStatus struct {
	Available           bool        `json:"available"`
	CPU                 float64     `json:"cpu"`
	Disk                float64     `json:"disk"`
	MaxDisk             float64     `json:"max_disk"`
	RelaxedDisk         float64     `json:"relaxed_disk"`
	Shares              []ShareLoad `json:"shares"`
	MaxShareMs          float64     `json:"max_share_ms"`
	RelaxedShareMs      float64     `json:"relaxed_share_ms"`
	Streaming           bool        `json:"streaming"`
	PauseWhileStreaming bool        `json:"pause_while_streaming"`
}

// CurrentLoad returns the load readings and whether anyone is streaming.
func CurrentLoad(streams *StreamTracker) LoadStatus {
	cpu, disk, ok := load.Readings()
	return LoadStatus{Available: ok, CPU: cpu, Disk: disk, MaxDisk: maxDisk, RelaxedDisk: relaxedDisk,
		Shares: load.Shares(), MaxShareMs: maxShareMs, RelaxedShareMs: relaxedShareMs,
		Streaming: streaming(streams), PauseWhileStreaming: pauseWhileStreaming()}
}

// overloadedFor says whether a job already reading path should stop: its disk or share well past
// what busyFor allows (its own work counts too), or, with no readings, anyone streaming. The CPU
// doesn't count (see busyFor).
func overloadedFor(path string, streams *StreamTracker) bool {
	if pauseWhileStreaming() && streaming(streams) {
		return true
	}
	if _, _, ok := load.Readings(); ok {
		return loadFor(path, streams).overloaded()
	}
	return streaming(streams)
}

type analyzedVideo struct {
	path              string
	duration          float64
	intro, credits    *database.Span
	opening, ending   []uint32 // fingerprints
	openFrom, endFrom float64
}

func (a *SegmentAnalyzer) analyzeDir(ctx context.Context, paths []string) {
	kind, _ := a.LibraryKind(paths[0])
	videos := make([]*analyzedVideo, 0, len(paths))
	for _, p := range paths {
		v := &analyzedVideo{path: p}
		if item, err := a.db.GetMediaItem(p); err == nil && item.Duration > 0 {
			v.duration = item.Duration
		} else if d, _, _, err := ProbeVideo(p); err == nil {
			v.duration = d
		}
		if !a.waitIdle(ctx, p) {
			return
		}
		videos = append(videos, v) // saved even when it can't be read, so it isn't tried again and again
		if v.duration <= 0 {
			continue
		}
		if intro, credits, err := ChapterSegments(ctx, p); err == nil {
			v.intro = span(intro, "chapters")
			v.credits = span(credits, "chapters")
		}
	}

	// A TV season: the opening and ending each episode shares with the episodes beside it.
	if kind == "tv" && len(videos) >= 2 {
		for _, v := range videos {
			if v.duration <= 0 {
				continue
			}
			if !a.waitIdle(ctx, v.path) {
				return
			}
			// The first quarter (at least 3 minutes, at most 10) and the last (2 to 6 minutes).
			openLen := min(max(v.duration*0.25, 180), v.duration, 600)
			endLen := min(max(v.duration*0.25, 120), v.duration, 360)
			v.openFrom, v.endFrom = 0, v.duration-endLen
			if fp, err := Fingerprint(ctx, v.path, v.openFrom, openLen); err == nil {
				v.opening = fp
			}
			if fp, err := Fingerprint(ctx, v.path, v.endFrom, endLen); err == nil {
				v.ending = fp
			}
		}
		for i, v := range videos {
			for _, j := range []int{i + 1, i - 1, i + 2, i - 2} {
				if j < 0 || j >= len(videos) {
					continue
				}
				w := videos[j]
				if v.intro == nil && len(v.opening) > 0 && len(w.opening) > 0 {
					if mine, _ := SharedSegment(v.opening, w.opening, v.openFrom, w.openFrom, 15, 150); mine != nil {
						v.intro = span(mine, "fingerprint")
					}
				}
				if v.credits == nil && len(v.ending) > 0 && len(w.ending) > 0 {
					if mine, _ := SharedSegment(v.ending, w.ending, v.endFrom, w.endFrom, 15, 600); mine != nil {
						v.credits = span(mine, "fingerprint")
					}
				}
			}
		}
	}

	// Credits still not found: rolling credits on black near the end (movies and episodes). An
	// episode's fingerprint match is checked too: music the episodes share can come minutes before
	// the credits (a recurring cue, a song over the last scene), and credits on black that start
	// later are the ones (For All Mankind: a 16-second cue 3½ minutes before its crawl). Starting
	// them late only shows Skip Credits late; starting them early skips the episode's end.
	if kind == "tv" || kind == "movie" {
		for _, v := range videos {
			fromFingerprint := v.credits != nil && v.credits.Source == "fingerprint"
			if v.credits != nil && !fromFingerprint || v.duration <= 0 {
				continue
			}
			if !a.waitIdle(ctx, v.path) {
				return
			}
			look := min(v.duration*0.15, 300)
			if kind == "movie" {
				look = min(v.duration*0.2, 900)
			}
			if seg, err := BlackCredits(ctx, v.path, v.duration, look); err == nil && seg != nil &&
				(!fromFingerprint || seg.Start > v.credits.Start) {
				v.credits = span(seg, "black")
			}
		}
	}

	if ffmpegMissing() != "" {
		return // gone part way through: nothing read is to be trusted, so it's analyzed again
	}
	for _, v := range videos {
		if ctx.Err() != nil {
			return
		}
		// Credits that end within a few seconds of the end (a fingerprint stops a little short) run to it.
		if c := v.credits; c != nil && c.Source != "chapters" && v.duration-c.End < 5 {
			c.End = v.duration
		}
		key, ok := config.ClientPath(v.path)
		if !ok {
			continue // its library is gone from the settings since
		}
		if err := a.db.SaveAnalysis(key, v.intro, v.credits, SegmentAnalysisVersion); err != nil {
			log.Printf("segments: saving %s: %v", v.path, err)
		}
	}
}

func span(s *Segment, source string) *database.Span {
	if s == nil {
		return nil
	}
	return &database.Span{Start: s.Start, End: s.End, Source: source}
}

// runners is how many trickplay workers work at once: the number set on the Jobs page (a change
// applies at once). Workers beyond it wait without holding a video; one that was part way through
// a video when the number went down keeps it until a runner frees up. The jobs run at low priority
// (backgroundCommand) instead of waiting for the CPU, whose load is mostly their own work.
type runners struct {
	mu    sync.Mutex
	limit int           // the configured number of runners, seen last
	moved chan struct{} // closed and remade when limit changes
	// Every video has been handed out: the runners holding one go on with it (else one parked
	// mid-video would wait for runners that have all finished), and wait as jobs usually do.
	draining bool
}

func newRunners() *runners {
	return &runners{limit: TrickplayRunners(), moved: make(chan struct{})}
}

// update applies a change of the setting (with mu held), waking the runners waiting on it.
func (r *runners) update(j *jobs.Job) {
	if lim := TrickplayRunners(); lim != r.limit {
		r.limit = lim
		r.wake()
		j.SetDetail(r.detail())
	}
}

func (r *runners) wake() {
	close(r.moved)
	r.moved = make(chan struct{})
}

// drain lets every runner finish the video it holds (called once all have been handed out).
func (r *runners) drain() {
	r.mu.Lock()
	r.draining = true
	r.wake()
	r.mu.Unlock()
}

// claim waits until runner w may start another video: while it's beyond the number of runners,
// it waits without holding one (so it can't keep a video from the runners that are working). True
// at once once every video has been handed out; false when ctx ends.
func (r *runners) claim(ctx context.Context, j *jobs.Job, w int) bool {
	for {
		r.mu.Lock()
		r.update(j)
		ok, moved := r.draining || w < r.limit, r.moved
		r.mu.Unlock()
		if ok {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-moved:
		}
	}
}

// detail is the runner count for the Jobs page. Callers hold mu, or call it before the runners
// start.
func (r *runners) detail() string {
	return plural(r.limit, "runner")
}

// wait is runner w's wait before each step of path: it parks while w is beyond the number of
// runners (it went down part way through w's video), and otherwise waits as jobs do (paused, the
// file's disk or share busy, streaming).
func (r *runners) wait(ctx context.Context, j *jobs.Job, streams *StreamTracker, w int, path string) bool {
	for {
		r.mu.Lock()
		r.update(j)
		limit, moved, draining := r.limit, r.moved, r.draining
		r.mu.Unlock()
		if !draining && w >= limit {
			j.RunnerWaiting(w, true)
			select {
			case <-ctx.Done():
				return false
			case <-moved:
			}
			j.RunnerWaiting(w, false)
			continue
		}
		return j.Wait(ctx, func() (bool, string) { return busyFor(path, streams) })
	}
}

// TrickplayJob makes trickplay frames (the pictures over the players' seek bars) in the
// background for every video in a library that doesn't have them all yet, pausing while anyone
// is streaming.
type TrickplayJob struct {
	gen     *TrickplayGenerator
	db      *database.DB
	streams *StreamTracker
	// InLibrary says whether a video is in a library (leftover rows are left alone).
	InLibrary func(path string) bool
	wake      chan struct{}
	job       *jobs.Job
}

func NewTrickplayJob(db *database.DB, streams *StreamTracker, inLibrary func(string) bool) *TrickplayJob {
	j := &TrickplayJob{gen: NewTrickplayGenerator(db), db: db, streams: streams, InLibrary: inLibrary, wake: make(chan struct{}, 1),
		job: jobs.Register("trickplay", "Trickplay frames", "Makes the pictures over the seek bars for videos (and discs) without them, and fills in lengths the library is missing.")}
	j.job.OnRunNow(j.Wake)
	return j
}

// Wake has the job look for new videos now (after a scan).
func (j *TrickplayJob) Wake() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

// Run makes missing frames until ctx ends: everything now, then again when woken or every few hours.
func (j *TrickplayJob) Run(ctx context.Context) {
	j.job.NextIn(2 * time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-j.wake:
	case <-time.After(2 * time.Minute):
	}
	failed := map[string]bool{} // unreadable files, not tried again until a restart
	for {
		j.job.Begin(0)
		paths, err := j.db.VideoPaths()
		if err != nil {
			log.Printf("trickplay: listing videos: %v", err)
		}
		noLength, err := j.db.VideosWithoutLength()
		if err != nil {
			log.Printf("trickplay: listing videos without a length: %v", err)
		}
		// Progress is of every video in the libraries, counting those already done (or given up on
		// until a restart), so it carries on from where the last run got to. A video whose frames
		// are made but that has no length stored gets its length read (making frames stores it
		// too); those go first, being quick.
		var todo, lengthOnly []string
		total, already := 0, 0
		for _, p := range paths {
			// Gone files (cleanup forgets them), empty ones and a Mac's leftovers aren't videos to
			// make frames for.
			if !j.InLibrary(p) || IgnoredMacFile(p) || !hasData(p) {
				continue
			}
			total++
			switch {
			case failed[p]:
				already++
			case HasTrickplay(p) && noLength[p] && !noFramesMade(p):
				// Not a video (marked so, with no frames): nothing to read a length from.
				lengthOnly = append(lengthOnly, p)
			case HasTrickplay(p):
				already++
			default:
				todo = append(todo, p)
			}
		}
		isLengthOnly := make(map[string]bool, len(lengthOnly))
		for _, p := range lengthOnly {
			isLengthOnly[p] = true
		}
		todo = append(lengthOnly, todo...)
		j.job.SetTotal(total)
		j.job.Step(already)
		run := newRunners()
		j.job.SetDetail(run.detail())
		// Up to MaxTrickplayRunners videos at once, as many as the setting says (one ffmpeg keeps only a few cores busy); discs one at a
		// time, since each decodes whole titles.
		n, lengths, skipped := 0, 0, 0
		var mu, discs sync.Mutex
		next := make(chan string)
		var wg sync.WaitGroup
		for w := 0; w < config.MaxTrickplayRunners; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					if !run.claim(ctx, j.job, w) {
						return
					}
					p, ok := <-next
					if !ok {
						return
					}
					var err error
					j.job.SetRunner(w, filepath.Base(p)) // the file it's on, for the Jobs page
					wait := func() bool { return run.wait(ctx, j.job, j.streams, w, p) }
					// A disc's decode stops part way when its disk gets much busier, or the job is paused.
					stop := func() bool { return overloadedFor(p, j.streams) || j.job.IsPaused() }
					if isLengthOnly[p] {
						if !wait() {
							j.job.Step(1)
							continue
						}
						err = j.gen.fillLength(p)
						j.job.Step(1)
						mu.Lock()
						if err == nil {
							lengths++
						} else if ctx.Err() == nil { // not a read cut off by shutting down
							failed[p] = true
							skipped++
							log.Printf("trickplay: %s: no length: %v", p, err)
						}
						mu.Unlock()
						j.job.SetRunner(w, "")
						continue
					}
					if IsDiscImage(p) {
						discs.Lock()
						err = j.gen.generateDisc(ctx, p, wait, stop)
						discs.Unlock()
					} else {
						err = j.gen.generate(ctx, p, wait)
					}
					j.job.Step(1)
					mu.Lock()
					if err == nil {
						n++
					} else if ctx.Err() == nil && !errors.Is(err, errInterrupted) {
						// An unreadable file (the share, or its permissions) may read on the next run, so
						// it isn't given up on until a restart. Other failures are.
						if !errors.Is(err, errUnreadableMedia) {
							failed[p] = true
						}
						skipped++
						log.Printf("trickplay: %s: %v", p, err)
					}
					mu.Unlock()
					j.job.SetRunner(w, "")
				}
			}()
		}
		for _, p := range todo {
			if ctx.Err() != nil {
				break
			}
			next <- p
		}
		close(next)
		run.drain()
		wg.Wait()
		if ctx.Err() != nil {
			return
		}
		if n > 0 {
			log.Printf("trickplay: made frames for %d video(s)", n)
		}
		result := "Frames made for " + plural(n, "video")
		if lengths > 0 {
			result += fmt.Sprintf(", %d lengths filled in", lengths)
		}
		if skipped > 0 {
			result += fmt.Sprintf(", %d unreadable", skipped)
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
