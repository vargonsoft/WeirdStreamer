package guide

import (
	"strconv"
	"time"
)

// The channels of their own (docs/GUIDE.md): the Guide channel and the Weather channel, on the
// home screen's Guide, drawn by the apps. They're stored as channels with no library.

// The special channels' keys and kinds
const (
	GuideChannelKey   = "special:guide"
	WeatherChannelKey = "special:weather"
	KindGuideChannel  = "guide"
	KindWeather       = "weather"
	GuideChannelNum   = 1 // below the network channels (from 2)
	// KindChannel is the ProgramInfo kind of a channel of its own's half hours in the grid.
	KindChannel = "channel"
)

// SpecialChannel is a channel of its own: its key, kind and name (its genre, for its logo).
type SpecialChannel struct {
	Key, Kind, Name string
	Title, Overview string // what the grid shows for its half hours
}

// SpecialChannels are the channels of their own, in the order they're made.
var SpecialChannels = []SpecialChannel{
	{Key: GuideChannelKey, Kind: KindGuideChannel, Name: "Guide", Title: "Program Guide",
		Overview: "What's on every channel, around the clock, with previews of what's coming up."},
	{Key: WeatherChannelKey, Kind: KindWeather, Name: "Weather", Title: "Local Forecast",
		Overview: "Current conditions, the forecast and the radar, with smooth jazz."},
}

// SpecialByKey is a channel of its own by its key.
func SpecialByKey(key string) (SpecialChannel, bool) {
	for _, s := range SpecialChannels {
		if s.Key == key {
			return s, true
		}
	}
	return SpecialChannel{}, false
}

// The Guide channel's top half: 30-second blocks by the clock.
const (
	GuideBlock      = 30 * time.Second
	previewAhead    = 6 * time.Hour   // previews are of programs starting within this
	previewNotSoon  = 2 * time.Minute // and not about to start
	maxFillerInSlot = GuideBlock + 3*time.Second
)

// The Guide channel's blocks
const (
	BlockPreview = "preview" // a program's middle, with when and where it airs
	BlockPromo   = "promo"   // a card for a program, with music
	BlockFiller  = "filler"  // a commercial
)

// Airing is a program that will air: on which channel, when, and its file.
type Airing struct {
	Channel int
	Start   time.Time
	Length  time.Duration
	Path    string
	Info    *ProgramInfo
}

// GuideChannelBlock is what the Guide channel's top half shows at now: by the block's place in a
// cycle of four (a preview, the promo card for it, another preview, a commercial), each picked by
// the block's number from upcoming (the programs ahead on the viewer's channels) and clips (the
// commercials they may see). Offset is how far into the file the block starts.
type GuideChannelSlot struct {
	Kind   string
	Start  time.Time
	End    time.Time
	Path   string
	Offset time.Duration
	Airing *Airing
}

func GuideChannelBlock(now time.Time, upcoming []Airing, clips []Clip) GuideChannelSlot {
	start := now.Truncate(GuideBlock)
	b := start.Unix() / int64(GuideBlock/time.Second)
	out := GuideChannelSlot{Start: start, End: start.Add(GuideBlock)}
	var ahead []Airing
	for _, a := range upcoming {
		if a.Info != nil && a.Start.After(now.Add(previewNotSoon)) && a.Start.Before(now.Add(previewAhead)) &&
			a.Length > 2*GuideBlock && a.Path != "" {
			ahead = append(ahead, a)
		}
	}
	pick := func(n int64) *Airing {
		if len(ahead) == 0 {
			return nil
		}
		a := ahead[hash("airing"+strconv.FormatInt(n, 10))%uint64(len(ahead))]
		return &a
	}
	switch b % 4 {
	case 0, 2:
		if a := pick(b); a != nil {
			out.Kind, out.Airing, out.Path = BlockPreview, a, a.Path
			// from 30% to 60% of the way in: past the opening, before the end
			frac := 0.3 + 0.3*float64(hash("offset"+strconv.FormatInt(b, 10))%1000)/1000
			out.Offset = time.Duration(float64(a.Length-GuideBlock) * frac).Truncate(time.Second)
			return out
		}
	case 1:
		if a := pick(b - 1); a != nil { // the card for the program just previewed
			out.Kind, out.Airing = BlockPromo, a
			return out
		}
	case 3:
		var fit []Clip
		for _, c := range clips {
			if c.Duration <= maxFillerInSlot {
				fit = append(fit, c)
			}
		}
		if len(fit) > 0 {
			c := fit[hash("clip"+strconv.FormatInt(b, 10))%uint64(len(fit))]
			out.Kind, out.Path = BlockFiller, c.Path
			return out
		}
	}
	// nothing to preview or air: a card (with music), for what's ahead if anything
	out.Kind, out.Airing = BlockPromo, pick(b)
	return out
}
