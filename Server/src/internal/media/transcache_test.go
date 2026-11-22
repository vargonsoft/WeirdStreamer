package media

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// useSpools runs "transcodes" as a shell script writing the numbers from 1, into a fresh cache.
func useSpools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	saved := transcodeCommand
	transcodeCommand = "sh"
	transcodes.mu.Lock()
	transcodes.dir, transcodes.byID, transcodes.spools = dir, map[string]*transcodeSpool{}, map[*transcodeSpool]bool{}
	transcodes.mu.Unlock()
	t.Cleanup(func() {
		transcodeCommand = saved
		transcodes.reapIdle(time.Now().Add(time.Hour))
	})
	return dir
}

func script(count int) []string { return []string{"-c", "seq 1 " + strconv.Itoa(count)} }

func expected(count int) []byte {
	var b bytes.Buffer
	for i := 1; i <= count; i++ {
		b.WriteString(strconv.Itoa(i) + "\n")
	}
	return b.Bytes()
}

func serve(s *Streamer, file string, args []string, rangeHeader string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/api/media/stream", nil)
	if rangeHeader != "" {
		r.Header.Set("Range", rangeHeader)
	}
	w := httptest.NewRecorder()
	s.runTranscode(w, r, file, args, nil)
	return w
}

func TestTranscodeResumes(t *testing.T) {
	useSpools(t)
	s := &Streamer{}
	args := script(50000)
	whole := expected(50000)
	first := serve(s, "/m/video.mkv", args, "")
	if first.Code != http.StatusOK || !bytes.Equal(first.Body.Bytes(), whole) {
		t.Fatalf("first: %d, %d bytes of %d", first.Code, first.Body.Len(), len(whole))
	}
	resumed := serve(s, "/m/video.mkv", args, "bytes=100000-")
	if resumed.Code != http.StatusPartialContent || !bytes.Equal(resumed.Body.Bytes(), whole[100000:]) {
		t.Fatalf("resumed: %d, %d bytes", resumed.Code, resumed.Body.Len())
	}
	if got := resumed.Header().Get("Content-Range"); !strings.HasPrefix(got, "bytes 100000-") || !strings.HasSuffix(got, "/*") {
		t.Errorf("Content-Range %q", got)
	}
	if past := serve(s, "/m/video.mkv", args, "bytes="+strconv.Itoa(len(whole)+5)+"-"); past.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("past the end: %d", past.Code)
	}
	if other := serve(s, "/m/other.mkv", args, "bytes=100-"); other.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Errorf("a stream that never started: %d", other.Code)
	}
}

func TestViewersDontShareTranscodes(t *testing.T) {
	useSpools(t)
	tracker := NewStreamTracker()
	alice, bob := tracker.Begin("alice", nil), tracker.Begin("bob", nil)
	args := []string{"-c", "head -c 400000000 /dev/zero"}
	first, err := transcodes.attach("/m/a.mkv", args, 0, alice)
	if err != nil {
		t.Fatal(err)
	}
	defer first.detach()
	second, err := transcodes.attach("/m/a.mkv", args, 0, bob)
	if err != nil {
		t.Fatal(err)
	}
	defer second.detach()
	if first == second {
		t.Error("two viewers share a transcode")
	}
	again, err := transcodes.attach("/m/a.mkv", args, 0, alice)
	if err != nil {
		t.Fatal(err)
	}
	defer again.detach()
	if again != first {
		t.Error("a viewer's reconnect didn't find their transcode")
	}
	if _, err := transcodes.attach("/m/b.mkv", args, 5, bob); err == nil {
		t.Error("resumed a stream this viewer never started")
	}
}

func TestTranscodeKeepsNoNames(t *testing.T) {
	dir := useSpools(t)
	serve(&Streamer{}, "/m/Secret Title (2020)/Secret Title.mkv", script(10), "")
	names, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(names) == 0 {
		t.Fatal("nothing in the cache")
	}
	for _, name := range names {
		if strings.Contains(strings.ToLower(name), "secret") || strings.Contains(name, ".mkv") {
			t.Errorf("the cache names the video: %s", name)
		}
		if len(strings.SplitN(filepath.Base(name), "-", 2)[0]) != 64 {
			t.Errorf("not a hash: %s", name)
		}
	}
}

func TestTranscodeWaitsForClients(t *testing.T) {
	useSpools(t)
	args := []string{"-c", "head -c 400000000 /dev/zero"}
	spool, err := transcodes.attach("/m/big.mkv", args, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	size := func() int64 {
		spool.mu.Lock()
		defer spool.mu.Unlock()
		return spool.size
	}
	settle := func() int64 {
		for last := int64(-1); ; time.Sleep(100 * time.Millisecond) {
			now := size()
			if now == last && now > 0 {
				return now
			}
			last = now
		}
	}
	if got := settle(); got > spoolAhead+int64(1<<20) {
		t.Errorf("ran %d bytes ahead", got)
	}
	// Reading on lets it go on, and past the chunks' size, none is half written.
	buf := make([]byte, 1<<20)
	if _, err := spool.read(context.Background(), spoolAhead-1, buf); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := settle(); got <= spoolAhead {
		t.Errorf("didn't go on: %d", got)
	}
	spool.detach()
	if !bytes.Equal(buf[:1], []byte{0}) {
		t.Errorf("read %v", buf[:1])
	}
}

func TestTranscodeDropsWhatsFarBehind(t *testing.T) {
	useSpools(t)
	spool := newTranscodeSpool(filepath.Join(t.TempDir(), "x"), "/m/a.mkv", nil)
	for n := int64(0); n < 12; n++ {
		if err := os.WriteFile(spool.chunk(n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	spool.wanted = spoolBehind + 3*spoolChunk + 1
	spool.stored(spool.wanted)
	if spool.first != 3*spoolChunk {
		t.Fatalf("kept from %d", spool.first)
	}
	if _, err := os.Stat(spool.chunk(2)); !os.IsNotExist(err) {
		t.Error("chunk 2 is still there")
	}
	if _, err := os.Stat(spool.chunk(3)); err != nil {
		t.Error("chunk 3 is gone")
	}
	if spool.join(spoolChunk, nil) {
		t.Error("joined at a dropped byte")
	}
}

func TestAbandonedTranscodeLeaves(t *testing.T) {
	dir := useSpools(t)
	serve(&Streamer{}, "/m/a.mkv", script(100), "")
	transcodes.reapIdle(time.Now())
	if names, _ := filepath.Glob(filepath.Join(dir, "*")); len(names) == 0 {
		t.Fatal("gone before its time")
	}
	transcodes.reapIdle(time.Now().Add(2 * spoolLinger))
	if names, _ := filepath.Glob(filepath.Join(dir, "*")); len(names) != 0 {
		t.Errorf("left %v", names)
	}
}

func TestTranscodeOffset(t *testing.T) {
	for header, want := range map[string]int64{"": 0, "bytes=0-": 0, "bytes=1179648-": 1179648, "bytes=x-": 0, "chunks=5-": 0} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Header.Set("Range", header)
		if got := transcodeOffset(r); got != want {
			t.Errorf("%q: %d", header, got)
		}
	}
}

// A GPU transcode that fails before making anything runs again with the CPU's arguments.
func TestHardwareFallback(t *testing.T) {
	dir := useSpools(t)
	t.Cleanup(func() { hardwareFailed.Store(0) })
	s := newTranscodeSpool(filepath.Join(dir, "fallback"), "/m/a.mkv", nil)
	s.gpu = true
	if err := s.start([]string{"-c", "echo 'No capable devices found' >&2; exit 1"}, script(1000)); err != nil {
		t.Fatal(err)
	}
	defer s.close()
	var got bytes.Buffer
	if err := s.streamFrom(context.Background(), &got, 0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), expected(1000)) {
		t.Errorf("got %d bytes, want the CPU's %d", got.Len(), len(expected(1000)))
	}
	s.mu.Lock()
	gpu := s.gpu
	s.mu.Unlock()
	if gpu || hardwareFailed.Load() == 0 {
		t.Errorf("gpu %v, failure noted %v", gpu, hardwareFailed.Load() != 0)
	}
	if transcodes.hardwareRoom() {
		t.Error("the GPU is tried again right after it failed")
	}
}
