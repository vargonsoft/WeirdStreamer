package media

import (
	"hash/fnv"
	"strings"
	"sync"
)

// Windows has no device numbers: a folder's "device" is its volume ("C:\", "\\server\share\"),
// numbered by a hash of the volume's root, and remembered with the disk it's on.
var volumes struct {
	mu    sync.Mutex
	root  map[uint64]string // device → volume root
	disks map[uint64]string // device → its physical disk ("" for a share)
}

// folderDevice is the volume a folder is on.
func folderDevice(dir string) (uint64, bool) {
	root, ok := volumeOf(dir)
	if !ok {
		return 0, false
	}
	h := fnv.New64a()
	h.Write([]byte(strings.ToLower(root)))
	dev := h.Sum64()
	volumes.mu.Lock()
	if volumes.root == nil {
		volumes.root = map[uint64]string{}
	}
	volumes.root[dev] = root
	volumes.mu.Unlock()
	return dev, true
}

// wholeDiskOf returns the physical disk (its number, as readDisks names it) a volume is on; for a
// volume over several disks, the first. "" for a network share, or a volume whose disks can't be
// read.
func wholeDiskOf(dev uint64) string {
	volumes.mu.Lock()
	root, known := volumes.root[dev]
	disk, done := volumes.disks[dev]
	volumes.mu.Unlock()
	if !known {
		return ""
	}
	if done {
		return disk
	}
	if !isTimedFS(root) {
		if d := volumeDisks(root); len(d) > 0 {
			disk = d[0]
		}
	}
	volumes.mu.Lock()
	if volumes.disks == nil {
		volumes.disks = map[uint64]string{}
	}
	volumes.disks[dev] = disk
	volumes.mu.Unlock()
	return disk
}
