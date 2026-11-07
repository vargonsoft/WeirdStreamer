package media

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// Folders' devices, remembered: a busy check runs at every step of a job, for its file and for
// every stream, and a folder doesn't change devices. By folder, not by file: a job goes through
// every file in the libraries, and remembering each one would hold them all.
var devices struct {
	mu    sync.Mutex
	byDir map[string]uint64
}

// deviceOf returns the device a file is on: a disk partition, or a network mount (each has its
// own), so two files are on the same disk or share when their devices match. A file is on its
// folder's device, unless it's a symlink: then it's on the folder of the file it points to.
func deviceOf(path string) (uint64, bool) {
	if path == "" {
		return 0, false
	}
	info, err := os.Lstat(path)
	if err != nil {
		return 0, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if path, err = filepath.EvalSymlinks(path); err != nil {
			return 0, false
		}
	}
	dir := filepath.Dir(path)
	devices.mu.Lock()
	dev, ok := devices.byDir[dir]
	devices.mu.Unlock()
	if ok {
		return dev, true
	}
	info, err = os.Stat(dir)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	devices.mu.Lock()
	if devices.byDir == nil {
		devices.byDir = map[string]uint64{}
	}
	devices.byDir[dir] = uint64(st.Dev)
	devices.mu.Unlock()
	return uint64(st.Dev), true
}

// wholeDiskOf returns the whole disk (as /proc/diskstats names it: sda, nvme0n1) a device is a
// partition of, or the device itself; "" for one that isn't a block device (a network mount).
func wholeDiskOf(dev uint64) string {
	dir, err := filepath.EvalSymlinks(fmt.Sprintf("/sys/dev/block/%d:%d", unix.Major(dev), unix.Minor(dev)))
	if err != nil {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "partition")); err == nil {
		return filepath.Base(filepath.Dir(dir))
	}
	return filepath.Base(dir)
}

// diskLoad is the load on the disk or share a file is on, as a job reading it sees it.
type diskLoad struct {
	known     bool    // which disk or share is known (else the busiest one counts)
	share     bool    // a network share (ms) rather than a local disk (use)
	name      string  // the disk, or the share's library
	use       float64 // a local disk's busy share of the time, 0..1
	ms        float64 // a share's read time
	contended bool    // someone is playing a video from the same disk or share
}

// loadFor finds the disk or share path is on and its load, and whether a stream is reading
// from it too. Without a known disk it falls back to the busiest disk and slowest share, with the
// lower limits (as before this told them apart).
func loadFor(path string, streams *StreamTracker) diskLoad {
	dev, ok := deviceOf(path)
	if !ok {
		return busiest()
	}
	d := diskLoad{known: true}
	if ms, library, ok := load.shareFor(dev); ok {
		d.share, d.name, d.ms = true, library, ms
	} else if name := wholeDiskOf(dev); name != "" {
		use, ok := load.diskUse(name)
		if !ok {
			return busiest() // a disk /proc/diskstats doesn't list as whole (LVM, say)
		}
		d.name, d.use = name, use
	} else {
		return busiest()
	}
	if streams != nil {
		for _, s := range streams.List() {
			if s.State != "playing" && s.State != "streaming" {
				continue
			}
			if sd, ok := deviceOf(s.Path); ok && sd == dev {
				d.contended = true
				break
			}
		}
	}
	return d
}

// busiest is the load to judge by when a file's disk isn't known: the busiest disk and the
// slowest share, at the lower limits.
func busiest() diskLoad {
	d := diskLoad{contended: true}
	if _, disk, ok := load.Readings(); ok {
		d.use = disk
	}
	if sh := load.Shares(); len(sh) > 0 {
		d.share, d.name, d.ms = true, sh[0].Library, sh[0].Ms
		if d.use > maxDisk {
			d.share = false // the disk is over its limit; that's the reason to give
		}
	}
	return d
}

// waitReason says whether a job reading the file should wait for its disk, and why: above the
// lower limits while someone plays from the same disk or share, else only near full.
func (d diskLoad) waitReason() (bool, string) {
	also := ""
	if d.known && d.contended {
		also = ", and someone is playing from it"
	}
	if d.share {
		limit := float64(relaxedShareMs)
		if d.contended {
			limit = maxShareMs
		}
		if d.ms > limit {
			return true, fmt.Sprintf("%s's share busy (reads take %.0f ms%s)", d.name, d.ms, also)
		}
		return false, ""
	}
	limit := relaxedDisk
	if d.contended {
		limit = maxDisk
	}
	if d.use > limit {
		name := "Disk"
		if d.name != "" {
			name = "Disk " + d.name
		}
		return true, fmt.Sprintf("%s busy (%.0f%%%s)", name, d.use*100, also)
	}
	return false, ""
}

// overloaded says whether a disc decode already running on the file should stop.
func (d diskLoad) overloaded() bool {
	if d.share {
		limit := float64(relaxedStopShareMs)
		if d.contended {
			limit = stopShareMs
		}
		return d.ms > limit
	}
	limit := relaxedStopDisk
	if d.contended {
		limit = stopDisk
	}
	return d.use > limit
}
