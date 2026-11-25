package media

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// Without ffmpeg on the PATH, a background command fails as not found (not as a tool's exit status,
// which reads like the file being turned down), and the jobs' wait names what's missing.
func TestBackgroundJobsWithoutFFmpeg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := ffmpegMissing(); got != "ffprobe" {
		t.Fatalf("ffmpegMissing() = %q, want ffprobe", got)
	}
	err := backgroundCommand(context.Background(), "ffprobe", "-version").Run()
	var exit *exec.ExitError
	if !errors.Is(err, exec.ErrNotFound) || errors.As(err, &exit) {
		t.Fatalf("running a missing ffprobe: %v, want exec.ErrNotFound", err)
	}
	if busy, why := busyFor("/nowhere/video.mkv", nil); !busy || why == "" {
		t.Fatalf("busyFor without ffmpeg = %v %q, want waiting with a reason", busy, why)
	}
}
