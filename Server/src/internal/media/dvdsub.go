package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// DVD subtitles (dvd_subtitle, VobSub) in a video file go to the players as PGS, which they already
// decode and draw over the video (format=sup), instead of being burned into a transcode. The server
// reads the track's packets with ffprobe, decodes each subpicture (its run-length lines, four colors
// from the track's palette and their transparency) and writes it as a PGS display set, with one
// that clears it at its end. DVD pixels are rarely square (720×480 shown 16:9), so the pictures are
// widened to square pixels, as the video is shown. Disc images (.iso) still burn theirs in.

// IsDVDSubtitle reports whether a subtitle codec is DVD's pictures.
func IsDVDSubtitle(codec string) bool { return codec == "dvd_subtitle" }

// ServeDVDSubtitles writes a DVD subtitle track (track among the file's subtitles) from start to
// start+duration (0: to the end) as a PGS .sup stream.
func ServeDVDSubtitles(w http.ResponseWriter, r *http.Request, filePath string, track int, start, duration float64) error {
	sup, err := DVDSubtitlesAsPGS(r.Context(), filePath, track, start, duration)
	if err != nil {
		http.Error(w, "Could not read the subtitles", http.StatusUnprocessableEntity)
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = w.Write(sup)
	return err
}

// DVDSubtitlesAsPGS reads a DVD subtitle track's subpictures that start in the window and writes
// them as PGS display sets.
func DVDSubtitlesAsPGS(ctx context.Context, filePath string, track int, start, duration float64) ([]byte, error) {
	look, err := probeDVDSubtitle(ctx, filePath, track)
	if err != nil {
		return nil, err
	}
	packets, err := dvdSubtitlePackets(ctx, filePath, track, start, duration)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	n := 0
	for i, p := range packets {
		pic, err := decodeSPU(p.data)
		if err != nil || pic.w <= 0 || pic.h <= 0 {
			continue // a damaged subpicture: the others still show
		}
		begin := p.time + pic.startDelay
		end := p.time + pic.stopDelay
		if pic.stopDelay <= pic.startDelay {
			// No stop command: until the packet's end, the next subpicture, or 5 seconds.
			switch {
			case p.duration > 0:
				end = p.time + p.duration
			case i+1 < len(packets):
				end = packets[i+1].time
			default:
				end = begin + 5
			}
		}
		if begin < start || duration > 0 && begin >= start+duration || end <= begin {
			continue
		}
		writePGS(&out, pic, look, begin, end, n)
		n += 2
	}
	return out.Bytes(), nil
}

// dvdLook is what a track's pictures are drawn with: its palette (16 RGB colors), the canvas the
// pictures are placed on, and the width each pixel shows at (the video's sample aspect).
type dvdLook struct {
	palette [16][3]uint8
	width   int
	height  int
	aspect  float64
}

// probeDVDSubtitle reads the track's palette and canvas (the .idx text Matroska keeps as its
// codec private data: "size: 720x480", "palette: 000000, …") and the video's sample aspect.
func probeDVDSubtitle(ctx context.Context, filePath string, track int) (*dvdLook, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_streams", "-show_data", "-of", "json", filePath).Output()
	if err != nil {
		return nil, err
	}
	var probe struct {
		Streams []struct {
			CodecType   string `json:"codec_type"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			SAR         string `json:"sample_aspect_ratio"`
			Extradata   string `json:"extradata"`
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}
	look := &dvdLook{palette: defaultDVDPalette, width: 720, height: 480, aspect: 1}
	videoSeen, subtitle := false, 0
	for _, s := range probe.Streams {
		switch {
		case s.CodecType == "video" && s.Disposition.AttachedPic == 0 && !videoSeen:
			videoSeen = true
			if s.Width > 0 && s.Height > 0 {
				look.width, look.height = s.Width, s.Height
			}
			look.aspect = parseRatio(s.SAR)
		case s.CodecType == "subtitle":
			if subtitle == track {
				parseIdx(string(hexDump(s.Extradata)), look)
				return look, nil
			}
			subtitle++
		}
	}
	return nil, errors.New("no such subtitle track")
}

// parseRatio reads "32:27" (1 when missing or 0:1).
func parseRatio(s string) float64 {
	a, b, ok := strings.Cut(s, ":")
	n, err1 := strconv.ParseFloat(a, 64)
	d, err2 := strconv.ParseFloat(b, 64)
	if !ok || err1 != nil || err2 != nil || n <= 0 || d <= 0 {
		return 1
	}
	return n / d
}

// parseIdx takes the size and palette from a VobSub .idx header.
func parseIdx(idx string, look *dvdLook) {
	for _, line := range strings.Split(idx, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "size":
			w, h, _ := strings.Cut(value, "x")
			if wi, err := strconv.Atoi(strings.TrimSpace(w)); err == nil && wi > 0 {
				if hi, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && hi > 0 {
					look.width, look.height = wi, hi
				}
			}
		case "palette":
			for i, c := range strings.Split(value, ",") {
				if i >= 16 {
					break
				}
				if b, err := hex.DecodeString(strings.TrimSpace(c)); err == nil && len(b) == 3 {
					look.palette[i] = [3]uint8{b[0], b[1], b[2]}
				}
			}
		}
	}
}

// defaultDVDPalette stands in for a track without one (a palette lives in the disc's IFO files):
// grays, as ffmpeg guesses.
var defaultDVDPalette = func() (p [16][3]uint8) {
	for i := range p {
		v := uint8(i * 17)
		p[i] = [3]uint8{v, v, v}
	}
	return
}()

type dvdPacket struct {
	time     float64 // seconds in the file
	duration float64 // the container's, 0 when it has none
	data     []byte
}

// dvdSubtitlePackets reads the track's packets from about start (where the seek lands) to
// start+duration, or to the end.
func dvdSubtitlePackets(ctx context.Context, filePath string, track int, start, duration float64) ([]dvdPacket, error) {
	args := []string{"-v", "error", "-select_streams", "s:" + strconv.Itoa(track), "-show_packets", "-show_data", "-of", "json"}
	if start > 0 || duration > 0 {
		interval := strconv.FormatFloat(max(start-1, 0), 'f', 3, 64) + "%"
		if duration > 0 {
			interval += "+" + strconv.FormatFloat(duration+1, 'f', 3, 64)
		}
		args = append(args, "-read_intervals", interval)
	}
	out, err := exec.CommandContext(ctx, "ffprobe", append(args, filePath)...).Output()
	if err != nil {
		return nil, err
	}
	var probe struct {
		Packets []struct {
			PTS      string `json:"pts_time"`
			Duration string `json:"duration_time"`
			Data     string `json:"data"`
		} `json:"packets"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}
	packets := make([]dvdPacket, 0, len(probe.Packets))
	for _, p := range probe.Packets {
		t, err := strconv.ParseFloat(p.PTS, 64)
		if err != nil {
			continue
		}
		d, _ := strconv.ParseFloat(p.Duration, 64)
		packets = append(packets, dvdPacket{time: t, duration: d, data: hexDump(p.Data)})
	}
	return packets, nil
}

// hexDump reads ffprobe's -show_data dump ("00000000: 0b77 02ff …  .w..") back into bytes.
func hexDump(dump string) []byte {
	var out []byte
	for _, line := range strings.Split(dump, "\n") {
		_, rest, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		groups, _, _ := strings.Cut(rest, "  ") // the hex, then two spaces and the text
		if b, err := hex.DecodeString(strings.ReplaceAll(groups, " ", "")); err == nil {
			out = append(out, b...)
		}
	}
	return out
}

// spuPicture is a decoded DVD subpicture: where it goes, its pixels (0-3 each), the palette
// entries those four values take and their opacity (0-15), and when it shows and hides, in
// seconds after its packet's time.
type spuPicture struct {
	x, y, w, h            int
	pixels                []byte
	colors, alpha         [4]uint8
	startDelay, stopDelay float64
	forced                bool
}

// decodeSPU decodes a DVD subpicture unit: its size, the offset of its control sequences, and the
// run-length lines of its two fields (even lines, then odd ones).
func decodeSPU(p []byte) (*spuPicture, error) {
	if len(p) < 4 {
		return nil, errors.New("too short")
	}
	size := int(binary.BigEndian.Uint16(p))
	if size > len(p) || size < 4 {
		size = len(p)
	}
	p = p[:size]
	ctrl := int(binary.BigEndian.Uint16(p[2:]))
	pic := &spuPicture{stopDelay: -1}
	top, bottom := -1, -1
	for seen := 0; ctrl+4 <= len(p) && seen < 64; seen++ {
		delay := float64(binary.BigEndian.Uint16(p[ctrl:])) * 1024 / 90000
		next := int(binary.BigEndian.Uint16(p[ctrl+2:]))
		i := ctrl + 4
	commands:
		for i < len(p) {
			cmd := p[i]
			i++
			switch cmd {
			case 0x00: // forced start
				pic.forced = true
				pic.startDelay = delay
			case 0x01: // start
				pic.startDelay = delay
			case 0x02: // stop
				pic.stopDelay = delay
			case 0x03: // colors: emphasis 2, emphasis 1, pattern, background
				if i+2 > len(p) {
					break commands
				}
				pic.colors = [4]uint8{p[i+1] & 0x0f, p[i+1] >> 4, p[i] & 0x0f, p[i] >> 4}
				i += 2
			case 0x04: // opacity, in the same order
				if i+2 > len(p) {
					break commands
				}
				pic.alpha = [4]uint8{p[i+1] & 0x0f, p[i+1] >> 4, p[i] & 0x0f, p[i] >> 4}
				i += 2
			case 0x05: // the display area
				if i+6 > len(p) {
					break commands
				}
				x1 := int(p[i])<<4 | int(p[i+1])>>4
				x2 := int(p[i+1]&0x0f)<<8 | int(p[i+2])
				y1 := int(p[i+3])<<4 | int(p[i+4])>>4
				y2 := int(p[i+4]&0x0f)<<8 | int(p[i+5])
				pic.x, pic.y, pic.w, pic.h = x1, y1, x2-x1+1, y2-y1+1
				i += 6
			case 0x06: // where the two fields' lines start
				if i+4 > len(p) {
					break commands
				}
				top, bottom = int(binary.BigEndian.Uint16(p[i:])), int(binary.BigEndian.Uint16(p[i+2:]))
				i += 4
			case 0x07: // color changes within the picture (rare): skipped by their size
				if i+2 > len(p) {
					break commands
				}
				i += int(binary.BigEndian.Uint16(p[i:]))
			case 0xff:
				break commands
			default:
				break commands
			}
		}
		if next == ctrl || next < 4 {
			break
		}
		ctrl = next
	}
	if pic.w <= 0 || pic.h <= 0 || pic.w > 4096 || pic.h > 4096 || top < 4 || bottom < 4 || top >= len(p) || bottom >= len(p) {
		return nil, errors.New("no picture")
	}
	pic.pixels = make([]byte, pic.w*pic.h)
	decodeRLE(p, top, pic, 0)
	decodeRLE(p, bottom, pic, 1)
	return pic, nil
}

// decodeRLE decodes one field's lines (from line first, every other one): runs of 4 to 16 bits,
// the count then the 2-bit value; a count of 0 fills the line; each line ends on a byte.
func decodeRLE(p []byte, offset int, pic *spuPicture, first int) {
	pos := offset * 2 // in nibbles
	nibble := func() int {
		if pos/2 >= len(p) {
			pos++
			return 0
		}
		b := p[pos/2]
		pos++
		if pos%2 == 1 {
			return int(b >> 4)
		}
		return int(b & 0x0f)
	}
	for y := first; y < pic.h; y += 2 {
		for x := 0; x < pic.w; {
			if pos/2 >= len(p) {
				return
			}
			v := nibble()
			if v < 0x4 {
				v = v<<4 | nibble()
				if v < 0x10 {
					v = v<<4 | nibble()
					if v < 0x40 {
						v = v<<4 | nibble()
						if v < 0x4 {
							v |= (pic.w - x) << 2
						}
					}
				}
			}
			n := min(v>>2, pic.w-x)
			if n == 0 {
				n = pic.w - x
			}
			color := byte(v & 3)
			row := pic.pixels[y*pic.w:]
			for i := 0; i < n; i++ {
				row[x+i] = color
			}
			x += n
		}
		pos += pos % 2 // to the next byte
	}
}

// writePGS writes pic as a PGS display set showing it at begin, and one clearing it at end. The
// pictures and canvas are widened to square pixels by the video's sample aspect. Palette entry 0
// is clear in the players: the four values take 1-4, or 0 when fully transparent.
func writePGS(out *bytes.Buffer, pic *spuPicture, look *dvdLook, begin, end float64, number int) {
	aspect := look.aspect
	if aspect <= 0 || math.IsNaN(aspect) {
		aspect = 1
	}
	scale := func(v int) int { return int(math.Round(float64(v) * aspect)) }
	canvasW, canvasH := scale(look.width), look.height
	x, w := scale(pic.x), max(scale(pic.w), 1)
	y, h := pic.y, pic.h
	if x+w > canvasW {
		canvasW = x + w
	}
	if y+h > canvasH {
		canvasH = y + h
	}
	var index [4]byte
	for v := range 4 {
		if pic.alpha[v] > 0 {
			index[v] = byte(v + 1)
		}
	}
	// The picture, widened (nearest pixel), run-length coded as PGS lines.
	var rle bytes.Buffer
	line := make([]byte, w)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			src := min(int((float64(col)+0.5)/aspect), pic.w-1)
			line[col] = index[pic.pixels[row*pic.w+src]]
		}
		pgsLine(&rle, line)
	}
	pts := uint32(math.Round(begin * 90000))

	// Composition: the canvas, and the object placed at x, y.
	pcs := []byte{0, 0, 0, 0, 0x10, 0, 0, 0x80, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(pcs[0:], uint16(canvasW))
	binary.BigEndian.PutUint16(pcs[2:], uint16(canvasH))
	binary.BigEndian.PutUint16(pcs[5:], uint16(number))
	binary.BigEndian.PutUint16(pcs[15:], uint16(x))
	binary.BigEndian.PutUint16(pcs[17:], uint16(y))
	pgsSegment(out, pts, 0x16, pcs)
	pgsSegment(out, pts, 0x17, pgsWindow(x, y, w, h))
	// Palette: the four colors, in BT.709 limited range as the players read it.
	pds := []byte{0, 0}
	for v := range 4 {
		if index[v] == 0 {
			continue
		}
		yy, cr, cb := ycrcb709(look.palette[pic.colors[v]])
		pds = append(pds, index[v], yy, cr, cb, pic.alpha[v]*17)
	}
	pgsSegment(out, pts, 0x14, pds)
	// The object, in fragments a segment can hold.
	data := rle.Bytes()
	head := make([]byte, 7)
	head[0], head[1], head[2] = byte((len(data)+4)>>16), byte((len(data)+4)>>8), byte(len(data)+4)
	binary.BigEndian.PutUint16(head[3:], uint16(w))
	binary.BigEndian.PutUint16(head[5:], uint16(h))
	const room = 65535 - 4 - 7
	for i, first := 0, true; first || i < len(data); first = false {
		chunk := data[i:min(i+room, len(data))]
		flags := byte(0)
		if first {
			flags |= 0x80
		}
		if i+len(chunk) >= len(data) {
			flags |= 0x40
		}
		ods := []byte{0, 0, 0, flags}
		if first {
			ods = append(ods, head...)
		}
		pgsSegment(out, pts, 0x15, append(ods, chunk...))
		i += len(chunk)
	}
	pgsSegment(out, pts, 0x80, nil)

	// At its end, a composition with no objects clears it.
	pts = uint32(math.Round(end * 90000))
	clear := []byte{0, 0, 0, 0, 0x10, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(clear[0:], uint16(canvasW))
	binary.BigEndian.PutUint16(clear[2:], uint16(canvasH))
	binary.BigEndian.PutUint16(clear[5:], uint16(number+1))
	pgsSegment(out, pts, 0x16, clear)
	pgsSegment(out, pts, 0x17, pgsWindow(x, y, w, h))
	pgsSegment(out, pts, 0x80, nil)
}

func pgsWindow(x, y, w, h int) []byte {
	wds := make([]byte, 10)
	wds[0] = 1
	binary.BigEndian.PutUint16(wds[2:], uint16(x))
	binary.BigEndian.PutUint16(wds[4:], uint16(y))
	binary.BigEndian.PutUint16(wds[6:], uint16(w))
	binary.BigEndian.PutUint16(wds[8:], uint16(h))
	return wds
}

func pgsSegment(out *bytes.Buffer, pts uint32, kind byte, payload []byte) {
	head := make([]byte, 13)
	head[0], head[1] = 'P', 'G'
	binary.BigEndian.PutUint32(head[2:], pts)
	head[10] = kind
	binary.BigEndian.PutUint16(head[11:], uint16(len(payload)))
	out.Write(head)
	out.Write(payload)
}

// pgsLine run-length codes a line: a lone nonzero pixel as itself, runs as 0, a flag byte (bit 7:
// a color follows; bit 6: a 14-bit count), the count and the color; 0, 0 ends the line.
func pgsLine(out *bytes.Buffer, line []byte) {
	for i := 0; i < len(line); {
		c := line[i]
		n := 1
		for i+n < len(line) && line[i+n] == c && n < 16383 {
			n++
		}
		switch {
		case c != 0 && n <= 2:
			for range n {
				out.WriteByte(c)
			}
		case c == 0 && n < 64:
			out.Write([]byte{0, byte(n)})
		case c == 0:
			out.Write([]byte{0, 0x40 | byte(n>>8), byte(n)})
		case n < 64:
			out.Write([]byte{0, 0x80 | byte(n), c})
		default:
			out.Write([]byte{0, 0xc0 | byte(n>>8), byte(n), c})
		}
		i += n
	}
	out.Write([]byte{0, 0})
}

// ycrcb709 is an RGB color as BT.709 limited-range Y, Cr, Cb.
func ycrcb709(rgb [3]uint8) (y, cr, cb uint8) {
	r, g, b := float64(rgb[0]), float64(rgb[1]), float64(rgb[2])
	luma := 0.2126*r + 0.7152*g + 0.0722*b
	clamp := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(v)))) }
	return clamp(16 + luma*219/255), clamp(128 + (r-luma)/1.5748*224/255), clamp(128 + (b-luma)/1.8556*224/255)
}
