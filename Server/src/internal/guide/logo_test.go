package guide

import (
	"bytes"
	"encoding/xml"
	"image/png"
	"testing"
)

// Every logo's artwork loads and draws, with its number, as a PNG of the size asked and as SVG that
// parses; every genre channel has one, and no two logos share a font.
func TestLogos(t *testing.T) {
	lib := artwork()
	if lib.err != nil {
		t.Fatal(lib.err)
	}
	fonts := map[string]string{}
	all := append([]*Artwork(nil), lib.stations...)
	for _, b := range append(append([]bucket(nil), showBuckets...), movieBuckets...) {
		a := lib.genres[b.label]
		if a == nil {
			t.Errorf("no logo for %s", b.label)
			continue
		}
		all = append(all, a)
	}
	for _, name := range []string{"Variety", "Movies"} {
		all = append(all, lib.genres[name])
	}
	for _, a := range all {
		if other, ok := fonts[a.Font]; ok && other != a.ID {
			t.Errorf("%s and %s share %s", a.ID, other, a.Font)
		}
		fonts[a.Font] = a.ID
		b, err := a.PNG(12, 256, white)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != 256 || img.Bounds().Dy() != 128 {
			t.Fatalf("%s: %v", a.ID, err)
		}
		svg, err := a.SVG(12, white)
		if err != nil {
			t.Fatal(err)
		}
		if err := xml.Unmarshal(svg, new(struct{})); err != nil {
			t.Fatalf("%s: SVG: %v", a.ID, err)
		}
	}
	if len(lib.stations) < 20 {
		t.Errorf("%d stations in the pool", len(lib.stations))
	}
	if LogoFor("WKBT", "Comedy", true).Name != "WKBT" || LogoFor("Kids", "Kids", false).Name != "Kids" {
		t.Error("LogoFor picked the wrong logo")
	}
}
