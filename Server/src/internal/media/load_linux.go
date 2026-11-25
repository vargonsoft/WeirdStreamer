//go:build linux

package media

import "golang.org/x/sys/unix"

// dropCache tells the kernel it can forget the cached pages of a read, so probes measure the disk.
func dropCache(fd int, offset, length int64) {
	unix.Fadvise(fd, offset, length, unix.FADV_DONTNEED)
}

// isTimedFS reports whether a folder is on a mount whose disks /proc/diskstats can't name, so its
// load is judged by timing reads: an SMB or NFS share, or a FUSE file system (a mergerfs pool of
// disks, sshfs, rclone), whose device is no block device.
func isTimedFS(dir string) bool {
	var st unix.Statfs_t
	if unix.Statfs(dir, &st) != nil {
		return false
	}
	switch uint32(st.Type) {
	case unix.SMB2_SUPER_MAGIC, unix.CIFS_SUPER_MAGIC, unix.NFS_SUPER_MAGIC, unix.SMB_SUPER_MAGIC, unix.FUSE_SUPER_MAGIC:
		return true
	}
	return false
}
