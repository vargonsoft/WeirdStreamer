package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadEncode(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 8 seconds of busy video at a high bitrate, so the targets are well below the original.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440", "-t", "8", "-c:v", "libx264", "-b:v", "6M", "-c:a", "aac",
		"-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	st, _ := os.Stat(src)

	m := NewDownloadManager(filepath.Join(dir, "jobs"))
	if _, err := m.Create(1, src, 100, 100, true, true); err == nil {
		t.Error("an original download of a plain file was accepted as a job")
	}
	if _, err := m.Create(1, src, 33, 100, false, false); err == nil {
		t.Error("an unknown size was accepted")
	}
	job, err := m.Create(1, src, 25, 50, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Get(2, job.ID) != nil {
		t.Error("another user can see the job")
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		j := m.Get(1, job.ID)
		if j.Status == "ready" || j.Status == "failed" {
			job = j
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("encode did not finish")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if job.Status != "ready" || job.Height != 180 || job.Width != 320 {
		t.Fatalf("job = %+v", job)
	}
	ratio := float64(job.Size) / float64(st.Size())
	if ratio < 0.12 || ratio > 0.4 {
		t.Errorf("25%% target came out at %.0f%% of the original (%d of %d bytes)", ratio*100, job.Size, st.Size())
	}
	file, ok := m.File(1, job.ID)
	if !ok {
		t.Fatal("no file for a ready job")
	}
	if !m.Delete(1, job.ID) {
		t.Fatal("delete failed")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("file left after delete: %v", err)
	}
}

// A compatible copy keeps what it's told to as it is and encodes the rest: here HEVC video kept
// (tagged hvc1 for MP4) and AC-3 5.1 audio made stereo AAC, from the default track.
func TestDownloadKeepsVideo(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mkv")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440", "-f", "lavfi", "-i", "sine=frequency=880", "-t", "3",
		"-map", "0", "-map", "1", "-map", "2", "-c:v", "libx265", "-x265-params", "log-level=error",
		"-c:a:0", "aac", "-c:a:1", "ac3", "-ac:a:1", "6", "-disposition:a:0", "0", "-disposition:a:1", "default",
		"-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	m := NewDownloadManager(filepath.Join(dir, "jobs"))
	if _, err := m.Create(1, src, 50, 100, true, false); err == nil {
		t.Error("a compressed copy that keeps the video was accepted")
	}
	remux, err := m.Create(1, src, 100, 100, true, true)
	if err != nil {
		t.Errorf("keeping the video and audio of an MKV (a copy into MP4) was refused: %v", err)
	} else {
		m.Delete(1, remux.ID)
	}
	job, err := m.Create(1, src, 100, 100, true, false)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for job.Status != "ready" && job.Status != "failed" {
		if time.Now().After(deadline) {
			t.Fatal("encode did not finish")
		}
		time.Sleep(200 * time.Millisecond)
		job = m.Get(1, job.ID)
	}
	if job.Status != "ready" {
		t.Fatalf("job = %+v", job)
	}
	file, _ := m.File(1, job.ID)
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name,codec_tag_string,channels",
		"-of", "csv=p=0", file).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "hevc,hvc1\naac,mp4a,2" {
		t.Errorf("streams = %q; want the HEVC kept (hvc1) and the default AC-3 track as stereo AAC", got)
	}
}

// Starting up clears only encodes left from an earlier run, as the folder may hold other files.
func TestDownloadManagerKeepsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	leftovers := []string{"0123456789abcdef01234567.mp4", "0123456789abcdef01234567.mp4.part"}
	keep := []string{"movie.mp4", "notes.txt", "0123456789abcdef01234567.mkv"}
	for _, name := range append(leftovers, keep...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	NewDownloadManager(dir)
	for _, name := range leftovers {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", name)
		}
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should have been kept: %v", name, err)
		}
	}
}
