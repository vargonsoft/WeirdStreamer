package guide

import (
	_ "embed"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Letters for the logos: a font's glyphs turned into outlines. The designer (logodesign.go) lays out
// names in many fonts; the server only draws channel numbers, in Liberation Sans Bold (SIL Open
// Font License, logofont/OFL.txt), the one font it carries.

//go:embed logofont/LiberationSans-Bold.ttf
var digitFontData []byte

var digitFont = sync.OnceValues(func() (*sfnt.Font, error) { return sfnt.Parse(digitFontData) })

const fontUnits = 1000 // outlines are loaded at this size, then scaled to size 1

// glyph is one letter's outline at size 1 (an em), from its origin on the baseline, and its advance.
type glyph struct {
	p       path
	advance float64
}

// layout is text set in f at size 1: each letter's outline and where it starts, and the width.
func layout(f *sfnt.Font, text string) ([]glyph, []float64, float64) {
	var b sfnt.Buffer
	ppem := fixed.I(fontUnits)
	var out []glyph
	var xs []float64
	x := 0.0
	var prev sfnt.GlyphIndex
	for i, r := range text {
		idx, err := f.GlyphIndex(&b, r)
		if err != nil || idx == 0 {
			idx, _ = f.GlyphIndex(&b, '?')
		}
		if i > 0 {
			if k, err := f.Kern(&b, prev, idx, ppem, font.HintingNone); err == nil {
				x += float64(k) / 64 / fontUnits
			}
		}
		g := glyph{}
		if adv, err := f.GlyphAdvance(&b, idx, ppem, font.HintingNone); err == nil {
			g.advance = float64(adv) / 64 / fontUnits
		}
		segs, _ := f.LoadGlyph(&b, idx, ppem, nil)
		conv := func(p fixed.Point26_6) pt {
			return pt{float64(p.X) / 64 / fontUnits, float64(p.Y) / 64 / fontUnits}
		}
		open := false
		for _, s := range segs {
			switch s.Op {
			case sfnt.SegmentOpMoveTo:
				if open {
					g.p.close()
				}
				g.p.moveTo(conv(s.Args[0]))
				open = true
			case sfnt.SegmentOpLineTo:
				g.p.lineTo(conv(s.Args[0]))
			case sfnt.SegmentOpQuadTo:
				g.p.quadTo(conv(s.Args[0]), conv(s.Args[1]))
			case sfnt.SegmentOpCubeTo:
				g.p.cubeTo(conv(s.Args[0]), conv(s.Args[1]), conv(s.Args[2]))
			}
		}
		if open {
			g.p.close()
		}
		out = append(out, g)
		xs = append(xs, x)
		x += g.advance
		prev = idx
	}
	return out, xs, x
}

// capHeight is how tall f's capitals are at size 1 (an H's top), for centering lines.
func capHeight(f *sfnt.Font) float64 {
	gs, _, _ := layout(f, "H")
	top := 0.0
	for _, s := range gs[0].p {
		for _, q := range s.p[:1] {
			top = math.Min(top, q.y)
		}
	}
	if top == 0 {
		return 0.7
	}
	return -top
}

// textStyle sets how a line of text is drawn: its font and size, narrowing (sx < 1, or wider),
// slant (leaning right) and extra spacing between letters (a fraction of the size).
type textStyle struct {
	font                      *sfnt.Font
	size, sx, slant, tracking float64
}

func (st textStyle) cap() float64 { return capHeight(st.font) * st.size }

// width is how wide text is drawn, its lean included.
func (st textStyle) width(text string) float64 {
	_, _, w := layout(st.font, text)
	n := float64(len([]rune(text)))
	return w*st.size*st.sx + st.tracking*st.size*math.Max(n-1, 0) + st.slant*st.cap()
}

// fit is st at the size that fills width (at most max).
func (st textStyle) fit(text string, width, max float64) textStyle {
	st.size = 1
	w := st.width(text)
	st.size = max
	if w > 0 {
		st.size = math.Min(max, width/w)
	}
	return st
}

// letters is text laid out from x along the baseline y: one path per letter (for letters painted
// apart), each transformed by each(i) about its own middle first.
func (st textStyle) letters(text string, x, y float64, each func(i int) matrix) []path {
	gs, xs, _ := layout(st.font, text)
	var out []path
	for i, g := range gs {
		gx := x + xs[i]*st.size*st.sx + st.tracking*st.size*float64(i)
		// size, narrowing and slant (y is down: a slant leans the tops right)
		m := matrix{a: st.size * st.sx, d: st.size, c: -st.slant * st.size, e: gx, f: y}
		if each != nil {
			cx, cy := gx+g.advance*st.size*st.sx/2, y-st.cap()/2
			m = translate(cx, cy).mul(each(i)).mul(translate(-cx, -cy)).mul(m)
		}
		out = append(out, g.p.transform(m))
	}
	return out
}

// text is text laid out from x along the baseline y as one path.
func (st textStyle) text(text string, x, y float64) path {
	var p path
	for _, g := range st.letters(text, x, y, nil) {
		p.add(g)
	}
	return p
}

// centered is text centered on c (its capitals' middle at c.y).
func (st textStyle) centered(text string, c pt) path {
	return st.text(text, c.x-st.width(text)/2, c.y+st.cap()/2)
}
