package guide

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"golang.org/x/image/vector"
)

// The Weather channel's condition icons (WeatherIcons), drawn like the logos' symbols: one color,
// in the unit square (y down), served as PNGs by GET /api/guide/weather/icon.

// cloudShape is a cloud w wide with its top left at (x, y), about 0.56 w high.
func cloudShape(x, y, w float64) path {
	at := func(px, py float64) pt { return pt{x + px*w, y + py*w} }
	p := circle(at(.28, .36), .2*w)
	p.add(circle(at(.52, .27), .26*w))
	p.add(circle(at(.76, .38), .17*w))
	p.add(rect(x+.08*w, y+.34*w, .84*w, .24*w, .12*w))
	return p
}

var weatherSymbols = map[string]func() path{
	"sun": func() path {
		p := circle(pt{.5, .5}, .2)
		for i := range 8 {
			a := 2 * math.Pi * float64(i) / 8
			w := 0.14
			p.add(polygon(pt{.5 + .28*math.Cos(a-w), .5 + .28*math.Sin(a-w)}, pt{.5 + .48*math.Cos(a), .5 + .48*math.Sin(a)},
				pt{.5 + .28*math.Cos(a+w), .5 + .28*math.Sin(a+w)}))
		}
		return p
	},
	"moon":     func() path { return symbols["crescent"]() },
	"cloudsun": func() path { return symbols["cloudsun"]() },
	"cloudmoon": func() path {
		p := symbolIn("crescent", .02, .04, .46, .46)
		p.add(cloudShape(.18, .4, .82))
		return p
	},
	"cloud": func() path { return cloudShape(0, .22, 1) },
	"rain": func() path {
		p := cloudShape(0, 0, 1)
		for i := range 3 {
			x := .3 + .2*float64(i)
			p.add(polygon(pt{x, .64}, pt{x + .07, .64}, pt{x - .01, .96}, pt{x - .08, .96}))
		}
		return p
	},
	"snow": func() path {
		p := cloudShape(0, 0, 1)
		for _, c := range []pt{{.3, .78}, {.52, .88}, {.74, .76}} {
			p.add(polygon(starPoints(c, .1, .035, 6)...))
		}
		return p
	},
	"storm": func() path {
		p := cloudShape(0, 0, 1)
		p.add(symbolIn("bolt", .36, .48, .3, .52))
		return p
	},
	"fog": func() path {
		var p path
		for i, span := range [][2]float64{{.05, .95}, {.15, .85}, {.05, .8}, {.2, .95}} {
			p.add(rect(span[0], .18+.18*float64(i), span[1]-span[0], .1, .05))
		}
		return p
	},
	"wind": func() path { // three gusts: the top one curling up, the bottom one down
		p := rect(.05, .3, .55, .08, .04)
		p.add(sector(pt{.6, .24}, .06, .14, -math.Pi*0.9, math.Pi/2))
		p.add(rect(.05, .52, .85, .08, .04))
		p.add(rect(.05, .74, .45, .08, .04))
		p.add(sector(pt{.5, .88}, .06, .14, -math.Pi/2, math.Pi*0.9))
		return p
	},
}

// WeatherIconPNG draws a condition icon size pixels square, in c, transparent around it.
func WeatherIconPNG(name string, size int, c color.NRGBA) ([]byte, bool) {
	f := weatherSymbols[name]
	if f == nil {
		return nil, false
	}
	size = min(max(size, 16), 1024)
	margin := float64(size) * 0.06
	p := fitPath(f(), margin, margin, float64(size)-2*margin, float64(size)-2*margin)
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	z := vector.NewRasterizer(size, size)
	addPath(z, p, 1)
	z.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{})
	var b bytes.Buffer
	if png.Encode(&b, dst) != nil {
		return nil, false
	}
	return b.Bytes(), true
}
