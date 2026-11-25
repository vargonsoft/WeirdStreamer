package media

import (
	"bufio"
	"fmt"
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
		if limit := cgroupValue("memory.max"); limit > 0 && (total == 0 || limit < total) {
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
	case "windows":
		return windowsMemory()
	}
	return 0
}

var macMemory struct {
	once  sync.Once
	bytes int64
}

// memTotal is /proc/meminfo's MemTotal in bytes (0 when it can't be read).
func memTotal() int64 { return meminfo("MemTotal:") }

// meminfo is one of /proc/meminfo's figures in bytes (0 when it can't be read).
func meminfo(name string) int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		// "MemTotal:        4028500 kB"
		if f := strings.Fields(s.Text()); len(f) >= 2 && f[0] == name {
			kb, _ := strconv.ParseInt(f[1], 10, 64)
			return kb << 10
		}
	}
	return 0
}

// cgroupValue is the lowest of a cgroup (v2) limit file, such as memory.max, for this process's
// cgroup and those above it; -1 when none sets one ("max" everywhere, or no cgroup).
func cgroupValue(file string) int64 {
	b, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return -1
	}
	var group string
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, "0::"); ok {
			group = rest
		}
	}
	if group == "" {
		return -1
	}
	lowest := int64(-1)
	for p := path.Clean(group); ; p = path.Dir(p) {
		if v, err := os.ReadFile(path.Join("/sys/fs/cgroup", p, file)); err == nil {
			if n, err := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64); err == nil && n >= 0 &&
				(lowest < 0 || n < lowest) {
				lowest = n
			}
		}
		if p == "/" || p == "." {
			return lowest
		}
	}
}

// The trickplay job's memory with a 4K HEVC 10-bit video (the most a library usually holds),
// measured with ffmpeg 8 for the Jobs page's estimate: each frame of a batch is an input with a
// decoder of its own, so a batch grows with its frames. Per process, then per frame:
//   - the CPU: 60 MB, then 228 MB (1080p HEVC 10-bit 84 MB a frame, 1080p H.264 45 MB);
//   - VA-API (an Intel GPU, Core Ultra 5 125U): 40 MB, then 370 MB. Its decoder keeps about 15 4K
//     pictures, which an integrated GPU takes from the server's memory (they don't show in ffmpeg's
//     own memory, only in its cgroup's);
//   - CUDA (a GTX 1060): 175 MB, then 124 MB of the server's memory, and 60 MB, then 540 MB of the
//     card's own.
type trickplayCost struct{ process, frame int64 }

var (
	trickplayCPU      = trickplayCost{60 << 20, 228 << 20}
	trickplayVAAPI    = trickplayCost{40 << 20, 370 << 20}
	trickplayCUDA     = trickplayCost{175 << 20, 124 << 20}
	trickplayCUDACard = trickplayCost{60 << 20, 540 << 20}
)

// TrickplayMemory is what the Jobs page shows beside trickplay's settings: the 4K figures its
// estimate is made from, for the decoder trickplay uses now (cpu, vaapi or cuda), the server's
// memory and swap, and for cuda the card's own memory (0 when unknown). The settings are used as
// they are: a batch the kernel kills for memory is made again a frame at a time, and one too big
// for a card is made smaller (gpuDecoder.tooBig).
type TrickplayMemory struct {
	Decoder            string `json:"decoder"`
	ProcessBytes4K     int64  `json:"process_bytes_4k"`
	FrameBytes4K       int64  `json:"frame_bytes_4k"`
	TotalBytes         int64  `json:"total_bytes"`
	SwapBytes          int64  `json:"swap_bytes"`
	CardProcessBytes4K int64  `json:"card_process_bytes_4k,omitempty"`
	CardFrameBytes4K   int64  `json:"card_frame_bytes_4k,omitempty"`
	CardBytes          int64  `json:"card_bytes,omitempty"`
}

func CurrentTrickplayMemory() TrickplayMemory {
	m := TrickplayMemory{Decoder: "cpu", TotalBytes: SystemMemory(), SwapBytes: SystemSwap()}
	cost := trickplayCPU
	if g := chooseTrickplayDecoder(); g != nil {
		m.Decoder = g.hwaccel
		switch g.hwaccel {
		case "vaapi":
			cost = trickplayVAAPI
		case "cuda":
			cost = trickplayCUDA
			m.CardProcessBytes4K, m.CardFrameBytes4K = trickplayCUDACard.process, trickplayCUDACard.frame
			m.CardBytes = cardBytes()
		}
	}
	m.ProcessBytes4K, m.FrameBytes4K = cost.process, cost.frame
	return m
}

var cardBytes = nvidiaCardBytes // tests set their own

// nvidiaCardBytes is the memory of the server's NVIDIA card ("6 GB" in the hardware report), 0 when
// unknown.
func nvidiaCardBytes() int64 {
	for _, g := range Hardware().GPUs {
		var gb float64
		if g.Vendor == "nvidia" {
			if _, err := fmt.Sscanf(g.Memory, "%g GB", &gb); err == nil && gb > 0 {
				return int64(gb * (1 << 30))
			}
		}
	}
	return 0
}

// SystemSwap is the swap this server may use, in bytes: /proc/meminfo's SwapTotal, or its cgroup's
// memory.swap.max when lower; 0 when there's none or it's unknown (macOS's grows as needed).
func SystemSwap() int64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	total := meminfo("SwapTotal:")
	if limit := cgroupValue("memory.swap.max"); limit >= 0 && limit < total {
		return limit
	}
	return total
}
