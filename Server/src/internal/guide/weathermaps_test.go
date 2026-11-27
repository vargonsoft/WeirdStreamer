package guide

import (
	"bytes"
	"image/png"
	"testing"
)

func TestWeatherCities(t *testing.T) {
	if n := len(nationalChoice()); n != len(nationalCityNames) {
		t.Errorf("national: %d of %d cities found", n, len(nationalCityNames))
	}
	lat, lon, _ := ZipPoint("10001")
	f := regionalFrame(lat, lon)
	cities := regionalChoice(f, lat, lon)
	if len(cities) < 5 || cities[0].name != "New York" || cities[1].name != "Philadelphia" {
		t.Fatalf("regional around 10001: %v", cities)
	}
	// the home city first, though a bigger one is near
	lat2, lon2, _ := ZipPoint("94103")
	if c := regionalChoice(regionalFrame(lat2, lon2), lat2, lon2); len(c) == 0 || c[0].name != "San Francisco" {
		t.Errorf("regional around 94103: %v", c)
	}
	for i, a := range cities {
		ax, ay := f.at(a.lat, a.lon)
		if ax < 0 || ax > 1 || ay < 0 || ay > 1 {
			t.Errorf("%s off the map: %.2f, %.2f", a.name, ax, ay)
		}
		for _, b := range cities[:i] {
			bx, by := f.at(b.lat, b.lon)
			if crowds(ax-bx, ay-by) {
				t.Errorf("%s crowds %s", a.name, b.name)
			}
		}
	}
	// the national map: the coasts near its edges, Kansas near the middle
	n := nationalFrame()
	if x, _ := n.at(39.0, -98.0); x < 0.4 || x > 0.6 {
		t.Errorf("Kansas at x %.2f", x)
	}
	if x, _ := n.at(47.6, -122.3); x > 0.2 {
		t.Errorf("Seattle at x %.2f", x)
	}
}

func TestNationalOnMap(t *testing.T) {
	f := nationalFrame()
	var cities []WeatherCity
	for _, c := range nationalChoice() {
		wc := WeatherCity{Name: c.name, State: c.state}
		wc.X, wc.Y = f.at(c.lat, c.lon)
		cities = append(cities, wc)
	}
	nationalOnMap(cities)
	on := map[string]bool{}
	for _, c := range cities {
		on[c.Name] = c.OnMap
	}
	// New York's label is there; Boston's, which would sit on it, isn't; Miami, alone in the corner, is
	if !on["New York"] || on["Boston"] || !on["Miami"] || !on["Los Angeles"] {
		t.Errorf("on the map: %v", on)
	}
}

func TestWeatherMap(t *testing.T) {
	w := NewWeatherService()
	for _, region := range []string{"national", "regional"} {
		b, err := w.Map(region, 640)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if s := img.Bounds().Size(); s.X != 640 || s.Y != 360 {
			t.Errorf("%s: %v", region, s)
		}
	}
	if _, err := w.Map("moon", 640); err == nil {
		t.Error("no such map")
	}
}
