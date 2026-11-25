package media

import (
	"runtime"
	"testing"
)

func TestSystemMemory(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("no memory reading here")
	}
	if got := SystemMemory(); got < 256<<20 {
		t.Errorf("SystemMemory() = %d", got)
	}
	if runtime.GOOS == "linux" {
		if memTotal() <= 0 {
			t.Error("MemTotal not read")
		}
		if swap := SystemSwap(); swap < 0 || swap > meminfo("SwapTotal:") {
			t.Errorf("SystemSwap() = %d, SwapTotal %d", swap, meminfo("SwapTotal:"))
		}
	}
	m := CurrentTrickplayMemory()
	if m.FrameBytes4K>>20 != 228 || m.ProcessBytes4K>>20 != 60 {
		t.Errorf("4K figures %d + %d MB a frame, want the measured 60 + 228", m.ProcessBytes4K>>20, m.FrameBytes4K>>20)
	}
}
