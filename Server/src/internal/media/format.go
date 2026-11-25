package media

import (
	"fmt"
	"strings"
)

// A video's format line for people, as the scanner stores it with each video (models.MediaItem's
// Format): its resolution, video codec, HDR and main audio track, such as
// "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1" or "1080p · H.264 · AAC 2.0".

// formatProbe is what ffprobe's -show_streams says that the line needs.
type formatProbe struct {
	Streams []struct {
		CodecType     string `json:"codec_type"`
		CodecName     string `json:"codec_name"`
		Profile       string `json:"profile"`
		Width         int    `json:"width"`
		Height        int    `json:"height"`
		Channels      int    `json:"channels"`
		ColorTransfer string `json:"color_transfer"`
		Disposition   struct {
			Default     int `json:"default"`
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
		SideData []struct {
			Type string `json:"side_data_type"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

// formatLine builds a video's format line from its probe; "" when it has no video stream.
func formatLine(p formatProbe) string {
	var parts []string
	video, audio := -1, -1
	for i, st := range p.Streams {
		switch {
		case st.CodecType == "video" && st.Disposition.AttachedPic == 0 && video < 0:
			video = i
		case st.CodecType == "audio" && (audio < 0 || (st.Disposition.Default == 1 && p.Streams[audio].Disposition.Default == 0)):
			audio = i
		}
	}
	if video < 0 {
		return ""
	}
	v := p.Streams[video]
	if r := resolutionLabel(v.Width, v.Height); r != "" {
		parts = append(parts, r)
	}
	parts = append(parts, videoCodecLabel(v.CodecName, v.Profile))
	if hdr := hdrLabel(v.ColorTransfer, v.SideData); hdr != "" {
		parts = append(parts, hdr)
	}
	if audio >= 0 {
		a := p.Streams[audio]
		parts = append(parts, audioLabel(a.CodecName, a.Profile, a.Channels))
	}
	return strings.Join(parts, " · ")
}

// resolutionLabel names a picture size as people do ("4K", "1080p"), by its width as well as its
// height, so a widescreen film cropped to 1920×800 is still 1080p.
func resolutionLabel(w, h int) string {
	switch {
	case w <= 0 || h <= 0:
		return ""
	case w >= 7000 || h >= 4000:
		return "8K"
	case w >= 3200 || h >= 2000:
		return "4K"
	case w >= 2200 || h >= 1300:
		return "1440p"
	case w >= 1700 || h >= 1000:
		return "1080p"
	case w >= 1100 || h >= 700:
		return "720p"
	case h > 500:
		return "576p"
	case h > 400 || w >= 640:
		return "480p"
	}
	return "SD"
}

var videoCodecNames = map[string]string{
	"h264": "H.264", "hevc": "HEVC", "av1": "AV1", "vp9": "VP9", "vp8": "VP8", "mpeg4": "MPEG-4",
	"mpeg2video": "MPEG-2", "mpeg1video": "MPEG-1", "vc1": "VC-1", "wmv3": "WMV 9", "wmv2": "WMV 8",
	"wmv1": "WMV 7", "msmpeg4v3": "DivX 3", "msmpeg4v2": "MS MPEG-4", "h263": "H.263", "prores": "ProRes",
	"theora": "Theora", "rv40": "RealVideo", "rv30": "RealVideo", "dvvideo": "DV", "mjpeg": "Motion JPEG",
}

func videoCodecLabel(codec, profile string) string {
	if name, ok := videoCodecNames[codec]; ok {
		return name
	}
	return strings.ToUpper(codec)
}

// hdrLabel names a video's HDR: Dolby Vision (its configuration record), else HDR10 (PQ) or HLG.
func hdrLabel(transfer string, sideData []struct {
	Type string `json:"side_data_type"`
}) string {
	for _, d := range sideData {
		if strings.Contains(strings.ToLower(d.Type), "dovi") || strings.Contains(strings.ToLower(d.Type), "dolby vision") {
			return "Dolby Vision"
		}
	}
	switch transfer {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	}
	return ""
}

var audioCodecNames = map[string]string{
	"truehd": "TrueHD", "eac3": "DD+", "ac3": "Dolby Digital", "dts": "DTS", "aac": "AAC", "flac": "FLAC",
	"opus": "Opus", "vorbis": "Vorbis", "mp3": "MP3", "mp2": "MP2", "alac": "ALAC", "wmav2": "WMA",
	"wmapro": "WMA Pro", "wmav1": "WMA",
}

// audioLabel names an audio track: its codec (DTS by its profile: "DTS-HD MA", "DTS:X"), Atmos
// when it carries it, and its channels ("5.1").
func audioLabel(codec, profile string, channels int) string {
	name, ok := audioCodecNames[codec]
	switch {
	case strings.HasPrefix(codec, "pcm_"):
		name = "PCM"
	case !ok:
		name = strings.ToUpper(codec)
	}
	if codec == "dts" {
		switch p := strings.ToLower(profile); {
		case strings.Contains(p, "dts:x") || strings.Contains(p, "dts-x"):
			name = "DTS:X"
		case strings.Contains(p, "ma"):
			name = "DTS-HD MA"
		case strings.Contains(p, "hra") || strings.Contains(p, "hi res"):
			name = "DTS-HD HRA"
		}
	}
	if strings.Contains(strings.ToLower(profile), "atmos") {
		name += " Atmos"
	}
	switch channels {
	case 0:
		return name
	case 1:
		return name + " 1.0"
	case 2:
		return name + " 2.0"
	case 3:
		return name + " 2.1"
	case 6:
		return name + " 5.1"
	case 7:
		return name + " 6.1"
	case 8:
		return name + " 7.1"
	}
	return fmt.Sprintf("%s %dch", name, channels)
}
