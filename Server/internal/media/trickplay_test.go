package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

func TestTrickplayTruncatedFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 400 seconds with the index at the front, then cut to 60% of its size: the index still lists
	// 400 seconds, but the data stops at about 240 (more than 10 frames short).
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10",
		"-t", "400", "-c:v", "libx264", "-g", "10", "-movflags", "+faststart", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	st, _ := os.Stat(src)
	if err := os.Truncate(src, st.Size()*6/10); err != nil {
		t.Fatal(err)
	}

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	tg := NewTrickplayGenerator(nil)
	if err := tg.generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !HasTrickplay(src) {
		t.Error("the truncated file wasn't marked done")
	}
	if _, err := os.Stat(filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(src), "frame-200.jpg")); err != nil {
		t.Error("the frames before the cut weren't kept")
	}
}

func TestTrickplayEmptiedFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 400 seconds, then zeros from 40% of the way in to the end (a download that never finished
	// filling the file it set aside): the size and index are whole, the data past 160s isn't.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10",
		"-t", "400", "-c:v", "libx264", "-g", "10", "-movflags", "+faststart", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	st, _ := os.Stat(src)
	f, err := os.OpenFile(src, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteAt(make([]byte, st.Size()-st.Size()*4/10), st.Size()*4/10)
	f.Close()

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	tg := NewTrickplayGenerator(nil)
	before, _ := os.Stat(src)
	if !tg.stillReadable(context.Background(), src, before, 50, "h264") {
		t.Error("a frame at 50s didn't come out")
	}
	if err := tg.generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !HasTrickplay(src) {
		t.Error("the emptied file wasn't marked done")
	}

	// A file that changed since its frames were started (or that's gone) isn't taken for damaged.
	if err := os.Truncate(src, before.Size()/2); err != nil {
		t.Fatal(err)
	}
	if tg.stillReadable(context.Background(), src, before, 50, "h264") {
		t.Error("a changed file was taken for the same one")
	}
	os.Remove(src)
	if tg.stillReadable(context.Background(), src, before, 50, "h264") {
		t.Error("a missing file was taken for readable")
	}
}

func TestTrickplayStopsAtTheVideosEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 30 seconds of video with 300 of audio: the file's length is the audio's (MP4 and AVI list
	// each stream's length too; MKV doesn't, so it keeps the file's).
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=30",
		"-f", "lavfi", "-i", "sine=duration=300", "-c:v", "libx264", "-c:a", "aac", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	tg := NewTrickplayGenerator(nil)
	if d, codec, err := tg.getVideoInfo(src); err != nil || d > 31 || codec != "h264" {
		t.Errorf("video info: %.1fs %q %v, want about 30s of h264", d, codec, err)
	}
	if err := tg.generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !HasTrickplay(src) {
		t.Error("not marked done")
	}
}

// dataISO writes an ISO 9660 image whose root holds one file, README.TXT (no VIDEO_TS).
func dataISO(t *testing.T, path string) {
	img := make([]byte, 19*sectorSize)
	pvd := img[16*sectorSize:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	root := pvd[156:190]
	root[0] = 34
	binary.LittleEndian.PutUint32(root[2:], 17)
	binary.LittleEndian.PutUint32(root[10:], sectorSize)
	root[25] = 2
	rec := img[17*sectorSize:]
	name := "README.TXT;1"
	rec[0] = byte(33 + len(name) + 1)
	binary.LittleEndian.PutUint32(rec[2:], 18)
	binary.LittleEndian.PutUint32(rec[10:], 5)
	rec[32] = byte(len(name))
	copy(rec[33:], name)
	copy(img[18*sectorSize:], "hello")
	if err := os.WriteFile(path, img, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTrickplayDataDisc(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "installer.iso")
	dataISO(t, iso)

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	if _, err := LoadDisc(iso); !IsNotVideoDisc(err) {
		t.Fatalf("LoadDisc: %v, want not a video disc", err)
	}
	if _, err := LoadDisc(filepath.Join(dir, "gone.iso")); IsNotVideoDisc(err) {
		t.Error("a missing image was taken for a data disc")
	}
	if err := NewTrickplayGenerator(nil).generateDisc(context.Background(), iso, nil, nil); err != nil {
		t.Fatalf("generateDisc: %v", err)
	}
	if !HasTrickplay(iso) || TrickplayTitle(iso) != 0 {
		t.Error("the data disc wasn't marked done without a title")
	}
}

// TestNoLengthInHeader: an ASF saved from a live stream has no length in its header (it's marked
// broadcast), so the length comes from its packets.
func TestNoLengthInHeader(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "live.asf")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=40",
		"-c:v", "wmv2", "-f", "asf", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	// The File Properties object: its play and send durations emptied, and the broadcast flag set.
	b, _ := os.ReadFile(src)
	guid := []byte{0xA1, 0xDC, 0xAB, 0x8C, 0x47, 0xA9, 0xCF, 0x11, 0x8E, 0xE4, 0x00, 0xC0, 0x0C, 0x20, 0x53, 0x65}
	at := bytes.Index(b, guid)
	if at < 0 {
		t.Fatal("no File Properties object")
	}
	clear(b[at+64 : at+80])
	b[at+88] |= 1
	os.WriteFile(src, b, 0o644)
	if out, _ := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", src).Output(); strings.TrimSpace(string(out)) != "N/A" {
		t.Fatalf("ffprobe still reads a length from the header: %q", out)
	}

	if d := PacketDuration(src); d < 39.5 || d > 40.5 {
		t.Errorf("PacketDuration = %.2f, want 40", d)
	}
	if d, _, _, err := ProbeVideo(src); err != nil || d < 39.5 {
		t.Errorf("ProbeVideo = %.2f (%v), want 40", d, err)
	}
	if c, err := ProbeCodecs(src); err != nil || c.Duration < 39.5 {
		t.Errorf("ProbeCodecs: %v (%v), want 40s", c, err)
	}

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}
	// Stored with no length (as older scans left it): making its frames stores the length.
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SaveMediaItem(&models.MediaItem{Path: src, Name: "live.asf", Type: "video", MimeType: "video/x-ms-asf"}); err != nil {
		t.Fatal(err)
	}
	if err := NewTrickplayGenerator(db).generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := os.Stat(filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(src), "frame-30.jpg")); err != nil {
		t.Error("no frame at 30s")
	}
	if item, err := db.GetMediaItem(src); err != nil || item.Duration < 39.5 || item.Duration > 40.5 {
		t.Errorf("stored length %v (%v), want 40", item, err)
	}

	// Its frames made but its length lost: reading it again stores it (and its picture size).
	if err := db.SaveMediaItem(&models.MediaItem{Path: src, Name: "live.asf", Type: "video", MimeType: "video/x-ms-asf"}); err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	log.SetOutput(&logged)
	err = NewTrickplayGenerator(db).fillLength(src)
	log.SetOutput(os.Stderr)
	if err != nil {
		t.Fatalf("fillLength: %v", err)
	}
	if !strings.Contains(logged.String(), "trickplay: "+src+": length filled in (40s)") {
		t.Errorf("log: %q", logged.String())
	}
	if item, err := db.GetMediaItem(src); err != nil || item.Duration < 39.5 || item.Width != 160 {
		t.Errorf("after fillLength: %+v (%v), want 40s at 160 wide", item, err)
	}
}

// TestNotAVideo: a file nothing can read as a video (code named .ts, random bytes) is marked done
// without frames; one that can't be read at all is left to try again.
func TestNotAVideo(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}
	tg := NewTrickplayGenerator(nil)

	code := filepath.Join(dir, "main.ts")
	os.WriteFile(code, []byte("import { createApp } from 'vue'\ncreateApp(App).mount('#app')\n"), 0o644)
	noise := filepath.Join(dir, "noise.mkv")
	b := make([]byte, 256<<10)
	for i := range b {
		b[i] = byte(i*7919 + i/13)
	}
	os.WriteFile(noise, b, 0o644)
	for _, p := range []string{code, noise} {
		if err := tg.generate(context.Background(), p, nil); err != nil {
			t.Errorf("%s: %v, want it marked not a video", filepath.Base(p), err)
		}
		if !HasTrickplay(p) {
			t.Errorf("%s wasn't marked done", filepath.Base(p))
		}
	}

	empty := filepath.Join(dir, "empty.mkv")
	os.WriteFile(empty, nil, 0o644)
	for _, p := range []string{empty, filepath.Join(dir, "gone.mkv")} {
		if err := tg.generate(context.Background(), p, nil); err == nil || HasTrickplay(p) {
			t.Errorf("%s: %v, marked done %v; want an error and no mark", filepath.Base(p), err, HasTrickplay(p))
		}
	}
}

func TestNonDiscISOIsNotVideo(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "installer.iso")
	dataISO(t, iso)

	if !IsNonDiscISO(iso) {
		t.Error("a data disc image wasn't taken for a non-video disc")
	}
	if IsNonDiscISO(filepath.Join(dir, "gone.iso")) {
		t.Error("a missing image was taken for a non-video disc")
	}
	movie := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(movie, []byte("not a disc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsNonDiscISO(movie) {
		t.Error("a non-ISO file was taken for a non-video disc")
	}
}
