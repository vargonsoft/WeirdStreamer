package media

import (
	"bufio"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// SystemMemory is the memory this server may use, in bytes: the machine's (in an LXC container,
// /proc/meminfo shows the container's), or its cgroup's limit when that's lower (a systemd
// MemoryMax, a Docker limit); 0 when unknown. Read each time: a VM's or container's memory can be
// changed while the server runs.
func SystemMemory() int64 {
	switch runtime.GOOS {
	case "linux":
		total := memTotal()
		if limit := cgroupLimit(); limit > 0 && (total == 0 || limit < total) {
			return limit
		}
		return total
	case "darwin":
		macMemory.once.Do(func() {
			if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
				macMemory.bytes, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			}
		})
		return macMemory.bytes
	}
	return 0
}

var macMemory struct {
	once  sync.Once
	bytes int64
}

// memTotal is /proc/meminfo's MemTotal in bytes (0 when it can't be read).
func memTotal() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		// "MemTotal:        4028500 kB"
		if f := strings.Fields(s.Text()); len(f) >= 2 && f[0] == "MemTotal:" {
			kb, _ := strconv.ParseInt(f[1], 10, 64)
			return kb << 10
		}
	}
	return 0
}

// cgroupLimit is the lowest memory.max of this process's cgroup (v2) and those above it, 0 for none.
func cgroupLimit() int64 {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0
	}
	var group string
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "0::"); ok {
			group = rest
		}
	}
	if group == "" {
		return 0
	}
	var lowest int64
	for p := path.Clean(group); ; p = path.Dir(p) {
		if v, err := os.ReadFile(path.Join("/sys/fs/cgroup", p, "memory.max")); err == nil {
			if n, err := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64); err == nil && n > 0 &&
				(lowest == 0 || n < lowest) {
				lowest = n
			}
		}
		if p == "/" || p == "." {
			return lowest
		}
	}
}

// The trickplay job's memory, measured with ffmpeg 8 (one input per frame, each decoder on one
// thread): an ffmpeg process takes about 60 MB, and each frame in its batch holds a decoder of its
// own, which grows with the picture. HEVC, AV1, VP9 and pictures of more than 8 bits keep more:
// 4K HEVC 10-bit 228 MB a frame, 1080p HEVC 10-bit 84 MB, 1080p H.264 45 MB.
const (
	trickplayProcessBytes = 60 << 20
	trickplay4KWidth      = 3840
	trickplay4KHeight     = 2160
)

// trickplayFrameBytes is about what one frame of a batch takes for a video of this size.
func trickplayFrameBytes(width, height int, heavy bool) int64 {
	px := int64(width) * int64(height)
	if heavy {
		return px*25 + 30<<20
	}
	return px*18 + 7<<20
}

// heavyDecoder says whether a video's decoder keeps the larger pictures: HEVC, AV1, VP9, or more
// than 8 bits per sample (pix_fmt yuv420p10le, p010le…).
func heavyDecoder(codec, pixFmt string) bool {
	switch codec {
	case "hevc", "av1", "vp9":
		return true
	}
	return strings.Contains(pixFmt, "10") || strings.Contains(pixFmt, "12")
}

// TrickplayMemory is what the Jobs page shows beside trickplay's settings: the 4K figures its
// estimate is made from (an ffmpeg process, and each frame of its batch, of a 4K HEVC 10-bit
// video), the server's memory and the share of it trickplay keeps within (0s when unknown).
type TrickplayMemory struct {
	ProcessBytes4K int64 `json:"process_bytes_4k"`
	FrameBytes4K   int64 `json:"frame_bytes_4k"`
	TotalBytes     int64 `json:"total_bytes"`
	BudgetBytes    int64 `json:"budget_bytes"`
}

func CurrentTrickplayMemory() TrickplayMemory {
	total := SystemMemory()
	return TrickplayMemory{ProcessBytes4K: trickplayProcessBytes,
		FrameBytes4K: trickplayFrameBytes(trickplay4KWidth, trickplay4KHeight, true),
		TotalBytes:   total, BudgetBytes: trickplayBudget(total)}
}

// trickplayBudget is the memory the trickplay job's runners keep within together: half the
// server's, leaving the rest to streams, the other jobs and the system. 0 (no limit) when unknown.
func trickplayBudget(total int64) int64 {
	return total / 2
}

// trickplayBatchLimit is how many frames a runner's batch may hold for a video of this size, so
// that runners batches like it fit in the budget (at least 1; 0 when there's no budget). The
// Jobs page shows the same sum for a 4K video.
func trickplayBatchLimit(width, height int, heavy bool, runners int, budget int64) int {
	if budget <= 0 || width <= 0 || height <= 0 {
		return 0
	}
	each := budget/int64(max(runners, 1)) - trickplayProcessBytes
	return max(1, int(each/trickplayFrameBytes(width, height, heavy)))
}
