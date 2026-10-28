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

// SegmentAnalysisVersion is stored with each video's segments; raising it has every video looked
// at again (after the analysis improves).
const SegmentAnalysisVersion = 1

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
			if done[p] < SegmentAnalysisVersion {
				todo = append(todo, d)
				break
			}
		}
	}
	a.job.SetTotal(len(todo))
	n := 0
	for _, d := range todo {
		if !a.waitIdle(ctx) {
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

// waitIdle waits while the machine is busy or the job is paused, so analysis doesn't compete
// with playback.
func (a *SegmentAnalyzer) waitIdle(ctx context.Context) bool {
	return a.job.Wait(ctx, func() (bool, string) { return busyReason(a.streams) })
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

// busyReason says whether a background job should wait, and why: the CPU or a disk too busy to
// share (or, with no readings, anyone streaming).
func busyReason(streams *StreamTracker) (bool, string) {
	if pauseWhileStreaming() && streaming(streams) {
		return true, "Someone is streaming (paused while streaming)"
	}
	if cpu, disk, ok := load.Readings(); ok {
		if cpu > maxCPU {
			return true, fmt.Sprintf("CPU busy (%.0f%%)", cpu*100)
		}
		if disk > maxDisk {
			return true, fmt.Sprintf("Disk busy (%.0f%%)", disk*100)
		}
		if sh := load.Shares(); len(sh) > 0 && sh[0].Ms > maxShareMs {
			return true, fmt.Sprintf("%s's share busy (reads take %.0f ms)", sh[0].Library, sh[0].Ms)
		}
		return false, ""
	}
	if streaming(streams) {
		return true, "Someone is streaming"
	}
	return false, ""
}

// LoadStatus is the machine's load as background jobs judge it, for the Jobs page. Shares are
// each network share's read time (ms), slowest first; jobs wait above MaxShareMs.
type LoadStatus struct {
	Available           bool        `json:"available"`
	CPU                 float64     `json:"cpu"`
	Disk                float64     `json:"disk"`
	MaxCPU              float64     `json:"max_cpu"`
	MaxDisk             float64     `json:"max_disk"`
	Shares              []ShareLoad `json:"shares"`
	MaxShareMs          float64     `json:"max_share_ms"`
	Streaming           bool        `json:"streaming"`
	PauseWhileStreaming bool        `json:"pause_while_streaming"`
}

// CurrentLoad returns the load readings and whether anyone is streaming.
func CurrentLoad(streams *StreamTracker) LoadStatus {
	cpu, disk, ok := load.Readings()
	return LoadStatus{Available: ok, CPU: cpu, Disk: disk, MaxCPU: maxCPU, MaxDisk: maxDisk,
		Shares: load.Shares(), MaxShareMs: maxShareMs, Streaming: streaming(streams), PauseWhileStreaming: pauseWhileStreaming()}
}

// overloaded says whether a job already running should stop: well past what busy allows (its own
// work counts too), or, with no readings, anyone streaming.
func overloaded(streams *StreamTracker) bool {
	if pauseWhileStreaming() && streaming(streams) {
		return true
	}
	if cpu, disk, ok := load.Readings(); ok {
		sh := load.Shares()
		return cpu > stopCPU || disk > 0.9 || len(sh) > 0 && sh[0].Ms > stopShareMs
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
		if !a.waitIdle(ctx) {
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
			if !a.waitIdle(ctx) {
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

	// Credits still not found: rolling credits on black near the end (movies and episodes).
	if kind == "tv" || kind == "movie" {
		for _, v := range videos {
			if v.credits != nil || v.duration <= 0 {
				continue
			}
			if !a.waitIdle(ctx) {
				return
			}
			look := min(v.duration*0.15, 300)
			if kind == "movie" {
				look = min(v.duration*0.2, 900)
			}
			if seg, err := BlackCredits(ctx, v.path, v.duration, look); err == nil {
				v.credits = span(seg, "black")
			}
		}
	}

	for _, v := range videos {
		if ctx.Err() != nil {
			return
		}
		// Credits that end within a few seconds of the end (a fingerprint stops a little short) run to it.
		if c := v.credits; c != nil && c.Source != "chapters" && v.duration-c.End < 5 {
			c.End = v.duration
		}
		if err := a.db.SaveAnalysis(v.path, v.intro, v.credits, SegmentAnalysisVersion); err != nil {
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

// trickplayWorkers is how many videos the trickplay job makes frames for at once, at most.
const trickplayWorkers = 3

// runners scales how many trickplay workers work. The job's own ffmpegs are usually what makes
// the CPU busy, so when it passes the limit one runner stops (after the frame it's on) instead of
// all of them pausing; only with one runner left does a busy CPU pause the job. A drop is given
// time to show in the (averaged) reading before another, and a runner is added back once the CPU
// has stayed well under the limit for a while.
type runners struct {
	mu      sync.Mutex
	active  int
	changed time.Time
	moved   chan struct{} // closed and remade when active changes
	// Every video has been handed out: the runners holding one go on with it (else one parked
	// mid-video would wait for runners that have all finished), and wait as jobs usually do.
	draining bool
}

const (
	runnerSettle = 20 * time.Second // after a drop, before another
	runnerRaise  = time.Minute      // under raiseCPU this long since the last change, add one
	raiseCPU     = 0.75 * maxCPU
)

func newRunners() *runners {
	return &runners{active: trickplayWorkers, changed: time.Now(), moved: make(chan struct{})}
}

func (r *runners) set(n int) {
	r.active, r.changed = n, time.Now()
	close(r.moved)
	r.moved = make(chan struct{})
}

// drain lets every runner finish the video it holds (called once all have been handed out).
func (r *runners) drain() {
	r.mu.Lock()
	r.draining = true
	r.set(trickplayWorkers)
	r.mu.Unlock()
}

func (r *runners) detail() string {
	return fmt.Sprintf("%d of %d runners", r.active, trickplayWorkers)
}

// wait is runner w's wait before each step: it parks while w is beyond the active count, scales
// the count by the CPU, and otherwise waits as jobs do (paused, disk busy, streaming).
func (r *runners) wait(ctx context.Context, j *jobs.Job, streams *StreamTracker, w int) bool {
	for {
		r.mu.Lock()
		active, moved, draining := r.active, r.moved, r.draining
		r.mu.Unlock()
		if draining {
			return j.Wait(ctx, func() (bool, string) { return busyReason(streams) })
		}
		if w >= active {
			select {
			case <-ctx.Done():
				return false
			case <-moved:
			}
			continue
		}
		ok := j.Wait(ctx, func() (bool, string) {
			busy, why := busyReason(streams)
			cpu, _, readings := load.Readings()
			r.mu.Lock()
			defer r.mu.Unlock()
			if busy && readings && strings.HasPrefix(why, "CPU") && r.active > 1 {
				// Too busy: one runner fewer, rather than a pause. Until the drop shows in the
				// reading, the rest carry on.
				if time.Since(r.changed) >= runnerSettle {
					r.set(r.active - 1)
					log.Printf("trickplay: %s, down to %s", why, r.detail())
					j.SetDetail(r.detail() + " (CPU busy)")
				}
				return false, ""
			}
			if !busy && readings && cpu < raiseCPU && r.active < trickplayWorkers && time.Since(r.changed) >= runnerRaise {
				r.set(r.active + 1)
				log.Printf("trickplay: CPU at %.0f%%, up to %s", cpu*100, r.detail())
				j.SetDetail(r.detail())
			}
			return busy, why
		})
		if !ok {
			return false
		}
		r.mu.Lock()
		still := w < r.active
		r.mu.Unlock()
		if still {
			return true
		}
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
			case HasTrickplay(p) && noLength[p]:
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
		// A disc's decode stops part way when the machine gets much busier, or the job is paused.
		stop := func() bool { return overloaded(j.streams) || j.job.IsPaused() }
		// trickplayWorkers videos at once (one ffmpeg keeps only a few cores busy); discs one at a
		// time, since each decodes whole titles.
		n, lengths, skipped := 0, 0, 0
		var mu, discs sync.Mutex
		next := make(chan string)
		var wg sync.WaitGroup
		for w := 0; w < trickplayWorkers; w++ {
			wg.Add(1)
			wait := func() bool { return run.wait(ctx, j.job, j.streams, w) }
			go func() {
				defer wg.Done()
				for p := range next {
					var err error
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
						failed[p] = true
						skipped++
						log.Printf("trickplay: %s: %v", p, err)
					}
					mu.Unlock()
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
