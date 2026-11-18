package media

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A fake ffprobe that hangs, first on PATH, with the disc timeout shortened.
func hangingFFprobe(t *testing.T) string {
	t.Helper()
	return fakeFFprobe(t, "1 2 3 4 5 6 7 8 9", "")
}

// fakeFFprobe puts a fake ffprobe first on PATH: titles in hang hang, titles in ok are read (title
// n is n*100 s long), others are missing. Each call's title goes to a log (probeLog reads it). The
// per-probe timeout is 300 ms. It returns a fake disc image (no tables: titles are probed until
// one is missing) and empties the disc cache, with no store.
func fakeFFprobe(t *testing.T, hang, ok string) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
title=""; prev=""
for a in "$@"; do [ "$prev" = "-title" ] && title=$a; prev=$a; done
echo "$title" >> "` + filepath.Join(dir, "log") + `"
case " ` + hang + ` " in *" $title "*) exec sleep 30;; esac
case " ` + ok + ` " in *" $title "*) ;; *) exit 1;; esac
case "$*" in
*read_intervals*) echo 0.0;;
*) echo "{\"format\":{\"duration\":\"${title}00\"},\"chapters\":[],\"streams\":[]}";;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "ffprobe"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	savedTimeout, savedBudget, savedWait, savedStore := discProbeTimeout, discProbeBudget, DiscRequestWait, discStore
	discProbeTimeout = 300 * time.Millisecond
	resetDiscCache()
	discStore = nil
	t.Cleanup(func() {
		discProbeTimeout, discProbeBudget, DiscRequestWait, discStore = savedTimeout, savedBudget, savedWait, savedStore
		resetDiscCache()
	})
	iso := filepath.Join(dir, "disc.iso")
	if err := os.WriteFile(iso, []byte("not really a disc"), 0644); err != nil {
		t.Fatal(err)
	}
	return iso
}

func resetDiscCache() {
	discMu.Lock()
	discCache = map[string]*discEntry{}
	discMu.Unlock()
}

// probeLog is the titles the fake ffprobe was called for, in order.
func probeLog(t *testing.T, iso string) []string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(iso), "log"))
	return strings.Fields(string(b))
}

type memDiscStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (m *memDiscStore) key(path string, size, modTime int64) string {
	return path + "|" + time.Unix(modTime, 0).String() + "|" + string(rune(size))
}

func (m *memDiscStore) GetDiscInfo(path string, size, modTime int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data[m.key(path, size, modTime)], nil
}

func (m *memDiscStore) SaveDiscInfo(path string, size, modTime int64, info []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[m.key(path, size, modTime)] = info
	return nil
}

func useDiscStore() *memDiscStore {
	s := &memDiscStore{data: map[string][]byte{}}
	discStore = s
	return s
}

func titleNumbers(info *DiscInfo) (read, unreadable []int) {
	for _, t := range info.Titles {
		read = append(read, t.Number)
	}
	for _, u := range info.Unreadable {
		unreadable = append(unreadable, u.Number)
	}
	return read, unreadable
}

func sameInts(a []int, b ...int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A title whose ffprobe hangs is given up on at the deadline, instead of holding up the scan.
func TestProbeTitleGivesUpOnHang(t *testing.T) {
	iso := hangingFFprobe(t)
	start := time.Now()
	_, err := probeTitle(context.Background(), iso, 5, false)
	if !errors.Is(err, errProbeTimeout) {
		t.Fatalf("got %v, want a timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %v: the probe wasn't stopped at its deadline", elapsed)
	}
}

// The chapter check gives up on a hang too, and reports it rather than calling the title empty.
func TestReadsPacketGivesUpOnHang(t *testing.T) {
	iso := hangingFFprobe(t)
	start := time.Now()
	readable, err := readsPacket(context.Background(), iso, 5, 1)
	if readable || !errors.Is(err, errProbeTimeout) {
		t.Fatalf("got readable=%v err=%v, want a timeout", readable, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %v: the probe wasn't stopped at its deadline", elapsed)
	}
}

// A command that exited by itself isn't taken for one killed at its deadline, even when the
// deadline has passed by the time it's looked at.
func TestExitedCommandIsNotATimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")
	cmd.Run()
	cancel()
	if killedAtDeadline(cmd, ctx) {
		t.Fatal("an exited command was taken for a killed one")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	cmd = exec.CommandContext(ctx2, "sleep", "30")
	cmd.Run()
	if !killedAtDeadline(cmd, ctx2) {
		t.Fatal("a killed command wasn't seen as killed")
	}
}

// A stuck title is saved as unreadable with the titles that work, and isn't probed again while
// the disc is unchanged; a changed disc is probed again.
func TestStuckTitleSavedNotProbedAgain(t *testing.T) {
	iso := fakeFFprobe(t, "2", "1 3")
	useDiscStore()
	info, err := LoadDisc(iso)
	if err != nil {
		t.Fatal(err)
	}
	read, unreadable := titleNumbers(info)
	if !sameInts(read, 1, 3) || !sameInts(unreadable, 2) || info.MainTitle != 3 {
		t.Fatalf("read %v, unreadable %v, main %d", read, unreadable, info.MainTitle)
	}
	calls := len(probeLog(t, iso))

	resetDiscCache() // as at the next start
	again, err := LoadDisc(iso)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(probeLog(t, iso)); n != calls {
		t.Fatalf("probed again: %d calls, then %d", calls, n)
	}
	read, unreadable = titleNumbers(again)
	if !sameInts(read, 1, 3) || !sameInts(unreadable, 2) {
		t.Fatalf("stored: read %v, unreadable %v", read, unreadable)
	}

	// No retry while unchanged; a new size or time makes it a new disc, probed again.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(iso, later, later); err != nil {
		t.Fatal(err)
	}
	resetDiscCache()
	if _, err := LoadDisc(iso); err != nil {
		t.Fatal(err)
	}
	if n := len(probeLog(t, iso)); n <= calls {
		t.Fatal("a changed disc wasn't probed again")
	}
}

// Probing a disc stops when its budget runs out; the next start goes on from the title it
// stopped at, keeping what was found.
func TestProbeBudget(t *testing.T) {
	iso := fakeFFprobe(t, "1 2", "3")
	discProbeBudget = 500 * time.Millisecond
	useDiscStore()
	start := time.Now()
	_, err := LoadDisc(iso)
	if !errors.Is(err, errProbeBudget) {
		t.Fatalf("got %v, want the budget's error", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %v with a budget of %v", elapsed, discProbeBudget)
	}
	if got := probeLog(t, iso); strings.Join(got, " ") != "1 2" {
		t.Fatalf("probed %v, want titles 1 and 2 only", got)
	}

	resetDiscCache()
	info, err := LoadDisc(iso)
	if err != nil {
		t.Fatal(err)
	}
	read, unreadable := titleNumbers(info)
	if !sameInts(read, 3) || !sameInts(unreadable, 1, 2) || info.NextTitle != 0 {
		t.Fatalf("read %v, unreadable %v, next %d", read, unreadable, info.NextTitle)
	}
	if got := probeLog(t, iso); got[2] != "2" {
		t.Fatalf("the second pass didn't start at title 2: %v", got)
	}
}

// A title that couldn't be read and may be longer than the main title is flagged.
func TestMainTitleUncertain(t *testing.T) {
	info := &DiscInfo{Titles: []DiscTitle{{Number: 1, Duration: 3600}, {Number: 2, Duration: 600}}, MainTitle: 1}
	if mainTitleUncertain(info) {
		t.Fatal("flagged with every title read")
	}
	info.Unreadable = []UnreadableTitle{{Number: 3, Duration: 1200}}
	if mainTitleUncertain(info) {
		t.Fatal("flagged for a shorter unreadable title")
	}
	info.Unreadable = append(info.Unreadable, UnreadableTitle{Number: 4, Duration: 7200})
	if !mainTitleUncertain(info) {
		t.Fatal("not flagged for a longer unreadable title")
	}
	info.Unreadable = []UnreadableTitle{{Number: 4}}
	if !mainTitleUncertain(info) {
		t.Fatal("not flagged for an unreadable title of unknown length")
	}
	info.Unreadable, info.NextTitle = nil, 3
	if !mainTitleUncertain(info) {
		t.Fatal("not flagged with titles left to probe")
	}
}

// A request doesn't wait for a disc being probed beyond its bound; the probe goes on.
func TestLoadDiscWithinDoesNotWait(t *testing.T) {
	iso := fakeFFprobe(t, "1", "")
	discProbeTimeout = time.Second
	start := time.Now()
	_, err := LoadDiscWithin(iso, 200*time.Millisecond)
	if !errors.Is(err, ErrDiscReading) {
		t.Fatalf("got %v, want ErrDiscReading", err)
	}
	if elapsed := time.Since(start); elapsed > 700*time.Millisecond {
		t.Fatalf("waited %v", elapsed)
	}
	LoadDisc(iso) // let the probe end before the test's files go
	if _, err := LoadDiscWithin(iso, 0); errors.Is(err, ErrDiscReading) {
		t.Fatal("the finished probe wasn't kept")
	}
}

func shortStallLimit(t *testing.T) {
	saved := ffmpegStallLimit
	ffmpegStallLimit = 300 * time.Millisecond
	t.Cleanup(func() { ffmpegStallLimit = saved })
}

// A disc transcode whose ffmpeg makes nothing is stopped, and the request gets the error instead
// of waiting forever.
func TestDiscTranscodeWatchdog(t *testing.T) {
	useSpools(t)
	shortStallLimit(t)
	r := httptest.NewRequest("GET", "/api/media/stream", nil)
	w := httptest.NewRecorder()
	start := time.Now()
	err := (&Streamer{}).runTranscode(w, r, "/m/disc.iso", []string{"-c", "exec sleep 30", "sh", "-f", "dvdvideo"}, nil)
	if !errors.Is(err, ErrFFmpegStalled) {
		t.Fatalf("got %v, want ErrFFmpegStalled", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %v", elapsed)
	}
	if w.Body.Len() > 0 {
		t.Fatal("a response was started")
	}
}

// The watch doesn't fire while paused (the transcode holding ffmpeg back on purpose).
func TestStallWatchPaused(t *testing.T) {
	shortStallLimit(t)
	fired := make(chan bool, 1)
	w := watchStalls(func() { fired <- true })
	defer w.done()
	w.pause(true)
	select {
	case <-fired:
		t.Fatal("fired while paused")
	case <-time.After(3 * ffmpegStallLimit):
	}
	w.pause(false)
	select {
	case <-fired:
	case <-time.After(10 * ffmpegStallLimit):
		t.Fatal("didn't fire after the pause")
	}
}

// A disc's trickplay ffmpeg that prints nothing is stopped by the watchdog.
func TestTrickplayDiscWatchdog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	shortStallLimit(t)
	title := &DiscTitle{Number: 1, Duration: 100, Segments: []DiscSegment{{Duration: 100}}}
	start := time.Now()
	err := (&TrickplayGenerator{}).discSegmentFrames(context.Background(), filepath.Join(dir, "disc.iso"), title, 0, title.Segments[0], dir, nil)
	if !errors.Is(err, ErrFFmpegStalled) {
		t.Fatalf("got %v, want ErrFFmpegStalled", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("took %v", elapsed)
	}
}
