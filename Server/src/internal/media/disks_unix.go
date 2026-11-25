//go:build unix

package media

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// folderDevice is the device a folder is on (its stat's st_dev).
func folderDevice(dir string) (uint64, bool) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
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
