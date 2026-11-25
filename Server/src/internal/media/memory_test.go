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

// The estimate's figures are the decoder's: the CPU's, an Intel GPU's (whose buffers are the server's
// memory) or an NVIDIA card's (the server's memory, and the card's own).
func TestTrickplayMemoryByDecoder(t *testing.T) {
	defer func() { chooseTrickplayDecoder, cardBytes = trickplayDecoder, nvidiaCardBytes }()
	cardBytes = func() int64 { return 6 << 30 }

	chooseTrickplayDecoder = func() *gpuDecoder { return nil }
	if m := CurrentTrickplayMemory(); m.Decoder != "cpu" || m.FrameBytes4K>>20 != 228 || m.CardFrameBytes4K != 0 {
		t.Errorf("CPU: %+v", m)
	}
	chooseTrickplayDecoder = func() *gpuDecoder { return &gpuDecoder{hwaccel: "vaapi", device: "/dev/dri/renderD128"} }
	if m := CurrentTrickplayMemory(); m.Decoder != "vaapi" || m.FrameBytes4K>>20 != 370 || m.ProcessBytes4K>>20 != 40 || m.CardBytes != 0 {
		t.Errorf("VA-API: %+v", m)
	}
	chooseTrickplayDecoder = func() *gpuDecoder { return &gpuDecoder{hwaccel: "cuda"} }
	if m := CurrentTrickplayMemory(); m.Decoder != "cuda" || m.FrameBytes4K>>20 != 124 || m.CardFrameBytes4K>>20 != 540 || m.CardBytes != 6<<30 {
		t.Errorf("CUDA: %+v", m)
	}
}
