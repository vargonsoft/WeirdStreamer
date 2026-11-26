package media

import (
	"context"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// lowPriorityClass starts a background job's command in the idle priority class, so playback and
// anything else the machine is doing come first (Windows has no nice; its I/O priority follows).
func lowPriorityClass(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.IDLE_PRIORITY_CLASS}
	return cmd
}

// memoryStatusEx is Win32's MEMORYSTATUSEX.
type memoryStatusEx struct {
	length, memoryLoad                        uint32
	totalPhys, availPhys, totalPageFile       uint64
	availPageFile, totalVirtual, availVirtual uint64
	availExtendedVirtual                      uint64
}

var globalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// windowsMemory is the machine's physical memory in bytes (0 when it can't be read).
func windowsMemory() int64 {
	m := memoryStatusEx{length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if ok, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); ok == 0 {
		return 0
	}
	return int64(m.totalPhys)
}
