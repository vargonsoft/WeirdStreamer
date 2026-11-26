// Command logoart draws the Guide's channel logos (internal/guide/logodesign.go) with the display
// fonts in a folder and saves their artwork in internal/guide/logoart, which the server embeds. The
// fonts are Google Fonts' families under the SIL Open Font License (their .ttf files, by the names in
// logodesign.go, from github.com/google/fonts); they're only needed here, not by the server.
//
// Usage: go run ./cmd/logoart -fonts <folder> [-out internal/guide/logoart] [-preview <folder>]
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/weirdstreamer/server/internal/guide"
)

func main() {
	fonts := flag.String("fonts", "", "the folder of the display fonts' .ttf files")
	out := flag.String("out", "internal/guide/logoart", "where the artwork goes")
	preview := flag.String("preview", "", "a folder for PNG previews (numbered as channel 7)")
	flag.Parse()
	if *fonts == "" {
		flag.Usage()
		os.Exit(2)
	}
	arts, err := guide.Design(*fonts)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(*out, "*.json"))
	for _, f := range old {
		os.Remove(f)
	}
	for _, a := range arts {
		b, err := json.Marshal(a)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, a.ID+".json"), append(b, '\n'), 0o644); err != nil {
			log.Fatal(err)
		}
		if *preview != "" {
			png, err := a.PNG(7, 1024, guide.LogoColor("ffffff"))
			if err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(*preview, a.ID+".png"), png, 0o644); err != nil {
				log.Fatal(err)
			}
		}
	}
	log.Printf("%d logos", len(arts))
}
