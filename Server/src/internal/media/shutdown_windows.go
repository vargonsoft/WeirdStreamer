package media

import (
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// StopChildren stops the processes this server started (ffmpeg, ffprobe…) that are still running,
// so none outlives it. Windows has no SIGTERM for them, so they're ended at once: what they were
// writing is a transcode's chunk or a part file, both thrown away anyway. grace is unused.
func StopChildren(grace time.Duration) {
	shuttingDown.Store(true)
	for _, pid := range children() {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			continue
		}
		windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// children lists this process's child processes, from a snapshot of the system's processes.
func children() []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	self := uint32(os.Getpid())
	var out []uint32
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ParentProcessID == self {
			out = append(out, e.ProcessID)
		}
	}
	return out
}
