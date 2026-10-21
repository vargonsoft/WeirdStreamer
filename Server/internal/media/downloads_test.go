package media

import (
	"os"
	"os/exec"
	"path/filepath"
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
	if _, err := m.Create(1, src, 100, 100); err == nil {
		t.Error("an original download of a plain file was accepted as a job")
	}
	if _, err := m.Create(1, src, 33, 100); err == nil {
		t.Error("an unknown size was accepted")
	}
	job, err := m.Create(1, src, 25, 50)
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
