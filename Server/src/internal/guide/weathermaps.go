package guide

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/vector"
)

// The Weather channel's regional and national pages (docs/GUIDE.md): the weather now and today's forecast in
// the largest cities around the ZIP code and in major cities across the country, from the Weather Service as
// the local pages are, and the maps they're shown on. The cities (every one of 25,000 people or more) and the
// states' outlines are the Census Bureau's (public domain), built in by scripts/build-weather-maps.py.

//go:embed uscities.txt.gz
var citiesData []byte

//go:embed usstates.txt.gz
var statesData []byte

const (
	areaEvery      = 30 * time.Minute // the cities' weather is read again after this
	regionalCities = 8
	// The regional map's frame: this far (miles) above and below the ZIP code, 16:9 across.
	regionalMiles = 200.0
	earthMiles    = 3958.8
	// MapAspect is the maps' width over their height.
	MapAspect = 16.0 / 9.0
)

// The national pages' cities, spread across the country.
var nationalCityNames = []string{
	"Seattle|WA", "San Francisco|CA", "Los Angeles|CA", "Phoenix|AZ", "Salt Lake City|UT", "Denver|CO",
	"Minneapolis|MN", "Dallas|TX", "Houston|TX", "Chicago|IL", "St. Louis|MO", "Atlanta|GA", "Miami|FL",
	"Washington|DC", "New York|NY", "Boston|MA",
}

type usCity struct {
	name, state string
	lat, lon    float64
	pop         int
}

var usCities = sync.OnceValue(func() []usCity {
	var out []usCity
	r, err := gzip.NewReader(bytes.NewReader(citiesData))
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 5 || strings.HasPrefix(f[0], "#") {
			continue
		}
		lat, err1 := strconv.ParseFloat(f[2], 64)
		lon, err2 := strconv.ParseFloat(f[3], 64)
		pop, err3 := strconv.Atoi(f[4])
		if err1 == nil && err2 == nil && err3 == nil {
			out = append(out, usCity{f[0], f[1], lat, lon, pop})
		}
	}
	return out // largest first
})

// usStates are the states' outlines: each state's rings of [lon, lat].
var usStates = sync.OnceValue(func() map[string][][][2]float64 {
	out := map[string][][][2]float64{}
	r, err := gzip.NewReader(bytes.NewReader(statesData))
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		st, rings, ok := strings.Cut(sc.Text(), "\t")
		if !ok || strings.HasPrefix(st, "#") {
			continue
		}
		for _, ring := range strings.Split(rings, ";") {
			var pts [][2]float64
			for _, p := range strings.Fields(ring) {
				lon, lat, _ := strings.Cut(p, ",")
				x, err1 := strconv.ParseFloat(lon, 64)
				y, err2 := strconv.ParseFloat(lat, 64)
				if err1 == nil && err2 == nil {
					pts = append(pts, [2]float64{x, y})
				}
			}
			if len(pts) >= 3 {
				out[st] = append(out[st], pts)
			}
		}
	}
	return out
})

// lower48 says whether a state is drawn on the national map (not Alaska, Hawaii or Puerto Rico).
func lower48(st string) bool { return st != "AK" && st != "HI" && st != "PR" }

// albers is an Albers equal-area conic projection (on a unit sphere), as US maps are drawn.
type albers struct{ n, c, rho0, lon0 float64 }

func newAlbers(lat0, lon0 float64) albers {
	rad := math.Pi / 180
	p1, p2 := 29.5*rad, 45.5*rad // the US's standard parallels
	n := (math.Sin(p1) + math.Sin(p2)) / 2
	c := math.Cos(p1)*math.Cos(p1) + 2*n*math.Sin(p1)
	return albers{n: n, c: c, rho0: math.Sqrt(c-2*n*math.Sin(lat0*rad)) / n, lon0: lon0}
}

// project is a point's place, x east and y north.
func (a albers) project(lat, lon float64) (x, y float64) {
	rad := math.Pi / 180
	d := lon - a.lon0
	if d > 180 { // the Aleutians past the 180th meridian
		d -= 360
	}
	rho := math.Sqrt(math.Max(a.c-2*a.n*math.Sin(lat*rad), 0)) / a.n
	theta := a.n * d * rad
	return rho * math.Sin(theta), a.rho0 - rho*math.Cos(theta)
}

// mapFrame is a map's projection and the part of it the image shows (MapAspect across).
type mapFrame struct {
	proj             albers
	minX, maxY, w, h float64
}

// at is a point's place on the map as fractions of its width and height (from the top left).
func (f mapFrame) at(lat, lon float64) (float64, float64) {
	x, y := f.proj.project(lat, lon)
	return (x - f.minX) / f.w, (f.maxY - y) / f.h
}

// fit frames the box minX..maxX, minY..maxY with a margin, widened or heightened to MapAspect.
func fitFrame(proj albers, minX, minY, maxX, maxY, margin float64) mapFrame {
	w, h := (maxX-minX)*(1+2*margin), (maxY-minY)*(1+2*margin)
	if w/h < MapAspect {
		w = h * MapAspect
	} else {
		h = w / MapAspect
	}
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	return mapFrame{proj: proj, minX: cx - w/2, maxY: cy + h/2, w: w, h: h}
}

// nationalFrame shows the lower 48 states.
var nationalFrame = sync.OnceValue(func() mapFrame {
	proj := newAlbers(37.5, -96)
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for st, rings := range usStates() {
		if !lower48(st) {
			continue
		}
		for _, ring := range rings {
			for _, p := range ring {
				x, y := proj.project(p[1], p[0])
				minX, minY, maxX, maxY = math.Min(minX, x), math.Min(minY, y), math.Max(maxX, x), math.Max(maxY, y)
			}
		}
	}
	return fitFrame(proj, minX, minY, maxX, maxY, 0.02)
})

// regionalFrame shows regionalMiles above and below the ZIP code's point.
func regionalFrame(lat, lon float64) mapFrame {
	proj := newAlbers(lat, lon)
	half := regionalMiles / earthMiles
	return fitFrame(proj, -half*MapAspect, -half, half*MapAspect, half, 0)
}

// regionalChoice is the home city (the largest within 25 miles of the ZIP code, at lat, lon), then the
// largest cities in the frame, none crowding another's label: up to regionalCities.
func regionalChoice(f mapFrame, lat, lon float64) []usCity {
	var out []usCity
	var places [][2]float64
	cities := usCities()
	if i := slices.IndexFunc(cities, func(c usCity) bool { return miles(lat, lon, c.lat, c.lon) <= 25 }); i >= 0 {
		cities = append([]usCity{cities[i]}, slices.Delete(slices.Clone(cities), i, i+1)...)
	}
	for _, c := range cities {
		x, y := f.at(c.lat, c.lon)
		if x < 0.08 || x > 0.92 || y < 0.1 || y > 0.9 {
			continue
		}
		if slices.ContainsFunc(places, func(p [2]float64) bool { return crowds(p[0]-x, p[1]-y) }) {
			continue
		}
		out = append(out, c)
		places = append(places, [2]float64{x, y})
		if len(out) == regionalCities {
			break
		}
	}
	return out
}

// crowds says whether two cities this far apart on the map (fractions of its width and height) would have
// their labels (an icon and temperature over the name) overlap: within an ellipse, so one diagonally off
// (Philadelphia from New York) still fits.
func crowds(dx, dy float64) bool { return (dx/0.13)*(dx/0.13)+(dy/0.12)*(dy/0.12) < 1 }

// nationalOnMap marks the cities shown on the national map: the largest first, then each that doesn't crowd
// one already there (the Travel Forecast lists them all).
func nationalOnMap(cities []WeatherCity) {
	pop := func(c WeatherCity) int {
		if i := slices.IndexFunc(usCities(), func(u usCity) bool { return u.name == c.Name && u.state == c.State }); i >= 0 {
			return usCities()[i].pop
		}
		return 0
	}
	order := make([]int, len(cities))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return pop(cities[b]) - pop(cities[a]) })
	var kept [][2]float64
	for _, i := range order {
		c := &cities[i]
		if slices.ContainsFunc(kept, func(p [2]float64) bool { return crowds(p[0]-c.X, p[1]-c.Y) }) {
			continue
		}
		c.OnMap = true
		kept = append(kept, [2]float64{c.X, c.Y})
	}
}

// miles is the distance between two points along the Earth.
func miles(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	a := math.Sin((lat2-lat1)*rad/2)*math.Sin((lat2-lat1)*rad/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin((lon2-lon1)*rad/2)*math.Sin((lon2-lon1)*rad/2)
	return 2 * earthMiles * math.Asin(math.Sqrt(a))
}

func nationalChoice() []usCity {
	var out []usCity
	for _, name := range nationalCityNames {
		n, st, _ := strings.Cut(name, "|")
		if i := slices.IndexFunc(usCities(), func(c usCity) bool { return c.name == n && c.state == st }); i >= 0 {
			out = append(out, usCities()[i])
		}
	}
	return out
}

// WeatherArea is the regional or national pages' cities, and their map (GET Map, MapAspect across).
type WeatherArea struct {
	Map    string        `json:"map"` // its address, with region= and the version
	Cities []WeatherCity `json:"cities"`
}

// WeatherCity is a city's weather: now (the regional pages), and today's forecast. Missing readings are left
// out.
type WeatherCity struct {
	Name       string  `json:"name"`
	State      string  `json:"state"`
	X          float64 `json:"x"` // on the area's map, fractions of its width and height from the top left
	Y          float64 `json:"y"`
	OnMap      bool    `json:"on_map"` // shown on the map (on the national one, not where a bigger city's label is)
	Temp       *int    `json:"temp,omitempty"` // now, °F
	Conditions string  `json:"conditions,omitempty"`
	Icon       string  `json:"icon,omitempty"` // now's (WeatherIcons)
	WindDir    string  `json:"wind_dir,omitempty"`
	Wind       *int    `json:"wind,omitempty"` // mph
	High       *int    `json:"high,omitempty"` // today's, or (after the day's forecast) tomorrow's
	Low        *int    `json:"low,omitempty"`  // tonight's
	Forecast   string  `json:"forecast,omitempty"`
	DayIcon    string  `json:"day_icon,omitempty"` // the forecast's
}

// weatherAreas are the regional and national pages' weather, read in the background (Get never waits for
// them).
type weatherAreas struct {
	mu       sync.Mutex
	zip      string
	regional *WeatherArea
	national *WeatherArea
	read     time.Time
	busy     bool
	points   map[string]*nwsPoint // the cities' grid points, by "lat,lon"
	maps     map[string][]byte    // drawn maps, by region, ZIP code and width
}

// areas are the regional and national pages for zip as last read, starting a new reading when they're old.
func (w *WeatherService) areas(zip string) (*WeatherArea, *WeatherArea) {
	a := &w.area
	a.mu.Lock()
	defer a.mu.Unlock()
	if (a.zip != zip || time.Since(a.read) >= areaEvery) && !a.busy {
		a.busy = true
		go w.readAreas(zip)
	}
	if a.zip != zip {
		return nil, a.national
	}
	return a.regional, a.national
}

func (w *WeatherService) readAreas(zip string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var regional *WeatherArea
	if lat, lon, ok := ZipPoint(zip); ok {
		f := regionalFrame(lat, lon)
		regional = &WeatherArea{Map: "/api/guide/weather/map?region=regional&v=" + zip,
			Cities: w.cityWeather(ctx, f, regionalChoice(f, lat, lon), true)}
	}
	national := &WeatherArea{Map: "/api/guide/weather/map?region=national",
		Cities: w.cityWeather(ctx, nationalFrame(), nationalChoice(), false)}
	if regional != nil {
		for i := range regional.Cities {
			regional.Cities[i].OnMap = true // chosen not to crowd
		}
	}
	nationalOnMap(national.Cities)
	a := &w.area
	a.mu.Lock()
	defer a.mu.Unlock()
	a.busy = false
	a.read = time.Now()
	if len(national.Cities) == 0 && a.national != nil { // nothing answered: try again in a few minutes
		a.read = a.read.Add(-areaEvery + 5*time.Minute)
		national = a.national
	}
	if regional == nil || len(regional.Cities) == 0 {
		if a.zip == zip && a.regional != nil {
			regional = a.regional
		}
	}
	a.zip, a.regional, a.national = zip, regional, national
	log.Printf("Weather: %d regional and %d national cities read\n", areaCount(regional), len(national.Cities))
}

func areaCount(a *WeatherArea) int {
	if a == nil {
		return 0
	}
	return len(a.Cities)
}

// cityWeather reads each city's forecast (and with now, its latest observation), four at a time; a city the
// Weather Service doesn't answer for is left out.
func (w *WeatherService) cityWeather(ctx context.Context, f mapFrame, cities []usCity, now bool) []WeatherCity {
	out := make([]*WeatherCity, len(cities))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, c := range cities {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			p, err := w.cityPoint(ctx, c)
			if err != nil {
				return
			}
			wc := &WeatherCity{Name: c.name, State: c.state}
			wc.X, wc.Y = f.at(c.lat, c.lon)
			if !w.cityForecast(ctx, p, wc) {
				return
			}
			if now {
				if o := w.observation(ctx, p); o != nil {
					wc.Temp, wc.Conditions, wc.Icon, wc.WindDir, wc.Wind = o.Temp, o.Conditions, o.Icon, o.WindDir, o.Wind
				}
			}
			out[i] = wc
		}()
	}
	wg.Wait()
	var list []WeatherCity
	for _, c := range out {
		if c != nil {
			list = append(list, *c)
		}
	}
	return list
}

// cityPoint is a city's grid point (its forecast and stations), kept once read.
func (w *WeatherService) cityPoint(ctx context.Context, c usCity) (*nwsPoint, error) {
	key := fmt.Sprintf("%.4f,%.4f", c.lat, c.lon)
	a := &w.area
	a.mu.Lock()
	p := a.points[key]
	a.mu.Unlock()
	if p != nil {
		return p, nil
	}
	var r struct {
		Properties struct {
			Forecast            string `json:"forecast"`
			ObservationStations string `json:"observationStations"`
		} `json:"properties"`
	}
	if err := w.getJSON(ctx, fmt.Sprintf("%s/points/%s", nwsAPI, key), &r); err != nil {
		return nil, err
	}
	if r.Properties.Forecast == "" {
		return nil, fmt.Errorf("no forecast for %s", c.name)
	}
	p = &nwsPoint{forecast: r.Properties.Forecast, stations: r.Properties.ObservationStations, lat: c.lat, lon: c.lon}
	a.mu.Lock()
	if a.points == nil {
		a.points = map[string]*nwsPoint{}
	}
	a.points[key] = p
	a.mu.Unlock()
	return p, nil
}

// cityForecast fills in today's high and tonight's low (after the day's forecast, tonight's low and tomorrow's
// high), and the daytime's forecast and icon.
func (w *WeatherService) cityForecast(ctx context.Context, p *nwsPoint, c *WeatherCity) bool {
	var fc struct {
		Properties struct {
			Periods []struct {
				IsDaytime     bool   `json:"isDaytime"`
				Temperature   int    `json:"temperature"`
				ShortForecast string `json:"shortForecast"`
			} `json:"periods"`
		} `json:"properties"`
	}
	if err := w.getJSON(ctx, p.forecast, &fc); err != nil || len(fc.Properties.Periods) == 0 {
		return false
	}
	for i, per := range fc.Properties.Periods {
		if i > 2 || (c.High != nil && c.Low != nil) {
			break
		}
		t := per.Temperature
		if per.IsDaytime && c.High == nil {
			c.High, c.Forecast, c.DayIcon = &t, per.ShortForecast, conditionIcon(per.ShortForecast, true)
		} else if !per.IsDaytime && c.Low == nil {
			c.Low = &t
			if c.Forecast == "" {
				c.Forecast, c.DayIcon = per.ShortForecast, conditionIcon(per.ShortForecast, false)
			}
		}
	}
	return true
}

// Map draws the regional (around the ZIP code) or national map width pixels wide (MapAspect across): the
// states' land, their borders, and nothing around them (the page's background is the water).
func (w *WeatherService) Map(region string, width int) ([]byte, error) {
	width = min(max(width, 320), 1920)
	height := int(math.Round(float64(width) / MapAspect))
	var f mapFrame
	key := fmt.Sprintf("%s|%d", region, width)
	switch region {
	case "national":
		f = nationalFrame()
	case "regional":
		zip := WeatherZip()
		lat, lon, ok := ZipPoint(zip)
		if !ok {
			return nil, fmt.Errorf("%s isn't a US ZIP code", zip)
		}
		f = regionalFrame(lat, lon)
		key += "|" + zip
	default:
		return nil, fmt.Errorf("no such map")
	}
	a := &w.area
	a.mu.Lock()
	b := a.maps[key]
	a.mu.Unlock()
	if b != nil {
		return b, nil
	}
	land, borders := vector.NewRasterizer(width, height), vector.NewRasterizer(width, height)
	line := math.Max(float64(width)/900, 1) // the borders' width, pixels
	pix := func(p [2]float64) pt {
		x, y := f.at(p[1], p[0])
		return pt{x * float64(width), y * float64(height)}
	}
	for st, rings := range usStates() {
		if region == "national" && !lower48(st) {
			continue
		}
		for _, ring := range rings {
			var shape path
			for i, p := range ring {
				if i == 0 {
					shape.moveTo(pix(p))
				} else {
					shape.lineTo(pix(p))
				}
			}
			shape.close()
			addPath(land, shape, 1)
			// each edge as a thin quad, all the same way round so where they overlap they add up
			for i := range ring {
				a, b := pix(ring[i]), pix(ring[(i+1)%len(ring)])
				dx, dy := b.x-a.x, b.y-a.y
				l := math.Hypot(dx, dy)
				if l == 0 {
					continue
				}
				nx, ny := -dy/l*line/2, dx/l*line/2
				addPath(borders, polygon(pt{a.x + nx, a.y + ny}, pt{b.x + nx, b.y + ny}, pt{b.x - nx, b.y - ny}, pt{a.x - nx, a.y - ny}), 1)
			}
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	land.Draw(dst, dst.Bounds(), image.NewUniform(color.NRGBA{0x4E, 0x6E, 0x3A, 0xFF}), image.Point{})
	borders.Draw(dst, dst.Bounds(), image.NewUniform(color.NRGBA{0xC8, 0xD8, 0xA8, 0xFF}), image.Point{})
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.maps == nil {
		a.maps = map[string][]byte{}
	}
	a.maps[key] = buf.Bytes()
	a.mu.Unlock()
	return buf.Bytes(), nil
}
