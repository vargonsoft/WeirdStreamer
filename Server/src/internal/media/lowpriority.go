package media

import (
	"context"
	"os/exec"
	"sync"
)

// backgroundCommand is a command for a background job (trickplay, intros and credits), run at the
// lowest CPU priority (nice 19) and idle disk priority (ionice -c3), so playback and anything else
// the machine is doing come first; the jobs don't wait for the CPU themselves. ionice and nice
// replace themselves with the command, so it keeps the same process (ctx still stops it). Where
// either isn't installed, the command runs without it.
func backgroundCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	prefix := lowPriorityPrefix()
	if _, err := exec.LookPath(name); err != nil || len(prefix) == 0 {
		// A tool that isn't installed runs as it is, so the error says so (exec.ErrNotFound): through
		// nice it would be nice's exit status 127, which reads like the tool turning the file down.
		return exec.CommandContext(ctx, name, args...)
	}
	all := append(append(append([]string{}, prefix[1:]...), name), args...)
	return exec.CommandContext(ctx, prefix[0], all...)
}

// ffmpegMissing names ffmpeg or ffprobe when one isn't on the PATH ("" when both are). Background
// jobs wait while it does (busyFor) and store nothing about a file then: a server started before
// ffmpeg was installed once marked every video done without frames, intros or styled subtitles.
func ffmpegMissing() string {
	for _, tool := range []string{"ffprobe", "ffmpeg"} {
		if _, err := exec.LookPath(tool); err != nil {
			return tool
		}
	}
	return ""
}

var lowPriority struct {
	once   sync.Once
	prefix []string
}

func lowPriorityPrefix() []string {
	lowPriority.once.Do(func() {
		var p []string
		if path, err := exec.LookPath("ionice"); err == nil {
			p = append(p, path, "-c3")
		}
		if path, err := exec.LookPath("nice"); err == nil {
			p = append(p, path, "-n", "19")
		}
		lowPriority.prefix = p
	})
	return lowPriority.prefix
}
