package media

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")

// readCPU returns the busy and total CPU time of all CPUs (100 ns units): GetSystemTimes, whose
// kernel time includes the idle time.
func readCPU() (busy, total uint64, err error) {
	var idle, kernel, user windows.Filetime
	if ok, _, e := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user))); ok == 0 {
		return 0, 0, e
	}
	ft := func(f windows.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	total = ft(kernel) + ft(user)
	return total - ft(idle), total, nil
}

// diskPerformance is DISK_PERFORMANCE (times in 100 ns units).
type diskPerformance struct {
	BytesRead, BytesWritten, ReadTime, WriteTime, IdleTime int64
	ReadCount, WriteCount, QueueDepth, SplitCount          uint32
	QueryTime                                              int64
	StorageDeviceNumber                                    uint32
	StorageManagerName                                     [8]uint16
}

const (
	ioctlDiskPerformance            = 0x70020  // IOCTL_DISK_PERFORMANCE
	ioctlVolumeGetVolumeDiskExtents = 0x560000 // IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS
	maxPhysicalDrives               = 32
)

// readDisks returns each physical disk's milliseconds spent doing I/O, named by its number ("0" for
// \\.\PhysicalDrive0, as Disk Management numbers them): the disk's counters' time less its idle
// time. Opening a disk with no access needs no admin rights.
func readDisks() (map[string]uint64, error) {
	out := map[string]uint64{}
	var lastErr error
	for n := 0; n < maxPhysicalDrives; n++ {
		perf, err := diskCounters(n)
		if err != nil {
			lastErr = err
			continue
		}
		out[strconv.Itoa(n)] = uint64(max(perf.QueryTime-perf.IdleTime, 0) / 10000)
	}
	if len(out) == 0 {
		return nil, lastErr
	}
	return out, nil
}

func diskCounters(n int) (diskPerformance, error) {
	var perf diskPerformance
	name, _ := windows.UTF16PtrFromString(`\\.\PhysicalDrive` + strconv.Itoa(n))
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return perf, err
	}
	defer windows.CloseHandle(h)
	var got uint32
	err = windows.DeviceIoControl(h, ioctlDiskPerformance, nil, 0, (*byte)(unsafe.Pointer(&perf)),
		uint32(unsafe.Sizeof(perf)), &got, nil)
	return perf, err
}

// dropCache does nothing on Windows (no fadvise): share probes may read cached pages, which the
// random places they read from make rare in a big file.
func dropCache(fd int, offset, length int64) {}

// isTimedFS reports whether a folder is on a network share (a UNC path or a mapped drive): its
// disks can't be read, so its load is judged by timing reads.
func isTimedFS(dir string) bool {
	root, ok := volumeOf(dir)
	return ok && windows.GetDriveType(windows.StringToUTF16Ptr(root)) == windows.DRIVE_REMOTE
}

// volumeOf is the root of the volume a folder is on ("C:\", "\\server\share\", or the folder a
// volume is mounted on).
func volumeOf(dir string) (string, bool) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return "", false
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

// volumeDisks returns the physical disks (their numbers) a volume is on: one, or more for a volume
// spanning disks; none for a share.
func volumeDisks(root string) []string {
	// The volume itself is opened by its root without the trailing backslash: \\.\C:, or by its
	// GUID name for one mounted on a folder.
	target := root
	if len(root) == 3 && root[1] == ':' {
		target = `\\.\` + root[:2]
	} else {
		r, _ := windows.UTF16PtrFromString(root)
		buf := make([]uint16, 64)
		if err := windows.GetVolumeNameForVolumeMountPoint(r, &buf[0], uint32(len(buf))); err != nil {
			return nil
		}
		target = windows.UTF16ToString(buf)
		target = target[:len(target)-1] // \\?\Volume{…}
	}
	name, _ := windows.UTF16PtrFromString(target)
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)
	// VOLUME_DISK_EXTENTS: a count, then DISK_EXTENTs of a disk number and two 64-bit offsets.
	buf := make([]uint64, 1+3*16)
	var got uint32
	if err := windows.DeviceIoControl(h, ioctlVolumeGetVolumeDiskExtents, nil, 0, (*byte)(unsafe.Pointer(&buf[0])),
		uint32(len(buf)*8), &got, nil); err != nil {
		return nil
	}
	count := min(int(uint32(buf[0])), 16)
	var disks []string
	for i := range count {
		disks = append(disks, strconv.Itoa(int(uint32(buf[1+3*i]))))
	}
	return disks
}
