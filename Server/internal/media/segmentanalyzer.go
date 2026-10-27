package media

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
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
}

func NewSegmentAnalyzer(db *database.DB, streams *StreamTracker, kind func(string) (string, bool)) *SegmentAnalyzer {
	return &SegmentAnalyzer{db: db, streams: streams, LibraryKind: kind, wake: make(chan struct{}, 1)}
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
	select { // let the server settle first
	case <-ctx.Done():
		return
	case <-time.After(time.Minute):
	}
	for {
		a.pass(ctx)
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
	n := 0
	for _, d := range dirs {
		todo := false
		for _, p := range byDir[d] {
			if done[p] < SegmentAnalysisVersion {
				todo = true
				break
			}
		}
		if !todo {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		a.analyzeDir(ctx, byDir[d])
		n++
	}
	if n > 0 {
		log.Printf("segments: analyzed %d folder(s)", n)
	}
}

// analyzable: a video file analysis can read (not a disc image).
func analyzable(path string) bool {
	return !strings.EqualFold(filepath.Ext(path), ".iso")
}

// waitIdle waits while anyone is streaming, so analysis doesn't compete with playback.
func (a *SegmentAnalyzer) waitIdle(ctx context.Context) bool {
	return waitForIdle(ctx, a.streams)
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

// busy says whether a background job should wait: the CPU or a disk too busy to share (or, with
// no readings, anyone streaming).
func busy(streams *StreamTracker) bool {
	if pauseWhileStreaming() && streaming(streams) {
		return true
	}
	if cpu, disk, ok := load.Readings(); ok {
		return cpu > maxCPU || disk > maxDisk
	}
	return streaming(streams)
}

// overloaded says whether a job already running should stop: well past what busy allows (its own
// work counts too), or, with no readings, anyone streaming.
func overloaded(streams *StreamTracker) bool {
	if pauseWhileStreaming() && streaming(streams) {
		return true
	}
	if cpu, disk, ok := load.Readings(); ok {
		return cpu > stopCPU || disk > 0.9
	}
	return streaming(streams)
}

// waitForIdle waits while the machine is busy (background jobs mustn't compete with playback);
// false when ctx ends first.
func waitForIdle(ctx context.Context, streams *StreamTracker) bool {
	for busy(streams) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(30 * time.Second):
		}
	}
	return ctx.Err() == nil
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
}

func NewTrickplayJob(db *database.DB, streams *StreamTracker, inLibrary func(string) bool) *TrickplayJob {
	return &TrickplayJob{gen: NewTrickplayGenerator(db), db: db, streams: streams, InLibrary: inLibrary, wake: make(chan struct{}, 1)}
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
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}
	failed := map[string]bool{} // unreadable files, not tried again until a restart
	for {
		paths, err := j.db.VideoPaths()
		if err != nil {
			log.Printf("trickplay: listing videos: %v", err)
		}
		n := 0
		for _, p := range paths {
			if ctx.Err() != nil {
				return
			}
			if failed[p] || !j.InLibrary(p) || HasTrickplay(p) {
				continue
			}
			wait := func() bool { return waitForIdle(ctx, j.streams) }
			var err error
			if IsDiscImage(p) {
				err = j.gen.generateDisc(ctx, p, wait, func() bool { return overloaded(j.streams) })
			} else {
				err = j.gen.generate(ctx, p, wait)
			}
			if err != nil {
				if ctx.Err() == nil && !errors.Is(err, errInterrupted) {
					failed[p] = true
					log.Printf("trickplay: %s: %v", filepath.Base(p), err)
				}
				continue
			}
			n++
		}
		if n > 0 {
			log.Printf("trickplay: made frames for %d video(s)", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-j.wake:
		case <-time.After(6 * time.Hour):
		}
	}
}
