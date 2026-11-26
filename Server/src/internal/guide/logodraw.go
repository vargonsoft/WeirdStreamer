package guide

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

// Drawing for the channel logos (logo.go): filled shapes, solid or with a linear gradient, on a
// 1024 × 512 canvas, written as SVG or rasterized to PNG at any size.

// The logo's canvas, in its own units: 2:1.
const (
	LogoWidth  = 1024
	LogoHeight = 512
)

// The plate a logo sits on: 8 in from the canvas's edges, corners of radius 56.
const plateMargin, plateRadius = 8.0, 56.0

type pt struct{ x, y float64 }

type segOp byte

const (
	opMove segOp = iota
	opLine
	opQuad
	opCube
	opClose
)

type seg struct {
	op segOp
	p  [3]pt
}

type path []seg

func (p *path) moveTo(a pt)       { *p = append(*p, seg{op: opMove, p: [3]pt{a}}) }
func (p *path) lineTo(a pt)       { *p = append(*p, seg{op: opLine, p: [3]pt{a}}) }
func (p *path) quadTo(a, b pt)    { *p = append(*p, seg{op: opQuad, p: [3]pt{a, b}}) }
func (p *path) cubeTo(a, b, c pt) { *p = append(*p, seg{op: opCube, p: [3]pt{a, b, c}}) }
func (p *path) close()            { *p = append(*p, seg{op: opClose}) }
func (p *path) add(q path)        { *p = append(*p, q...) }

// paint is a solid color or a linear gradient (from a to b, its stops by offset), or mono: the one
// color a logo is drawn in, chosen when it's drawn.
type paint struct {
	solid color.NRGBA
	a, b  pt
	stops []stop
	mono  bool
}

var mono = paint{mono: true}

// tinted is shapes with mono paint in c.
func tinted(shapes []shape, c color.NRGBA) []shape {
	out := make([]shape, len(shapes))
	for i, s := range shapes {
		out[i] = s
		if s.fill.mono {
			out[i].fill = solid(c)
		}
	}
	return out
}

type stop struct {
	at float64
	c  color.NRGBA
}

func solid(c color.NRGBA) paint { return paint{solid: c} }

func gradient(a, b pt, stops ...stop) paint { return paint{a: a, b: b, stops: stops} }

func (f paint) visible() bool { return f.mono || len(f.stops) > 0 || f.solid.A > 0 }

type shape struct {
	p    path
	fill paint
}

// matrix is an affine transform: x' = a·x + c·y + e, y' = b·x + d·y + f.
type matrix struct{ a, b, c, d, e, f float64 }

func (m matrix) apply(p pt) pt {
	return pt{m.a*p.x + m.c*p.y + m.e, m.b*p.x + m.d*p.y + m.f}
}

func translate(x, y float64) matrix { return matrix{1, 0, 0, 1, x, y} }

func rotate(a float64) matrix {
	return matrix{math.Cos(a), math.Sin(a), -math.Sin(a), math.Cos(a), 0, 0}
}

// mul is m after n.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		a: m.a*n.a + m.c*n.b, b: m.b*n.a + m.d*n.b,
		c: m.a*n.c + m.c*n.d, d: m.b*n.c + m.d*n.d,
		e: m.a*n.e + m.c*n.f + m.e, f: m.b*n.e + m.d*n.f + m.f,
	}
}

func (p path) transform(m matrix) path {
	out := make(path, len(p))
	for i, s := range p {
		out[i].op = s.op
		for j := range s.p {
			out[i].p[j] = m.apply(s.p[j])
		}
	}
	return out
}

const kappa = 0.5523 // a quarter circle's control points, as a fraction of its radius

// rect is a rectangle with corners of radius r, clockwise.
func rect(x, y, w, h, r float64) path {
	r = math.Min(r, math.Min(w, h)/2)
	k := kappa * r
	var p path
	p.moveTo(pt{x + r, y})
	p.lineTo(pt{x + w - r, y})
	p.cubeTo(pt{x + w - r + k, y}, pt{x + w, y + r - k}, pt{x + w, y + r})
	p.lineTo(pt{x + w, y + h - r})
	p.cubeTo(pt{x + w, y + h - r + k}, pt{x + w - r + k, y + h}, pt{x + w - r, y + h})
	p.lineTo(pt{x + r, y + h})
	p.cubeTo(pt{x + r - k, y + h}, pt{x, y + h - r + k}, pt{x, y + h - r})
	p.lineTo(pt{x, y + r})
	p.cubeTo(pt{x, y + r - k}, pt{x + r - k, y}, pt{x + r, y})
	p.close()
	return p
}

// circle at c of radius r, clockwise.
func circle(c pt, r float64) path {
	k := kappa * r
	var p path
	p.moveTo(pt{c.x + r, c.y})
	p.cubeTo(pt{c.x + r, c.y + k}, pt{c.x + k, c.y + r}, pt{c.x, c.y + r})
	p.cubeTo(pt{c.x - k, c.y + r}, pt{c.x - r, c.y + k}, pt{c.x - r, c.y})
	p.cubeTo(pt{c.x - r, c.y - k}, pt{c.x - k, c.y - r}, pt{c.x, c.y - r})
	p.cubeTo(pt{c.x + k, c.y - r}, pt{c.x + r, c.y - k}, pt{c.x + r, c.y})
	p.close()
	return p
}

// polygon through the points, closed.
func polygon(ps ...pt) path {
	var p path
	p.moveTo(ps[0])
	for _, q := range ps[1:] {
		p.lineTo(q)
	}
	p.close()
	return p
}

// plate is the logo's background shape.
func plate() path {
	return rect(plateMargin, plateMargin, LogoWidth-2*plateMargin, LogoHeight-2*plateMargin, plateRadius)
}

// plateInset is how far in from the canvas's sides the plate's edge is at height y (more in its
// rounded corners).
func plateInset(y float64) float64 {
	top, bottom := plateMargin+plateRadius, LogoHeight-plateMargin-plateRadius
	var dy float64
	switch {
	case y < top:
		dy = math.Min(top-y, plateRadius)
	case y > bottom:
		dy = math.Min(y-bottom, plateRadius)
	default:
		return plateMargin
	}
	return plateMargin + plateRadius - math.Sqrt(plateRadius*plateRadius-dy*dy)
}

// band is the plate between heights y0 and y1, following its rounded corners.
func band(y0, y1 float64) path {
	var left, right []pt
	const steps = 12
	for i := 0; i <= steps; i++ {
		y := y0 + (y1-y0)*float64(i)/steps
		in := plateInset(y)
		left = append(left, pt{in, y})
		right = append(right, pt{LogoWidth - in, y})
	}
	var ps []pt
	ps = append(ps, right...)
	for i := len(left) - 1; i >= 0; i-- {
		ps = append(ps, left[i])
	}
	return polygon(ps...)
}

func hexColor(s string) color.NRGBA {
	var r, g, b uint8
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.NRGBA{0xE4, 0x57, 0x2E, 0xFF}
	}
	return color.NRGBA{r, g, b, 0xFF}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// darker mixes c toward black by f (0 to 1).
func darker(c color.NRGBA, f float64) color.NRGBA {
	return color.NRGBA{uint8(float64(c.R) * (1 - f)), uint8(float64(c.G) * (1 - f)), uint8(float64(c.B) * (1 - f)), c.A}
}

// lighter mixes c toward white by f.
func lighter(c color.NRGBA, f float64) color.NRGBA {
	mix := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*f) }
	return color.NRGBA{mix(c.R), mix(c.G), mix(c.B), c.A}
}

var (
	white  = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	shadow = color.NRGBA{0, 0, 0, 0x99}
)

func colorString(c color.NRGBA) string {
	if c.A == 0xFF {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// parseColor reads colorString's "#rrggbb" or "#rrggbbaa".
func parseColor(s string) color.NRGBA {
	s = strings.TrimPrefix(s, "#")
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}
	}
	if len(s) == 8 {
		return color.NRGBA{uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v)}
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xFF}
}

func svgColor(c color.NRGBA) string {
	if c.A == 0xFF {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", c.R, c.G, c.B, float64(c.A)/255)
}

func num(v float64) string { return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64) }

// pathData is the path in SVG's notation (absolute M, L, Q, C and Z).
func pathData(p path) string {
	var b strings.Builder
	for _, s := range p {
		switch s.op {
		case opMove:
			b.WriteString("M" + num(s.p[0].x) + " " + num(s.p[0].y))
		case opLine:
			b.WriteString("L" + num(s.p[0].x) + " " + num(s.p[0].y))
		case opQuad:
			b.WriteString("Q" + num(s.p[0].x) + " " + num(s.p[0].y) + " " + num(s.p[1].x) + " " + num(s.p[1].y))
		case opCube:
			b.WriteString("C" + num(s.p[0].x) + " " + num(s.p[0].y) + " " + num(s.p[1].x) + " " + num(s.p[1].y) +
				" " + num(s.p[2].x) + " " + num(s.p[2].y))
		case opClose:
			b.WriteString("Z")
		}
	}
	return b.String()
}

// parsePathData reads pathData's notation back.
func parsePathData(d string) (path, error) {
	var p path
	i := 0
	nums := func(n int) ([]float64, error) {
		out := make([]float64, 0, n)
		for len(out) < n {
			for i < len(d) && (d[i] == ' ' || d[i] == ',') {
				i++
			}
			j := i
			for j < len(d) && (d[j] == '-' || d[j] == '.' || d[j] >= '0' && d[j] <= '9' || d[j] == 'e') {
				j++
			}
			v, err := strconv.ParseFloat(d[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("path data at %d: %w", i, err)
			}
			out = append(out, v)
			i = j
		}
		return out, nil
	}
	for i < len(d) {
		c := d[i]
		i++
		switch c {
		case ' ':
		case 'M', 'L':
			v, err := nums(2)
			if err != nil {
				return nil, err
			}
			if c == 'M' {
				p.moveTo(pt{v[0], v[1]})
			} else {
				p.lineTo(pt{v[0], v[1]})
			}
		case 'Q':
			v, err := nums(4)
			if err != nil {
				return nil, err
			}
			p.quadTo(pt{v[0], v[1]}, pt{v[2], v[3]})
		case 'C':
			v, err := nums(6)
			if err != nil {
				return nil, err
			}
			p.cubeTo(pt{v[0], v[1]}, pt{v[2], v[3]}, pt{v[4], v[5]})
		case 'Z':
			p.close()
		default:
			return nil, fmt.Errorf("path data: unknown command %q", c)
		}
	}
	return p, nil
}

// svgOf writes shapes as an SVG document of the logo's canvas.
func svgOf(shapes []shape) []byte {
	var defs, body bytes.Buffer
	for i, s := range shapes {
		if !s.fill.visible() {
			continue
		}
		fill := svgColor(s.fill.solid)
		if len(s.fill.stops) > 0 {
			id := fmt.Sprintf("g%d", i)
			fmt.Fprintf(&defs, `<linearGradient id="%s" gradientUnits="userSpaceOnUse" x1="%s" y1="%s" x2="%s" y2="%s">`,
				id, num(s.fill.a.x), num(s.fill.a.y), num(s.fill.b.x), num(s.fill.b.y))
			for _, st := range s.fill.stops {
				fmt.Fprintf(&defs, `<stop offset="%.3f" stop-color="%s"/>`, st.at, svgColor(st.c))
			}
			defs.WriteString(`</linearGradient>`)
			fill = "url(#" + id + ")"
		}
		fmt.Fprintf(&body, `<path fill="%s" d="%s"/>`, fill, pathData(s.p))
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`,
		LogoWidth, LogoHeight, LogoWidth, LogoHeight)
	if defs.Len() > 0 {
		b.WriteString("<defs>")
		b.Write(defs.Bytes())
		b.WriteString("</defs>")
	}
	b.Write(body.Bytes())
	b.WriteString("</svg>\n")
	return b.Bytes()
}

// pngOf rasterizes shapes to a PNG width pixels wide (half as high), transparent where nothing is.
func pngOf(shapes []shape, width int) ([]byte, error) {
	width = min(max(width, 32), 4096)
	height := width * LogoHeight / LogoWidth
	scale := float64(width) / LogoWidth
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	z := vector.NewRasterizer(width, height)
	for _, s := range shapes {
		if !s.fill.visible() {
			continue
		}
		z.Reset(width, height)
		open := false
		for _, sg := range s.p {
			p := func(i int) (float32, float32) { return float32(sg.p[i].x * scale), float32(sg.p[i].y * scale) }
			switch sg.op {
			case opMove:
				if open {
					z.ClosePath()
				}
				x, y := p(0)
				z.MoveTo(x, y)
				open = true
			case opLine:
				x, y := p(0)
				z.LineTo(x, y)
			case opQuad:
				x0, y0 := p(0)
				x1, y1 := p(1)
				z.QuadTo(x0, y0, x1, y1)
			case opCube:
				x0, y0 := p(0)
				x1, y1 := p(1)
				x2, y2 := p(2)
				z.CubeTo(x0, y0, x1, y1, x2, y2)
			case opClose:
				z.ClosePath()
				open = false
			}
		}
		if open {
			z.ClosePath()
		}
		z.Draw(dst, dst.Bounds(), paintImage(s.fill, scale), image.Point{})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, dst); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func paintImage(f paint, scale float64) image.Image {
	if len(f.stops) == 0 {
		return image.NewUniform(f.solid)
	}
	return &gradientImage{f: f, scale: scale}
}

// gradientImage is a linear gradient as an image, for compositing through a shape's mask.
type gradientImage struct {
	f     paint
	scale float64
}

func (g *gradientImage) ColorModel() color.Model { return color.NRGBAModel }

func (g *gradientImage) Bounds() image.Rectangle { return image.Rect(-1e6, -1e6, 1e6, 1e6) }

func (g *gradientImage) At(x, y int) color.Color {
	px, py := (float64(x)+0.5)/g.scale, (float64(y)+0.5)/g.scale
	dx, dy := g.f.b.x-g.f.a.x, g.f.b.y-g.f.a.y
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = ((px-g.f.a.x)*dx + (py-g.f.a.y)*dy) / l
	}
	stops := g.f.stops
	if t <= stops[0].at {
		return stops[0].c
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].at {
			a, b := stops[i-1], stops[i]
			u := (t - a.at) / (b.at - a.at)
			mix := func(p, q uint8) uint8 { return uint8(float64(p) + (float64(q)-float64(p))*u + 0.5) }
			return color.NRGBA{mix(a.c.R, b.c.R), mix(a.c.G, b.c.G), mix(a.c.B, b.c.B), mix(a.c.A, b.c.A)}
		}
	}
	return stops[len(stops)-1].c
}
