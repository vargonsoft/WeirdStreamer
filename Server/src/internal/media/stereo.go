package media

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// Side-by-side and other 3D H.264 videos often mark their layout twice: in the container
// (Matroska's StereoMode) and in the stream's own SEI. ffmpeg 8 then gives each decoded frame the
// marker twice and stops ("File exists" from the filter graph), so such a video got no thumbnail,
// trickplay frames or transcode, though players reading it themselves play it. Reading it with the
// stream's SEI messages left out (an input bitstream filter: the file isn't touched) leaves the
// container's marker alone, and it decodes.

var stereoCache = struct {
	sync.Mutex
	files map[string]stereoEntry
}{files: map[string]stereoEntry{}}

type stereoEntry struct {
	size, mtime int64
	twice       bool
}

// stereoMarked says whether path is an H.264 video whose container marks it as 3D (remembered
// until the file changes).
func stereoMarked(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	stereoCache.Lock()
	e, ok := stereoCache.files[path]
	stereoCache.Unlock()
	if ok && e.size == info.Size() && e.mtime == info.ModTime().UnixNano() {
		return e.twice
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := backgroundCommand(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name:stream_side_data=side_data_type", "-of", "json", path).Output()
	if err != nil {
		return false // unknown: not remembered, asked again
	}
	var probe struct {
		Streams []struct {
			Codec    string `json:"codec_name"`
			SideData []struct {
				Type string `json:"side_data_type"`
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	twice := false
	if json.Unmarshal(out, &probe) == nil && len(probe.Streams) > 0 && probe.Streams[0].Codec == "h264" {
		for _, d := range probe.Streams[0].SideData {
			if strings.Contains(strings.ToLower(strings.ReplaceAll(d.Type, " ", "")), "stereo3d") {
				twice = true
			}
		}
	}
	stereoCache.Lock()
	stereoCache.files[path] = stereoEntry{info.Size(), info.ModTime().UnixNano(), twice}
	stereoCache.Unlock()
	return twice
}

// stereoFilter is the bitstream filter that leaves an H.264 stream's SEI out, for reading a 3D
// video marked in its container (stereoMarked).
const stereoFilter = "filter_units=remove_types=6"

// inputFilters are the input options (before its -i) for decoding path's video: matrix, the
// bitstream filter some decoders need (h264_metadata=matrix_coefficients=1, or ""), and the SEI
// left out of a 3D video marked in its container. Nothing when neither applies.
func inputFilters(path, matrix string) []string {
	var chain []string
	if matrix != "" {
		chain = append(chain, matrix)
	}
	if stereoMarked(path) {
		chain = append(chain, stereoFilter)
	}
	if len(chain) == 0 {
		return nil
	}
	return []string{"-bsf:v", strings.Join(chain, ",")}
}
