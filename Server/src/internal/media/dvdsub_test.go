package media

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// encodeSPU makes a DVD subpicture of pic, shown at once and hidden after stop seconds: the
// encoder the decoder reads (two fields of nibble runs, then the control sequences).
func encodeSPU(pic *spuPicture, stop float64) []byte {
	var nibbles []byte
	put := func(v, n int) { // v in n nibbles
		for i := n - 1; i >= 0; i-- {
			nibbles = append(nibbles, byte(v>>(4*i))&0x0f)
		}
	}
	field := func(first int) []byte {
		nibbles = nibbles[:0]
		for y := first; y < pic.h; y += 2 {
			row := pic.pixels[y*pic.w : (y+1)*pic.w]
			for x := 0; x < len(row); {
				c := row[x]
				n := 1
				for x+n < len(row) && row[x+n] == c && n < 255 {
					n++
				}
				v := n<<2 | int(c)
				switch {
				case n < 4:
					put(v, 1)
				case n < 16:
					put(v, 2)
				case n < 64:
					put(v, 3)
				default:
					put(v, 4)
				}
				x += n
			}
			if len(nibbles)%2 == 1 {
				nibbles = append(nibbles, 0)
			}
		}
		out := make([]byte, len(nibbles)/2)
		for i := range out {
			out[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
		}
		return out
	}
	top, bottom := field(0), field(1)
	ctrl := 4 + len(top) + len(bottom)
	second := ctrl + 4 + 3 + 3 + 7 + 5 + 1 + 1
	var b bytes.Buffer
	b.Write([]byte{0, 0, byte(ctrl >> 8), byte(ctrl)})
	b.Write(top)
	b.Write(bottom)
	b.Write([]byte{0, 0, byte(second >> 8), byte(second)})
	c, a := pic.colors, pic.alpha
	b.Write([]byte{0x03, c[3]<<4 | c[2], c[1]<<4 | c[0]})
	b.Write([]byte{0x04, a[3]<<4 | a[2], a[1]<<4 | a[0]})
	x2, y2 := pic.x+pic.w-1, pic.y+pic.h-1
	b.Write([]byte{0x05, byte(pic.x >> 4), byte(pic.x<<4) | byte(x2>>8), byte(x2), byte(pic.y >> 4), byte(pic.y<<4) | byte(y2>>8), byte(y2)})
	b.Write([]byte{0x06, 0, 4, byte((4 + len(top)) >> 8), byte(4 + len(top))})
	start := byte(0x01) // start display; 0x00 starts it as forced
	if pic.forced {
		start = 0x00
	}
	b.Write([]byte{start, 0xff})
	date := int(math.Round(stop * 90000 / 1024))
	b.Write([]byte{byte(date >> 8), byte(date), byte(second >> 8), byte(second), 0x02, 0xff})
	out := b.Bytes()
	binary.BigEndian.PutUint16(out, uint16(len(out)))
	return out
}

// testPicture is a 40×6 subpicture: a background with a pattern band and an emphasis block.
func testPicture() *spuPicture {
	pic := &spuPicture{x: 100, y: 400, w: 40, h: 6, colors: [4]uint8{0, 15, 1, 2}, alpha: [4]uint8{0, 15, 15, 8}}
	pic.pixels = make([]byte, pic.w*pic.h)
	for y := 0; y < pic.h; y++ {
		for x := 0; x < pic.w; x++ {
			switch {
			case y >= 1 && y <= 4 && x >= 5 && x < 30:
				pic.pixels[y*pic.w+x] = 1
			case y == 2 && x >= 32:
				pic.pixels[y*pic.w+x] = 3
			case x == 0:
				pic.pixels[y*pic.w+x] = 2
			}
		}
	}
	return pic
}

func TestDecodeSPU(t *testing.T) {
	want := testPicture()
	got, err := decodeSPU(encodeSPU(want, 2.5))
	if err != nil {
		t.Fatal(err)
	}
	if got.x != want.x || got.y != want.y || got.w != want.w || got.h != want.h {
		t.Fatalf("area %d,%d %dx%d, want %d,%d %dx%d", got.x, got.y, got.w, got.h, want.x, want.y, want.w, want.h)
	}
	if got.colors != want.colors || got.alpha != want.alpha {
		t.Errorf("colors %v alpha %v, want %v %v", got.colors, got.alpha, want.colors, want.alpha)
	}
	if !bytes.Equal(got.pixels, want.pixels) {
		t.Errorf("pixels differ:\n got %v\nwant %v", got.pixels, want.pixels)
	}
	if got.startDelay != 0 || math.Abs(got.stopDelay-2.5) > 0.02 {
		t.Errorf("shown %v to %v, want 0 to 2.5", got.startDelay, got.stopDelay)
	}
}

// pgsSet is a display set as the players read it.
type pgsSet struct {
	time          float64
	width, height int
	x, y          int
	objW, objH    int
	pixels        []byte // palette indexes
	palette       map[byte][4]byte
	objects       int
}

func readPGS(t *testing.T, sup []byte) []pgsSet {
	var sets []pgsSet
	var cur pgsSet
	var data []byte
	for i := 0; i+13 <= len(sup); {
		if sup[i] != 'P' || sup[i+1] != 'G' {
			t.Fatalf("no PG at %d", i)
		}
		pts := binary.BigEndian.Uint32(sup[i+2:])
		kind, size := sup[i+10], int(binary.BigEndian.Uint16(sup[i+11:]))
		p := sup[i+13 : i+13+size]
		i += 13 + size
		switch kind {
		case 0x16:
			cur = pgsSet{time: float64(pts) / 90000, width: int(binary.BigEndian.Uint16(p)), height: int(binary.BigEndian.Uint16(p[2:])),
				objects: int(p[10]), palette: map[byte][4]byte{}}
			if cur.objects > 0 {
				cur.x, cur.y = int(binary.BigEndian.Uint16(p[15:])), int(binary.BigEndian.Uint16(p[17:]))
			}
		case 0x14:
			for q := 2; q+5 <= len(p); q += 5 {
				cur.palette[p[q]] = [4]byte{p[q+1], p[q+2], p[q+3], p[q+4]}
			}
		case 0x15:
			if p[3]&0x80 != 0 {
				cur.objW, cur.objH = int(binary.BigEndian.Uint16(p[7:])), int(binary.BigEndian.Uint16(p[9:]))
				data = append([]byte(nil), p[11:]...)
			} else {
				data = append(data, p[4:]...)
			}
		case 0x80:
			if cur.objects > 0 {
				cur.pixels = pgsDecodeRLE(data, cur.objW, cur.objH)
			}
			sets = append(sets, cur)
		}
	}
	return sets
}

func pgsDecodeRLE(d []byte, w, h int) []byte {
	out := make([]byte, w*h)
	x, y := 0, 0
	for i := 0; i < len(d) && y < h; {
		b := d[i]
		i++
		if b != 0 {
			out[y*w+x] = b
			x++
			continue
		}
		f := d[i]
		i++
		if f == 0 {
			x, y = 0, y+1
			continue
		}
		n := int(f & 0x3f)
		if f&0x40 != 0 {
			n = n<<8 | int(d[i])
			i++
		}
		c := byte(0)
		if f&0x80 != 0 {
			c = d[i]
			i++
		}
		for k := 0; k < n && x < w; k++ {
			out[y*w+x] = c
			x++
		}
	}
	return out
}

func TestWritePGS(t *testing.T) {
	pic := testPicture()
	look := &dvdLook{palette: defaultDVDPalette, width: 720, height: 480, aspect: 1}
	look.palette[15] = [3]uint8{255, 255, 255}
	look.palette[1] = [3]uint8{0, 0, 0}
	look.palette[2] = [3]uint8{255, 0, 0}
	var out bytes.Buffer
	writePGS(&out, pic, look, 10, 12.5, 0)
	sets := readPGS(t, out.Bytes())
	if len(sets) != 2 || sets[0].objects != 1 || sets[1].objects != 0 {
		t.Fatalf("%d sets: %+v", len(sets), sets)
	}
	shown, cleared := sets[0], sets[1]
	if shown.time != 10 || cleared.time != 12.5 || shown.width != 720 || shown.height != 480 || shown.x != 100 || shown.y != 400 {
		t.Errorf("shown %+v", shown)
	}
	// Square pixels already: the same picture, values 1-3 as palette entries 2-4, 0 clear.
	for i, v := range pic.pixels {
		want := byte(0)
		if v != 0 {
			want = v + 1
		}
		if shown.pixels[i] != want {
			t.Fatalf("pixel %d: %d, want %d", i, shown.pixels[i], want)
		}
	}
	if white := shown.palette[2]; white[0] < 230 || white[3] != 255 {
		t.Errorf("pattern color %v, want white and opaque", white)
	}
	if red := shown.palette[4]; red[1] < 200 || red[3] != 136 {
		t.Errorf("emphasis color %v, want red at half opacity", red)
	}

	// 16:9 NTSC: 32:27 pixels widen the canvas and the picture.
	look.aspect = 32.0 / 27
	out.Reset()
	writePGS(&out, pic, look, 0, 1, 0)
	wide := readPGS(t, out.Bytes())[0]
	if wide.width != 853 || wide.objW != 47 || wide.x != 119 || wide.objH != 6 {
		t.Errorf("widened: canvas %d, picture %d at %d (height %d)", wide.width, wide.objW, wide.x, wide.objH)
	}
}

func TestParseIdx(t *testing.T) {
	look := &dvdLook{palette: defaultDVDPalette, width: 720, height: 480}
	parseIdx("# VobSub index file, v7\nsize: 720x576\npalette: 000000, ffffff, 808080, 0000ff, 112233\n", look)
	if look.width != 720 || look.height != 576 || look.palette[1] != [3]uint8{255, 255, 255} || look.palette[3] != [3]uint8{0, 0, 255} || look.palette[4] != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("%+v", look)
	}
	if got := hexDump("\n00000000: 7369 7a65 3a20 3732 3078 3438 300a 7061  size: 720x480.pa\n00000010: 6c  l\n"); string(got) != "size: 720x480\npal" {
		t.Errorf("hexDump %q", got)
	}
}
