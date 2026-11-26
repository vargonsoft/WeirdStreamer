package guide

import (
	"math"
	"slices"
	"strconv"
)

// Symbols for the logos: plain one-color marks like a station's (a star, a bolt, a ringed planet…),
// each drawn in a unit square (y down) and then placed. Holes are shapes running the other way
// (counterclockwise), as glyphs' outlines do, so a digit added to a solid shape cuts it out.

// arcPoints are n+1 points on a circle from angle a0 to a1 (radians, clockwise on screen).
func arcPoints(c pt, r, a0, a1 float64, n int) []pt {
	out := make([]pt, 0, n+1)
	for i := 0; i <= n; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(n)
		out = append(out, pt{c.x + r*math.Cos(a), c.y + r*math.Sin(a)})
	}
	return out
}

// reversed is a polygon's points the other way round (a hole).
func reversed(ps []pt) []pt {
	out := slices.Clone(ps)
	slices.Reverse(out)
	return out
}

// sector is the band between radii r0 and r1 from angle a0 to a1 (a whole ring when they're a
// full turn apart).
func sector(c pt, r0, r1, a0, a1 float64) path {
	outer := arcPoints(c, r1, a0, a1, 48)
	inner := reversed(arcPoints(c, r0, a0, a1, 48))
	return polygon(append(outer, inner...)...)
}

func ring(c pt, r0, r1 float64) path { return sector(c, r0, r1, 0, 2*math.Pi) }

// ellipse is an ellipse rotated by a, clockwise (counterclockwise with hole).
func ellipse(c pt, rx, ry, a float64, hole bool) path {
	var ps []pt
	for i := range 64 {
		t := 2 * math.Pi * float64(i) / 64
		x, y := rx*math.Cos(t), ry*math.Sin(t)
		ps = append(ps, pt{c.x + x*math.Cos(a) - y*math.Sin(a), c.y + x*math.Sin(a) + y*math.Cos(a)})
	}
	if hole {
		ps = reversed(ps)
	}
	return polygon(ps...)
}

// ellipseRing is the band between two ellipses.
func ellipseRing(c pt, rx, ry, w, a float64) path {
	p := ellipse(c, rx, ry, a, false)
	p.add(ellipse(c, rx-w, ry-w, a, true))
	return p
}

func circleHole(c pt, r float64) path {
	return polygon(reversed(arcPoints(c, r, 0, 2*math.Pi, 64)[:64])...)
}

// starPoints are a star's n points between radii ro and ri, the first straight up.
func starPoints(c pt, ro, ri float64, n int) []pt {
	var ps []pt
	for i := range 2 * n {
		r := ro
		if i%2 == 1 {
			r = ri
		}
		a := -math.Pi/2 + math.Pi*float64(i)/float64(n)
		ps = append(ps, pt{c.x + r*math.Cos(a), c.y + r*math.Sin(a)})
	}
	return ps
}

// symbols draw in the unit square.
var symbols = map[string]func() path{
	"star": func() path { return polygon(starPoints(pt{.5, .53}, .5, .2, 5)...) },
	"starring": func() path {
		p := ring(pt{.5, .5}, .42, .5)
		p.add(polygon(starPoints(pt{.5, .53}, .34, .14, 5)...))
		return p
	},
	"bolt": func() path {
		return polygon(pt{.62, 0}, pt{.16, .58}, pt{.45, .58}, pt{.34, 1}, pt{.84, .38}, pt{.55, .38}, pt{.72, 0})
	},
	"planet": func() path {
		p := circle(pt{.5, .5}, .27)
		p.add(ellipseRing(pt{.5, .5}, .5, .17, .07, -0.35))
		return p
	},
	"sunburst": func() path {
		p := circle(pt{.5, .5}, .22)
		for i := range 12 {
			a := 2 * math.Pi * float64(i) / 12
			w := 0.09
			p.add(polygon(pt{.5 + .29*math.Cos(a-w), .5 + .29*math.Sin(a-w)}, pt{.5 + .5*math.Cos(a), .5 + .5*math.Sin(a)},
				pt{.5 + .29*math.Cos(a+w), .5 + .29*math.Sin(a+w)}))
		}
		return p
	},
	"globe": func() path {
		p := ring(pt{.5, .5}, .43, .5)
		p.add(ellipseRing(pt{.5, .5}, .2, .46, .06, 0))
		p.add(rect(.05, .46, .9, .08, 0))
		p.add(rect(.13, .25, .74, .06, 0))
		p.add(rect(.13, .69, .74, .06, 0))
		return p
	},
	"tower": func() path {
		p := polygon(pt{.5, .3}, pt{.62, 1}, pt{.38, 1})
		p.add(polygon(reversed([]pt{{.5, .52}, {.56, .88}, {.44, .88}})...))
		for _, r := range [][2]float64{{.12, .18}, {.26, .32}, {.40, .46}} {
			p.add(sector(pt{.5, .3}, r[0], r[1], -math.Pi*0.85, -math.Pi*0.15))
		}
		return p
	},
	"chevrons": func() path {
		var p path
		for i := range 3 {
			y := .08 + float64(i)*.3
			p.add(polygon(pt{.5, y}, pt{1, y + .28}, pt{1, y + .44}, pt{.5, y + .16}, pt{0, y + .44}, pt{0, y + .28}))
		}
		return p
	},
	"diamond": func() path {
		p := polygon(pt{.5, 0}, pt{1, .5}, pt{.5, 1}, pt{0, .5})
		p.add(polygon(reversed([]pt{{.5, .14}, {.86, .5}, {.5, .86}, {.14, .5}})...))
		p.add(polygon(pt{.5, .3}, pt{.7, .5}, pt{.5, .7}, pt{.3, .5}))
		return p
	},
	"waves": func() path {
		var p path
		for i := range 3 {
			y0 := .2 + float64(i)*.25
			var top, bottom []pt
			for k := 0; k <= 32; k++ {
				x := float64(k) / 32
				y := y0 + .07*math.Sin(x*2*math.Pi*1.5)
				top = append(top, pt{x, y})
				bottom = append(bottom, pt{x, y + .12})
			}
			p.add(polygon(append(top, reversed(bottom)...)...))
		}
		return p
	},
	"mountains": func() path {
		p := polygon(pt{0, 1}, pt{.36, .25}, pt{.56, .62}, pt{.72, .42}, pt{1, 1})
		p.add(polygon(reversed([]pt{{.36, .25}, {.47, .48}, {.4, .43}, {.36, .5}, {.31, .42}, {.25, .48}})...))
		return p
	},
	"bars": func() path {
		var p path
		for i, h := range []float64{.4, .7, 1, .7, .4} {
			p.add(rect(float64(i)*.21, 1-h, .16, h, .03))
		}
		return p
	},
	"hexagon": func() path {
		var outer, inner []pt
		for i := range 6 {
			a := math.Pi/6 + math.Pi*float64(i)/3
			outer = append(outer, pt{.5 + .5*math.Cos(a), .5 + .5*math.Sin(a)})
			inner = append(inner, pt{.5 + .38*math.Cos(a), .5 + .38*math.Sin(a)})
		}
		p := polygon(outer...)
		p.add(polygon(reversed(inner)...))
		p.add(circle(pt{.5, .5}, .16))
		return p
	},
	"crescent": func() path {
		outer := arcPoints(pt{.5, .5}, .5, math.Pi*0.3, math.Pi*1.7, 48)
		inner := reversed(arcPoints(pt{.68, .42}, .4, math.Pi*0.55, math.Pi*1.45, 48))
		return polygon(append(outer, inner...)...)
	},
	"sunset": func() path {
		var p path
		c, r := pt{.5, .62}, .5
		for i, b := range [][2]float64{{.12, .5}, {.55, .66}, {.71, .8}, {.85, .92}} {
			_ = i
			var left, right []pt
			for k := 0; k <= 16; k++ {
				y := b[0] + (b[1]-b[0])*float64(k)/16
				dy := y - c.y
				if math.Abs(dy) >= r {
					continue
				}
				dx := math.Sqrt(r*r - dy*dy)
				left = append(left, pt{c.x - dx, y})
				right = append(right, pt{c.x + dx, y})
			}
			if len(left) > 1 {
				p.add(polygon(append(right, reversed(left)...)...))
			}
		}
		return p
	},
	"crown": func() path {
		return polygon(pt{0, .2}, pt{.25, .55}, pt{.5, .1}, pt{.75, .55}, pt{1, .2}, pt{.88, .85}, pt{.12, .85})
	},
	"heart": func() path {
		p := circle(pt{.29, .33}, .26)
		p.add(circle(pt{.71, .33}, .26))
		p.add(polygon(pt{.05, .45}, pt{.95, .45}, pt{.5, .95}))
		return p
	},
	"bubble": func() path {
		p := rect(0, .05, 1, .7, .2)
		p.add(polygon(pt{.2, .7}, pt{.42, .7}, pt{.12, .98}))
		p.add(circleHole(pt{.28, .4}, .07))
		p.add(circleHole(pt{.5, .4}, .07))
		p.add(circleHole(pt{.72, .4}, .07))
		return p
	},
	"reel": func() path {
		p := circle(pt{.5, .5}, .5)
		for i := range 5 {
			a := -math.Pi/2 + 2*math.Pi*float64(i)/5
			p.add(circleHole(pt{.5 + .27*math.Cos(a), .5 + .27*math.Sin(a)}, .12))
		}
		p.add(circleHole(pt{.5, .5}, .05))
		return p
	},
	"clapper": func() path {
		p := rect(0, .42, 1, .55, .04)
		p.add(polygon(reversed([]pt{{.08, .5}, {.92, .5}, {.92, .56}, {.08, .56}})...))
		// the clapper, open, its stripes cut out
		arm := []pt{{0, .3}, {.95, .05}, {.99, .2}, {.04, .45}}
		p.add(polygon(arm...))
		for i := range 3 {
			x := .18 + float64(i)*.28
			p.add(polygon(reversed([]pt{{x, .27 - x*.26}, {x + .12, .24 - (x+.12)*.26}, {x + .08, .39 - (x+.08)*.26}, {x - .04, .42 - (x-.04)*.26}})...))
		}
		return p
	},
	"curtains": func() path {
		// two drapes swagged back under a scalloped valance
		var p path
		var valance []pt
		valance = append(valance, pt{0, 0}, pt{1, 0})
		for k := 0; k <= 40; k++ {
			x := 1 - float64(k)/40
			valance = append(valance, pt{x, .14 + .04*math.Abs(math.Sin(x*math.Pi*4))})
		}
		p.add(polygon(valance...))
		drape := func(flip bool) path {
			var ps []pt
			fx := func(x float64) float64 {
				if flip {
					return 1 - x
				}
				return x
			}
			ps = append(ps, pt{fx(0), .14}, pt{fx(.47), .14})
			for k := 1; k <= 16; k++ { // down and in to the tie-back
				t := float64(k) / 16
				ps = append(ps, pt{fx(.47 - .29*math.Sin(t*math.Pi/2)), .14 + .5*t})
			}
			for k := 1; k <= 8; k++ { // then out to the floor
				t := float64(k) / 8
				ps = append(ps, pt{fx(.18 + .1*t), .64 + .36*t})
			}
			ps = append(ps, pt{fx(0), 1})
			if flip {
				ps = reversed(ps)
			}
			return polygon(ps...)
		}
		p.add(drape(false))
		p.add(drape(true))
		return p
	},
	"rocket": func() path {
		p := polygon(pt{.5, 0}, pt{.66, .2}, pt{.66, .7}, pt{.34, .7}, pt{.34, .2})
		p.add(circleHole(pt{.5, .3}, .07))
		p.add(polygon(pt{.34, .45}, pt{.18, .8}, pt{.34, .7}))
		p.add(polygon(pt{.66, .45}, pt{.66, .7}, pt{.82, .8}))
		p.add(polygon(pt{.4, .74}, pt{.6, .74}, pt{.5, 1}))
		return p
	},
	"bat": func() path {
		return polygon(pt{.5, .3}, pt{.56, .2}, pt{.6, .32}, pt{.75, .18}, pt{1, .25}, pt{.92, .45}, pt{.82, .42}, pt{.76, .58},
			pt{.64, .5}, pt{.5, .72}, pt{.36, .5}, pt{.24, .58}, pt{.18, .42}, pt{.08, .45}, pt{0, .25}, pt{.25, .18},
			pt{.4, .32}, pt{.44, .2})
	},
	"house": func() path {
		p := polygon(pt{.5, .05}, pt{1, .5}, pt{.86, .5}, pt{.86, 1}, pt{.14, 1}, pt{.14, .5}, pt{0, .5})
		p.add(polygon(reversed([]pt{{.42, .66}, {.58, .66}, {.58, 1.01}, {.42, 1.01}})...))
		return p
	},
	"smile": func() path {
		p := ring(pt{.5, .5}, .43, .5)
		p.add(ellipse(pt{.36, .38}, .06, .09, 0, false))
		p.add(ellipse(pt{.64, .38}, .06, .09, 0, false))
		p.add(sector(pt{.5, .5}, .22, .3, math.Pi*0.15, math.Pi*0.85))
		return p
	},
	"arrow": func() path {
		return polygon(pt{0, .35}, pt{.55, .35}, pt{.55, .1}, pt{1, .5}, pt{.55, .9}, pt{.55, .65}, pt{0, .65})
	},
	"atom": func() path {
		p := circle(pt{.5, .5}, .1)
		for i := range 3 {
			p.add(ellipseRing(pt{.5, .5}, .5, .18, .05, math.Pi*float64(i)/3))
		}
		return p
	},
	"eye-less burst": func() path { // an eight-point burst, a hole in the middle
		p := polygon(starPoints(pt{.5, .5}, .5, .32, 8)...)
		p.add(circleHole(pt{.5, .5}, .18))
		return p
	},
}

// symbol is a symbol filling the square at x, y of side s (centered, its shape kept).
func symbol(name string, x, y, s float64) path { return symbolIn(name, x, y, s, s) }

// symbolIn is a symbol as large as fits the box at x, y of w × h (centered, its shape kept).
func symbolIn(name string, x, y, bw, bh float64) path {
	f := symbols[name]
	if f == nil {
		f = symbols["star"]
	}
	p := f()
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, sg := range p {
		for _, q := range sg.p {
			if sg.op == opClose {
				continue
			}
			minX, maxX = math.Min(minX, q.x), math.Max(maxX, q.x)
			minY, maxY = math.Min(minY, q.y), math.Max(maxY, q.y)
		}
	}
	w, h := maxX-minX, maxY-minY
	if w <= 0 || h <= 0 {
		return p.transform(matrix{a: bw, d: bh, e: x, f: y})
	}
	k := math.Min(bw/w, bh/h)
	return p.transform(matrix{a: k, d: k, e: x + (bw-w*k)/2 - minX*k, f: y + (bh-h*k)/2 - minY*k})
}

// numberHolder draws a channel's number in a holder filling the box (x, y, w, h): outlined (ring,
// square, rect) with the number inside, or solid (disc, block, bar, triangle) with the number cut
// out of it. One path, in one color.
func numberHolder(kind string, number int, x, y, w, h float64) (path, error) {
	f, err := digitFont()
	if err != nil {
		return nil, err
	}
	n := strconv.Itoa(number)
	st := textStyle{font: f, sx: 0.86}
	c := pt{x + w/2, y + h/2}
	s := math.Min(w, h)
	var p path
	room := 0.0 // how much of the holder the number may fill
	switch kind {
	case "ring":
		p = ring(c, s/2*0.86, s/2)
		room = 0.62 * s
	case "disc":
		p = circle(c, s/2)
		room = 0.66 * s
	case "square":
		p = rect(c.x-s/2, c.y-s/2, s, s, s*0.08)
		p.add(polygon(reversed(rectPoints(c.x-s/2*0.86, c.y-s/2*0.86, s*0.86, s*0.86))...))
		room = 0.66 * s
	case "block":
		p = rect(c.x-s/2, c.y-s/2, s, s, s*0.08)
		room = 0.7 * s
	case "rect":
		p = rect(x, y, w, h, h*0.12)
		p.add(polygon(reversed(rectPoints(x+h*0.08, y+h*0.08, w-h*0.16, h*0.84))...))
		room = 0.8 * math.Min(w, h*1.4)
	case "bar":
		p = rect(x, y, w, h, h*0.18)
		room = 0.8 * math.Min(w, h*1.4)
	case "triangle":
		top, base := pt{c.x, y}, y+h
		p = polygon(top, pt{c.x + w/2, base}, pt{c.x - w/2, base})
		// the number low in the triangle, where it's wide
		st = st.fit(n, w*0.42, h*0.42/capHeight(f))
		p.add(reversePath(st.centered(n, pt{c.x, y + h*0.68})))
		return p, nil
	}
	st = st.fit(n, room, room/capHeight(f))
	switch kind {
	case "rect", "bar":
		st = st.fit(n, w*0.78, h*0.62/capHeight(f))
	}
	digits := st.centered(n, c)
	if kind == "disc" || kind == "block" || kind == "bar" {
		digits = reversePath(digits) // cut out of the solid holder
	}
	p.add(digits)
	return p, nil
}

// reversePath is p with each of its outlines running the other way (a filled shape becomes a hole).
func reversePath(p path) path {
	var out path
	start := 0
	for start < len(p) {
		end := start + 1
		for end < len(p) && p[end].op != opMove {
			end++
		}
		sub := p[start:end]
		// the points each segment ends at; a closing segment adds none
		var ends []pt
		var segs []seg
		for _, s := range sub {
			switch s.op {
			case opMove, opLine:
				ends = append(ends, s.p[0])
				segs = append(segs, s)
			case opQuad:
				ends = append(ends, s.p[1])
				segs = append(segs, s)
			case opCube:
				ends = append(ends, s.p[2])
				segs = append(segs, s)
			}
		}
		if len(ends) > 0 {
			out.moveTo(ends[len(ends)-1])
			for i := len(segs) - 1; i >= 1; i-- {
				s, to := segs[i], ends[i-1]
				switch s.op {
				case opLine:
					out.lineTo(to)
				case opQuad:
					out.quadTo(s.p[0], to)
				case opCube:
					out.cubeTo(s.p[1], s.p[0], to)
				}
			}
			out.close()
		}
		start = end
	}
	return out
}

// rectPoints are a rectangle's corners, clockwise.
func rectPoints(x, y, w, h float64) []pt {
	return []pt{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}
}
