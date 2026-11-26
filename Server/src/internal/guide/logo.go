package guide

import (
	"embed"
	"encoding/json"
	"fmt"
	"image/color"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Channel logos (docs/GUIDE.md), in one color as stations' were: a symbol with the name beside or
// below it, and the channel's number in a holder (a ring, a square, a triangle…). Each is artwork
// made once by the designer (cmd/logoart, from logodesign.go, with display fonts that stay on the
// machine that ran it): its letters are outlines, saved in logoart/ with where the number goes.
// Genre channels have a logo each (Comedy, Fright Night…); network channels take one of a pool of
// made-up stations (its call sign is the channel's name). The server draws the number into its
// holder and serves the logo, in the color asked for, as SVG or as PNG at any size.

//go:embed logoart/*.json
var artFiles embed.FS

// Artwork is a logo as the designer made it.
type Artwork struct {
	ID     string     `json:"id"`   // "genre-comedy", "station-wkbt"
	Name   string     `json:"name"` // the channel name it's for: a genre's ("Comedy") or a call sign
	Font   string     `json:"font"` // the font its letters came from (each channel's is its own)
	Bug    Bug        `json:"bug"`
	Shapes []artShape `json:"shapes"`
}

// Bug is where the server draws the channel's number: a holder (numberHolder's kinds) filling a box.
type Bug struct {
	Kind string  `json:"kind"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// artShape is a shape of the artwork: in the logo's one color, unless it has a fill or gradient.
type artShape struct {
	D    string   `json:"d"`
	Fill string   `json:"fill,omitempty"`
	Grad *artGrad `json:"grad,omitempty"`
}

type artGrad struct {
	X1    float64   `json:"x1"`
	Y1    float64   `json:"y1"`
	X2    float64   `json:"x2"`
	Y2    float64   `json:"y2"`
	Stops []artStop `json:"stops"`
}

type artStop struct {
	At    float64 `json:"at"`
	Color string  `json:"color"`
}

type artLibrary struct {
	genres   map[string]*Artwork // by genre name
	stations []*Artwork          // the pool, by ID
	err      error
}

var artwork = sync.OnceValue(func() *artLibrary {
	lib := &artLibrary{genres: map[string]*Artwork{}}
	files, err := fs.Glob(artFiles, "logoart/*.json")
	if err != nil {
		lib.err = err
		return lib
	}
	for _, f := range files {
		b, err := artFiles.ReadFile(f)
		if err != nil {
			lib.err = err
			return lib
		}
		var a Artwork
		if err := json.Unmarshal(b, &a); err != nil {
			lib.err = fmt.Errorf("%s: %w", f, err)
			return lib
		}
		if strings.HasPrefix(a.ID, "station-") {
			lib.stations = append(lib.stations, &a)
		} else {
			lib.genres[a.Name] = &a
		}
	}
	sort.Slice(lib.stations, func(i, j int) bool { return lib.stations[i].ID < lib.stations[j].ID })
	return lib
})

// StationPool is the made-up stations network channels are named after, in order.
func StationPool() []*Artwork { return artwork().stations }

// PickStation is the call sign a new network channel takes from the pool: in an order its key
// sets, the first station not taken whose font no other channel's logo uses (taken and fonts are
// those of the channels there are), else the first not taken.
func PickStation(key string, taken, fonts map[string]bool) string {
	pool := slices.Clone(StationPool())
	if len(pool) == 0 {
		return "WXYZ"
	}
	sort.SliceStable(pool, func(i, j int) bool { return hash(key+pool[i].ID) < hash(key+pool[j].ID) })
	for _, a := range pool {
		if !taken[a.Name] && !fonts[a.Font] {
			return a.Name
		}
	}
	for _, a := range pool {
		if !taken[a.Name] {
			return a.Name
		}
	}
	return pool[0].Name
}

// IsStation reports whether name is a station of the pool.
func IsStation(name string) bool {
	return slices.ContainsFunc(StationPool(), func(a *Artwork) bool { return a.Name == name })
}

// GenreFonts are the fonts the genre channels' logos use (a station picked for a channel avoids them).
func GenreFonts() map[string]bool {
	out := map[string]bool{}
	for _, a := range artwork().genres {
		out[a.Font] = true
	}
	return out
}

// LogoFor is a channel's logo: its station's (by call sign) for a network channel, else its
// genre's, else Variety's.
func LogoFor(name, genre string, network bool) *Artwork {
	lib := artwork()
	if network {
		for _, a := range lib.stations {
			if a.Name == name {
				return a
			}
		}
	}
	if a := lib.genres[genre]; a != nil {
		return a
	}
	if a := lib.genres[name]; a != nil {
		return a
	}
	if a := lib.genres["Variety"]; a != nil {
		return a
	}
	if len(lib.stations) > 0 {
		return lib.stations[0]
	}
	return nil
}

// shapes is the artwork with the channel's number in its bug.
func (a *Artwork) shapes(number int) ([]shape, error) {
	var out []shape
	for _, s := range a.Shapes {
		p, err := parsePathData(s.D)
		if err != nil {
			return nil, fmt.Errorf("logo %s: %w", a.ID, err)
		}
		f := mono
		if s.Fill != "" {
			f = solid(parseColor(s.Fill))
		}
		if g := s.Grad; g != nil {
			f = paint{a: pt{g.X1, g.Y1}, b: pt{g.X2, g.Y2}}
			for _, st := range g.Stops {
				f.stops = append(f.stops, stop{st.At, parseColor(st.Color)})
			}
		}
		out = append(out, shape{p, f})
	}
	holder, err := numberHolder(a.Bug.Kind, number, a.Bug.X, a.Bug.Y, a.Bug.W, a.Bug.H)
	if err != nil {
		return nil, err
	}
	return append(out, shape{holder, mono}), nil
}

// SVG is the logo with the channel's number in color c, as an SVG document.
func (a *Artwork) SVG(number int, c color.NRGBA) ([]byte, error) {
	s, err := a.shapes(number)
	if err != nil {
		return nil, err
	}
	return svgOf(tinted(s, c)), nil
}

// PNG is the logo with the channel's number in color c, width pixels wide (half as high),
// transparent around it.
func (a *Artwork) PNG(number, width int, c color.NRGBA) ([]byte, error) {
	s, err := a.shapes(number)
	if err != nil {
		return nil, err
	}
	return pngOf(tinted(s, c), width)
}

// LogoColor reads a logo's color from "rrggbb" (white if it isn't one).
func LogoColor(hex string) color.NRGBA {
	if len(hex) != 6 {
		return white
	}
	c := parseColor(hex)
	if c.A == 0 {
		return white
	}
	return c
}
