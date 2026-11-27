package media

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// TrickplayRunners is how many videos the trickplay job makes frames for at once (live): the Jobs
// page's number, or when that's automatic (0), one per disk the libraries are on (LibraryDisks),
// since a runner's reads are what a disk has to keep up with.
func TrickplayRunners() int {
	if r := config.TrickplayRunnersSetting(); r != 0 {
		return r
	}
	return min(max(LibraryDisks(), 1), config.MaxTrickplayRunners)
}

// TrickplayTierScale is a tier's memory per frame against 4K's (config.TrickplayTiers' order): 1080p's
// measured on the CPU (HEVC 10-bit, 84 MB against 228; H.264 took 45), the smaller ones by their
// pixels with room to spare. A GPU's decoder pictures shrink with the pixels too.
var TrickplayTierScale = []float64{1, 0.37, 0.18, 0.09}

// TrickplayTier is the tier of a width×height video (config.TrickplayTiers): 4K for 1440p and up, and
// for a size not known.
func TrickplayTier(width, height int) int {
	switch resolutionLabel(width, height) {
	case "1080p":
		return 1
	case "720p":
		return 2
	case "576p", "480p", "SD":
		return 3
	}
	return 0
}

// TrickplayFramesPerRunner is how many frames one process makes for a runner on a video of a tier
// (live): the Jobs page's number, or when that's automatic (0), AutoTrickplayFrames for the runners
// in effect.
func TrickplayFramesPerRunner(tier int) int {
	if f := config.TrickplayFramesSetting(tier); f != 0 {
		return f
	}
	return AutoTrickplayFrames(TrickplayRunners(), CurrentTrickplayMemory(), tier)
}

// AutoTrickplayFrames is the most frames per batch with which r runners take half the server's
// memory with videos of a tier (by the estimate for the decoder in use, m, scaled for the tier), and
// on an NVIDIA card fit in 90% of its own memory (the card's is trickplay's to use; the server's is
// what needs care): 1 to the tier's MaxFrames, and at most its MaxProduct at once. With the memory
// unknown, 10.
func AutoTrickplayFrames(r int, m TrickplayMemory, tier int) int {
	r = max(r, 1)
	t, scale := config.TrickplayTiers[tier], TrickplayTierScale[tier]
	f := 10
	if frame := int64(float64(m.FrameBytes4K) * scale); m.TotalBytes > 0 && frame > 0 {
		f = int((m.TotalBytes/2/int64(r) - m.ProcessBytes4K) / frame)
	}
	if frame := int64(float64(m.CardFrameBytes4K) * scale); m.CardBytes > 0 && frame > 0 {
		f = min(f, int((m.CardBytes*9/10/int64(r)-m.CardProcessBytes4K)/frame))
	}
	return min(max(f, 1), t.MaxFrames, max(t.MaxProduct/r, 1))
}

// The libraries' disks, counted at most once a minute: the runners ask at every step.
var libraryDisks struct {
	mu      sync.Mutex
	n       int
	counted time.Time
}

// LibraryDisks is how many disks the libraries are on: whole disks (two partitions of one count
// once), each network or FUSE mount (a mergerfs pool is one), on Windows each drive; libraries
// whose disk can't be told count as one more. At least 1.
func LibraryDisks() int {
	libraryDisks.mu.Lock()
	defer libraryDisks.mu.Unlock()
	if !libraryDisks.counted.IsZero() && time.Since(libraryDisks.counted) < time.Minute {
		return libraryDisks.n
	}
	var paths []string
	if c := config.GlobalConfig; c != nil {
		for _, p := range c.Media.Paths {
			paths = append(paths, p.Path)
		}
	}
	libraryDisks.n, libraryDisks.counted = countDisks(paths), time.Now()
	return libraryDisks.n
}

// countDisks is how many disks the folders are on (LibraryDisks), at least 1.
func countDisks(paths []string) int {
	disks := map[string]bool{}
	for _, p := range paths {
		disks[diskKey(p)] = true
	}
	return max(len(disks), 1)
}

// diskKey names the disk a folder is on: its whole disk, else its device (a network or FUSE
// mount), else its drive (Windows), else "?" (unknown ones count together).
func diskKey(dir string) string {
	if dev, ok := folderDevice(filepath.Clean(dir)); ok {
		if name := wholeDiskOf(dev); name != "" {
			return "disk " + name
		}
		return fmt.Sprintf("device %d", dev)
	}
	if v := filepath.VolumeName(dir); v != "" {
		return "drive " + strings.ToUpper(v)
	}
	return "?"
}
