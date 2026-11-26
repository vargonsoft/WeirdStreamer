//go:build !linux && !windows

package media

// dropCache does nothing off Linux: there is no fadvise, so probes may read cached pages.
func dropCache(fd int, offset, length int64) {}

// isTimedFS is false off Linux, where the SMB, NFS and FUSE checks aren't available.
func isTimedFS(dir string) bool { return false }
