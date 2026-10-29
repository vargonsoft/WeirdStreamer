package media

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// StopChildren stops the processes this server started (ffmpeg, ffprobe…) that are still running,
// so none outlives it: asked to stop, then killed after grace.
func StopChildren(grace time.Duration) {
	kids := children()
	for _, pid := range kids {
		syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(grace)
	for len(kids) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		kids = children()
	}
	for _, pid := range kids {
		syscall.Kill(pid, syscall.SIGKILL)
	}
}

// children lists this process's child processes, from /proc (Linux).
func children() []int {
	self := os.Getpid()
	entries, _ := os.ReadDir("/proc")
	var out []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		// pid (comm) state ppid …; comm can hold spaces and parentheses, so read after the last ")".
		s := string(stat)
		fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
		if len(fields) > 1 {
			if ppid, _ := strconv.Atoi(fields[1]); ppid == self {
				out = append(out, pid)
			}
		}
	}
	return out
}

// partPath is where a picture is made before it's renamed to path, so a stop part way never
// leaves a cut-off one under the real name: "frame-10.jpg" becomes "frame-10.part.jpg" (ffmpeg
// picks the format by the extension).
func partPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + ".part" + ext
}

// finishPart renames a finished picture made at partPath(path) to path.
func finishPart(path string) error {
	return os.Rename(partPath(path), path)
}
