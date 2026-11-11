//go:build !linux

package media

// dropCache does nothing off Linux: there is no fadvise, so probes may read cached pages.
func dropCache(fd int, offset, length int64) {}

// isNetworkFS is false off Linux, where the SMB and NFS checks aren't available.
func isNetworkFS(dir string) bool { return false }
