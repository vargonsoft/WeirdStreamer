package media

import (
	"log"
	"sync"
	"time"
)

// gpuDecoder decodes trickplay batches on the GPU Settings chose for hardware encoding: VA-API on
// Intel and AMD (Quick Sync's GPU too), CUDA on NVIDIA. Each frame of a batch otherwise costs the
// CPU a decode of every frame from the keyframe before it to its time; measured on 4K, the GPU took
// a ninth to a tenth of the CPU time. Frames are scaled on the GPU and only the 320-pixel picture
// comes back (copying every decoded 4K frame back cost more than decoding on the CPU). A video the
// GPU can't decode goes back to the CPU, as before.
type gpuDecoder struct {
	hwaccel string // ffmpeg's -hwaccel: vaapi or cuda
	device  string // the render node, for vaapi
}

// chooseTrickplayDecoder picks the GPU for a video's frames (tests choose their own).
var chooseTrickplayDecoder = trickplayDecoder

// trickplayDecoder is the GPU to decode trickplay frames on, nil for the CPU: none chosen in
// Settings, VideoToolbox (untried), or the GPU turned out not to decode for a while (gpuOff).
func trickplayDecoder() *gpuDecoder {
	id, device, ok := hardwareChoice()
	if !ok {
		return nil
	}
	var g *gpuDecoder
	switch id {
	case "vaapi", "qsv":
		if device != "" {
			g = &gpuDecoder{hwaccel: "vaapi", device: device}
		}
	case "nvenc":
		g = &gpuDecoder{hwaccel: "cuda"}
	}
	if g == nil || gpuDecode.off() {
		return nil
	}
	return g
}

func (g *gpuDecoder) key() string { return g.hwaccel + ":" + g.device }

// globalArgs come before the inputs: one device for every input of the batch (a device each costs
// a context per input, which made batches slower than the CPU).
func (g *gpuDecoder) globalArgs() []string {
	if g.hwaccel == "vaapi" {
		return []string{"-init_hw_device", "vaapi=tp:" + g.device}
	}
	return []string{"-init_hw_device", "cuda=tp"}
}

// inputArgs come before each input: decode on the device, keeping the frames there.
func (g *gpuDecoder) inputArgs() []string {
	return []string{"-hwaccel", g.hwaccel, "-hwaccel_device", "tp", "-hwaccel_output_format", g.hwaccel}
}

// filter scales a frame to 320 pixels wide and 8 bits on the GPU, then brings it back for the JPEG.
func (g *gpuDecoder) filter() string {
	scale := "scale_vaapi=w=320:h=-2:format=nv12"
	if g.hwaccel == "cuda" {
		scale = "scale_cuda=320:-2:format=nv12"
	}
	return scale + ",hwdownload,format=nv12,format=yuvj420p"
}

// gpuDecode is what's been learned about the GPU while making frames: the most pixels a batch may
// decode at once (a GPU runs out of memory: a 6 GB GTX 1060 took 10 4K decoders, not 15), and
// videos it couldn't decode at all in a row, which turn it off for a while (a driver gone wrong).
var gpuDecode = &gpuDecodeState{pixels: map[string]int64{}}

type gpuDecodeState struct {
	mu       sync.Mutex
	pixels   map[string]int64 // by gpuDecoder.key; 0 or missing: no limit found
	failed   int              // videos in a row the GPU couldn't decode
	offUntil time.Time
}

const (
	gpuFailuresToRest = 3
	gpuRest           = 10 * time.Minute
)

func (s *gpuDecodeState) off() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.offUntil)
}

// batchLimit is how many frames of a width×height video a batch may decode at once on the GPU (0:
// no limit found yet).
func (g *gpuDecoder) batchLimit(width, height int) int {
	gpuDecode.mu.Lock()
	limit := gpuDecode.pixels[g.key()]
	gpuDecode.mu.Unlock()
	if limit <= 0 {
		return 0
	}
	return max(1, int(limit/framePixels(width, height)))
}

// tooBig records that a batch of n frames of a width×height video failed on the GPU while one frame
// of it worked: its memory ran out, so batches keep to half of that from now on.
func (g *gpuDecoder) tooBig(n, width, height int) {
	limit := int64(max(n/2, 1)) * framePixels(width, height)
	gpuDecode.mu.Lock()
	if old := gpuDecode.pixels[g.key()]; old == 0 || limit < old {
		gpuDecode.pixels[g.key()] = limit
		gpuDecode.mu.Unlock()
		log.Printf("trickplay: the GPU (%s) ran out of memory with %d frames of %dx%d at once: up to %d at a time from now on",
			g.hwaccel, n, width, height, max(n/2, 1))
		return
	}
	gpuDecode.mu.Unlock()
}

// videoDone records whether the GPU decoded a video; gpuFailuresToRest videos in a row it couldn't
// send trickplay to the CPU for gpuRest.
func (g *gpuDecoder) videoDone(decoded bool) {
	gpuDecode.mu.Lock()
	defer gpuDecode.mu.Unlock()
	if decoded {
		gpuDecode.failed = 0
		return
	}
	if gpuDecode.failed++; gpuDecode.failed >= gpuFailuresToRest {
		gpuDecode.failed = 0
		gpuDecode.offUntil = time.Now().Add(gpuRest)
		log.Printf("trickplay: the GPU (%s) couldn't decode %d videos in a row: decoding on the CPU for %s",
			g.hwaccel, gpuFailuresToRest, gpuRest)
	}
}

// framePixels is a frame's size for the GPU's limit; 4K when unknown.
func framePixels(width, height int) int64 {
	if width <= 0 || height <= 0 {
		return 3840 * 2160
	}
	return int64(width) * int64(height)
}
