package guide

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/image/font/sfnt"
)

// The logo designer (run by cmd/logoart, not by the server): draws each logo's artwork with the
// display fonts in a folder (Google Fonts' families under the SIL Open Font License, which stay on
// the machine that runs it; only the outlines drawn with them are kept, as artwork). Logos are one
// color, as stations' were: a symbol with the name beside or below it, and the number in a holder.
// Every logo has a font of its own, so no two channels look alike.

// design is one logo: its name (a genre's or a call sign), font, symbol (logosymbols.go; "" for
// none: the number's holder is the mark), layout and number holder.
type design struct {
	name, font, symbol string
	layout             string // "beside": the mark left, the name right; "below": the mark over the name
	holder             string // numberHolder's kinds
	keepCase           bool   // the name as written (script fonts), not in capitals
}

// The genre channels' logos (channels.go's bucket labels).
var genreDesigns = []design{
	{"Kids", "TitanOne-Regular", "star", "beside", "disc", false},
	{"Cartoons", "Bangers-Regular", "bubble", "beside", "square", false},
	{"Comedy", "Shrikhand-Regular", "smile", "below", "ring", true},
	{"Family Movies", "LilitaOne-Regular", "house", "beside", "block", false},
	{"Comedy Movies", "Lobster-Regular", "heart", "beside", "ring", true},
	{"Variety", "Monoton-Regular", "sunburst", "below", "rect", false},
	{"Sci-Fi", "Orbitron[wght]", "planet", "beside", "triangle", false},
	{"Sci-Fi Movies", "Audiowide-Regular", "rocket", "beside", "bar", false},
	{"Fright Night", "Creepster-Regular", "bat", "below", "triangle", false},
	{"Drama", "AbrilFatface-Regular", "curtains", "beside", "square", false},
	{"Drama Movies", "Cinzel[wght]", "reel", "beside", "ring", false},
	{"Real Life", "ArchivoBlack-Regular", "globe", "beside", "block", false},
	{"Action Movies", "RacingSansOne-Regular", "bolt", "beside", "bar", false},
	{"Movies", "AlfaSlabOne-Regular", "clapper", "below", "disc", false},
}

// The pool of made-up stations network channels are named after.
var stationDesigns = []design{
	{"WKBT", "Anton-Regular", "", "beside", "disc", false},
	{"KZXM", "Staatliches-Regular", "chevrons", "beside", "square", false},
	{"WLVR", "Teko[wght]", "waves", "below", "bar", false},
	{"KQTV", "BowlbyOneSC-Regular", "", "beside", "triangle", false},
	{"WPLN", "Pacifico-Regular", "sunset", "beside", "ring", true},
	{"KBRX", "Bungee-Regular", "diamond", "below", "block", false},
	{"WMTX", "Michroma-Regular", "", "beside", "ring", false},
	{"KJRV", "RussoOne-Regular", "arrow", "beside", "rect", false},
	{"WDXB", "BlackOpsOne-Regular", "starring", "below", "square", false},
	{"KPXL", "PressStart2P-Regular", "", "beside", "block", false},
	{"WZGT", "BungeeShade-Regular", "bars", "below", "disc", false},
	{"KVRN", "FasterOne-Regular", "", "beside", "bar", false},
	{"WQRP", "Wallpoet-Regular", "atom", "beside", "triangle", false},
	{"KTLX", "RubikMonoOne-Regular", "", "beside", "square", false},
	{"WGHB", "PassionOne-Black", "mountains", "below", "ring", false},
	{"KCVD", "Graduate-Regular", "", "below", "rect", false},
	{"WRNG", "Rye-Regular", "crown", "beside", "disc", false},
	{"KDMF", "SquadaOne-Regular", "tower", "beside", "block", false},
	{"WVXA", "ChangaOne-Italic", "", "beside", "rect", false},
	{"KFBZ", "Fascinate-Regular", "eye-less burst", "below", "triangle", false},
	{"WBNQ", "BebasNeue-Regular", "hexagon", "beside", "square", false},
	{"KWPD", "BubblegumSans-Regular", "crescent", "beside", "disc", false},
	{"WTJM", "Fredoka[wdth,wght]", "", "below", "ring", false},
	{"KRGL", "Righteous-Regular", "star", "below", "bar", false},
	{"WNSF", "Nosifer-Regular", "", "beside", "triangle", false},
}

// Design draws every logo's artwork with the fonts in fontDir (their .ttf files, by the names above).
func Design(fontDir string) ([]Artwork, error) {
	fonts := map[string]*sfnt.Font{}
	load := func(name string) (*sfnt.Font, error) {
		if f := fonts[name]; f != nil {
			return f, nil
		}
		b, err := os.ReadFile(filepath.Join(fontDir, name+".ttf"))
		if err != nil {
			return nil, err
		}
		f, err := sfnt.Parse(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		fonts[name] = f
		return f, nil
	}
	var out []Artwork
	for i, ds := range [][]design{genreDesigns, stationDesigns} {
		for _, d := range ds {
			f, err := load(d.font)
			if err != nil {
				return nil, err
			}
			id := "genre-" + slug(d.name)
			if i == 1 {
				id = "station-" + strings.ToLower(d.name)
			}
			p, bug := d.draw(f)
			out = append(out, Artwork{ID: id, Name: d.name, Font: d.font, Bug: bug, Shapes: []artShape{{D: pathData(p)}}})
		}
	}
	return out, nil
}

func slug(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, s)
}

// holderWidth is how wide a holder of height h is.
func holderWidth(kind string, h float64) float64 {
	switch kind {
	case "rect", "bar":
		return h * 1.45
	case "triangle":
		return h * 1.15
	}
	return h
}

const logoMargin = 24.0

// draw lays the logo out on the canvas: one path in the logo's color, and where the number goes.
func (d design) draw(f *sfnt.Font) (path, Bug) {
	name := d.name
	if !d.keepCase {
		name = strings.ToUpper(name)
	}
	st := textStyle{font: f, sx: 1}
	if d.layout == "below" {
		return d.drawBelow(name, st)
	}
	return d.drawBeside(name, st)
}

// drawBelow: the mark over the name; with a symbol, the number's holder follows the name.
func (d design) drawBelow(name string, st textStyle) (path, Bug) {
	var p path
	f := st.font
	baseline := LogoHeight - logoMargin - 10.0
	if d.symbol == "" {
		// the holder is the mark, the name under it alone
		s := st.fit(name, LogoWidth-2*logoMargin, 170/capHeight(f))
		p.add(s.text(name, (LogoWidth-s.width(name))/2, baseline))
		hh := baseline - s.cap() - 40 - logoMargin
		hw := holderWidth(d.holder, hh)
		return p, Bug{Kind: d.holder, X: (LogoWidth - hw) / 2, Y: logoMargin, W: hw, H: hh}
	}
	holderH := 140.0
	holderW := holderWidth(d.holder, holderH)
	s := st.fit(name, LogoWidth-2*logoMargin-holderW-30, 170/capHeight(f))
	holderH = math.Min(holderH, s.cap()*1.3)
	if d.holder == "triangle" {
		holderH *= 1.4
	}
	holderW = holderWidth(d.holder, holderH)
	w := s.width(name)
	x := (LogoWidth - (w + 30 + holderW)) / 2
	p.add(s.text(name, x, baseline))
	markSize := baseline - s.cap() - 40 - logoMargin
	markW := markSize * 1.8 // room for a wide symbol (a bat, a planet's ring)
	p.add(symbolIn(d.symbol, (LogoWidth-markW)/2, logoMargin, markW, markSize))
	mid := baseline - s.cap()/2
	return p, Bug{Kind: d.holder, X: x + w + 30, Y: mid - holderH/2, W: holderW, H: holderH}
}

// drawBeside: the mark left (the symbol, or the number's holder), the name right; with a symbol,
// the holder follows the name's last line.
func (d design) drawBeside(name string, st textStyle) (path, Bug) {
	var p path
	f := st.font
	markH := LogoHeight - 2*logoMargin - 100
	markY := (LogoHeight - markH) / 2
	textX := logoMargin
	if d.symbol == "" {
		hw := holderWidth(d.holder, markH)
		textX += hw + 40
		s := st.fit(name, LogoWidth-logoMargin-textX, markH*0.8/capHeight(f))
		p.add(s.text(name, textX, LogoHeight/2+s.cap()/2))
		return p, Bug{Kind: d.holder, X: logoMargin, Y: markY, W: hw, H: markH}
	}
	p.add(symbol(d.symbol, logoMargin, markY, markH))
	textX += markH + 40
	holderH := 120.0
	room := LogoWidth - logoMargin - textX - holderWidth(d.holder, holderH) - 26
	ls := lines(name)
	gap := 30.0
	// one size for the lines (the smallest that fits), so they read as one name
	s := st
	s.size = math.Inf(1)
	for _, line := range ls {
		max := (LogoHeight - 2*logoMargin - gap*float64(len(ls)-1)) / float64(len(ls)) * 0.8 / capHeight(f)
		if fit := st.fit(line, room, max); fit.size < s.size {
			s = fit
		}
	}
	total := s.cap()*float64(len(ls)) + gap*float64(len(ls)-1)
	y := (LogoHeight-total)/2 + s.cap()
	end := 0.0
	for i, line := range ls {
		p.add(s.text(line, textX, y))
		end = textX + s.width(line)
		if i < len(ls)-1 {
			y += s.cap() + gap
		}
	}
	holderH = math.Min(holderH, s.cap()*1.3)
	if d.holder == "triangle" {
		holderH *= 1.4 // its number sits low in it, smaller
	}
	return p, Bug{Kind: d.holder, X: end + 26, Y: y - s.cap()/2 - holderH/2, W: holderWidth(d.holder, holderH), H: holderH}
}

// lines splits a name into one or two lines, the halves as even as can be.
func lines(name string) []string {
	ws := strings.Fields(name)
	if len(ws) < 2 || len([]rune(name)) <= 8 {
		return []string{name}
	}
	best, bestDiff := 1, math.MaxInt
	for i := 1; i < len(ws); i++ {
		d := len(strings.Join(ws[:i], " ")) - len(strings.Join(ws[i:], " "))
		if d < 0 {
			d = -d
		}
		if d < bestDiff {
			best, bestDiff = i, d
		}
	}
	return []string{strings.Join(ws[:best], " "), strings.Join(ws[best:], " ")}
}
