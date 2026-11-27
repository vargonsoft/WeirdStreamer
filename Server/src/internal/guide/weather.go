package guide

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// The Weather channel's data (docs/GUIDE.md): the US National Weather Service's current conditions,
// forecast, alerts and radar for the server's ZIP code (guide.weatherZip), asked for at most every
// 10 minutes. The ZIP code's position is the Census Bureau's (its 2024 ZIP code tabulation areas,
// public domain), built in.

//go:embed zipcodes.txt.gz
var zipData []byte

// DefaultWeatherZip is New York City's (Midtown Manhattan).
const DefaultWeatherZip = "10001"

const (
	weatherEvery = 10 * time.Minute
	radarEvery   = 5 * time.Minute
	nwsAPI       = "https://api.weather.gov"
	// The Weather Service asks for a User-Agent naming the program; a generic one, no contact.
	nwsAgent = "WeirdStreamer (self-hosted media server)"
)

var zipPoints = sync.OnceValue(func() map[string][2]float64 {
	out := map[string][2]float64{}
	r, err := gzip.NewReader(bytes.NewReader(zipData))
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 || strings.HasPrefix(f[0], "#") {
			continue
		}
		lat, err1 := strconv.ParseFloat(f[1], 64)
		lon, err2 := strconv.ParseFloat(f[2], 64)
		if err1 == nil && err2 == nil {
			out[f[0]] = [2]float64{lat, lon}
		}
	}
	return out
})

// ZipPoint is a US ZIP code's position (its tabulation area's inner point).
func ZipPoint(zip string) (lat, lon float64, ok bool) {
	p, ok := zipPoints()[strings.TrimSpace(zip)]
	return p[0], p[1], ok
}

// WeatherZip is the ZIP code the Weather channel is for: the config's, else New York City's.
func WeatherZip() string {
	if c := config.GlobalConfig; c != nil && strings.TrimSpace(c.Guide.WeatherZip) != "" {
		return strings.TrimSpace(c.Guide.WeatherZip)
	}
	return DefaultWeatherZip
}

// Weather is what the Weather channel shows: GET /api/guide/weather.
type Weather struct {
	Place   string          `json:"place"` // "New York, NY"
	Zip     string          `json:"zip"`
	Updated int64           `json:"updated"` // when it was read, Unix ms
	Current *WeatherNow     `json:"current,omitempty"`
	Periods []WeatherPeriod `json:"periods"`
	Alerts  []WeatherAlert  `json:"alerts"`
	Radar   bool            `json:"radar"` // GET /api/guide/weather/radar has the area's radar image
	// The regional and national pages (weathermaps.go): nil until they've been read (in the background)
	Regional *WeatherArea `json:"regional,omitempty"`
	National *WeatherArea `json:"national,omitempty"`
	Error    string       `json:"error,omitempty"` // why there's nothing (an unknown ZIP code, no answer)
}

// WeatherNow is the latest observation, in US units. Missing readings are left out.
type WeatherNow struct {
	Time       int64    `json:"time"`
	Station    string   `json:"station"`
	Conditions string   `json:"conditions"`
	Icon       string   `json:"icon"`                 // WeatherIcons' names
	Temp       *int     `json:"temp,omitempty"`       // °F
	FeelsLike  *int     `json:"feels_like,omitempty"` // wind chill or heat index, °F
	Dewpoint   *int     `json:"dewpoint,omitempty"`
	Humidity   *int     `json:"humidity,omitempty"` // %
	WindDir    string   `json:"wind_dir,omitempty"` // "NE"
	Wind       *int     `json:"wind,omitempty"`     // mph
	Gust       *int     `json:"gust,omitempty"`
	Pressure   *float64 `json:"pressure,omitempty"`   // inches of mercury
	Visibility *float64 `json:"visibility,omitempty"` // miles
}

// WeatherPeriod is a forecast's half day ("Tonight", "Friday").
type WeatherPeriod struct {
	Name     string `json:"name"`
	Daytime  bool   `json:"daytime"`
	Temp     int    `json:"temp"` // °F: the day's high, the night's low
	Forecast string `json:"forecast"`
	Detail   string `json:"detail"`
	Icon     string `json:"icon"`
	Wind     string `json:"wind"`             // "NE 20 to 29 mph"
	Precip   *int   `json:"precip,omitempty"` // chance, %
}

// WeatherAlert is a watch, warning or advisory in force.
type WeatherAlert struct {
	Event    string `json:"event"` // "High Wind Warning"
	Headline string `json:"headline"`
	Severity string `json:"severity"`
	Ends     int64  `json:"ends,omitempty"`
}

// WeatherIcons are the condition icons the server draws (WeatherIconPNG).
var WeatherIcons = []string{"sun", "moon", "cloudsun", "cloudmoon", "cloud", "rain", "snow", "storm", "fog", "wind"}

// conditionIcon is the icon for a forecast's or observation's words.
func conditionIcon(text string, day bool) string {
	t := strings.ToLower(text)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(t, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("thunder", "t-storm", "tstorm"):
		return "storm"
	case has("snow", "flurr", "blizzard", "sleet", "ice pellets", "freezing"):
		return "snow"
	case has("rain", "shower", "drizzle", "precip"):
		return "rain"
	case has("fog", "haze", "smoke", "mist", "dust"):
		return "fog"
	case has("partly", "mostly sunny", "mostly clear", "few clouds", "partly sunny"):
		if day {
			return "cloudsun"
		}
		return "cloudmoon"
	case has("cloud", "overcast"):
		return "cloud"
	case has("wind", "breezy", "blustery"):
		return "wind"
	case has("sunny", "clear", "fair", "hot"):
		if day {
			return "sun"
		}
		return "moon"
	}
	return "cloud"
}

// WeatherService reads the weather and keeps it for a while.
type WeatherService struct {
	mu      sync.Mutex
	data    *Weather
	read    time.Time
	point   *nwsPoint // the ZIP code's grid point, kept while the ZIP code stays
	radar   []byte
	radarAt time.Time
	client  *http.Client
	area    weatherAreas // the regional and national pages
}

type nwsPoint struct {
	zip, forecast, stations, radar, place string
	lat, lon                              float64
}

// NewWeatherService makes the Weather channel's reader.
func NewWeatherService() *WeatherService {
	return &WeatherService{client: &http.Client{Timeout: 20 * time.Second}}
}

// Get is the weather, read again when it's 10 minutes old (one reading at a time; a failed one keeps
// what was read before), with the regional and national pages as last read (they're read in the
// background every 30 minutes).
func (w *WeatherService) Get(ctx context.Context) *Weather {
	local := w.local(ctx)
	out := *local
	out.Regional, out.National = w.areas(out.Zip)
	return &out
}

func (w *WeatherService) local(ctx context.Context) *Weather {
	w.mu.Lock()
	defer w.mu.Unlock()
	zip := WeatherZip()
	if w.data != nil && w.data.Zip == zip && time.Since(w.read) < weatherEvery {
		return w.data
	}
	data, err := w.fetch(ctx, zip)
	if err != nil {
		if w.data != nil && w.data.Zip == zip {
			w.read = time.Now().Add(-weatherEvery + time.Minute) // try again in a minute
			return w.data
		}
		return &Weather{Zip: zip, Periods: []WeatherPeriod{}, Alerts: []WeatherAlert{}, Error: err.Error()}
	}
	w.data, w.read = data, time.Now()
	return data
}

// Radar is the area's latest radar image (a GIF), kept 5 minutes.
func (w *WeatherService) Radar(ctx context.Context) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.radar != nil && time.Since(w.radarAt) < radarEvery && w.point != nil && w.point.zip == WeatherZip() {
		return w.radar, nil
	}
	p, err := w.pointFor(ctx, WeatherZip())
	if err != nil {
		return nil, err
	}
	if p.radar == "" {
		return nil, errors.New("no radar for this area")
	}
	b, err := w.getBytes(ctx, "https://radar.weather.gov/ridge/standard/"+p.radar+"_0.gif")
	if err != nil {
		if w.radar != nil {
			return w.radar, nil
		}
		return nil, err
	}
	w.radar, w.radarAt = b, time.Now()
	return b, nil
}

func (w *WeatherService) getBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", nwsAgent)
	req.Header.Set("Accept", "application/geo+json, image/gif, */*")
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the Weather Service didn't answer")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the Weather Service answered %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func (w *WeatherService) getJSON(ctx context.Context, url string, v any) error {
	b, err := w.getBytes(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// pointFor is a ZIP code's grid point: where its forecast, stations and radar are.
func (w *WeatherService) pointFor(ctx context.Context, zip string) (*nwsPoint, error) {
	if w.point != nil && w.point.zip == zip {
		return w.point, nil
	}
	lat, lon, ok := ZipPoint(zip)
	if !ok {
		return nil, fmt.Errorf("%s isn't a US ZIP code", zip)
	}
	var r struct {
		Properties struct {
			Forecast            string `json:"forecast"`
			ObservationStations string `json:"observationStations"`
			RadarStation        string `json:"radarStation"`
			RelativeLocation    struct {
				Properties struct {
					City  string `json:"city"`
					State string `json:"state"`
				} `json:"properties"`
			} `json:"relativeLocation"`
		} `json:"properties"`
	}
	if err := w.getJSON(ctx, fmt.Sprintf("%s/points/%.4f,%.4f", nwsAPI, lat, lon), &r); err != nil {
		return nil, err
	}
	p := r.Properties
	place := p.RelativeLocation.Properties.City
	if s := p.RelativeLocation.Properties.State; s != "" {
		place += ", " + s
	}
	w.point = &nwsPoint{zip: zip, forecast: p.Forecast, stations: p.ObservationStations, radar: p.RadarStation,
		place: place, lat: lat, lon: lon}
	return w.point, nil
}

func (w *WeatherService) fetch(ctx context.Context, zip string) (*Weather, error) {
	p, err := w.pointFor(ctx, zip)
	if err != nil {
		return nil, err
	}
	out := &Weather{Place: p.place, Zip: zip, Updated: time.Now().UnixMilli(), Radar: p.radar != "",
		Periods: []WeatherPeriod{}, Alerts: []WeatherAlert{}}

	var fc struct {
		Properties struct {
			Periods []struct {
				Name             string `json:"name"`
				IsDaytime        bool   `json:"isDaytime"`
				Temperature      int    `json:"temperature"`
				WindSpeed        string `json:"windSpeed"`
				WindDirection    string `json:"windDirection"`
				ShortForecast    string `json:"shortForecast"`
				DetailedForecast string `json:"detailedForecast"`
				Precip           struct {
					Value *float64 `json:"value"`
				} `json:"probabilityOfPrecipitation"`
			} `json:"periods"`
		} `json:"properties"`
	}
	if err := w.getJSON(ctx, p.forecast, &fc); err != nil {
		return nil, err
	}
	for _, f := range fc.Properties.Periods {
		wp := WeatherPeriod{Name: f.Name, Daytime: f.IsDaytime, Temp: f.Temperature, Forecast: f.ShortForecast,
			Detail: f.DetailedForecast, Icon: conditionIcon(f.ShortForecast, f.IsDaytime),
			Wind: strings.TrimSpace(f.WindDirection + " " + f.WindSpeed)}
		if f.Precip.Value != nil {
			v := int(math.Round(*f.Precip.Value))
			wp.Precip = &v
		}
		out.Periods = append(out.Periods, wp)
	}

	out.Current = w.observation(ctx, p)

	var al struct {
		Features []struct {
			Properties struct {
				Event    string `json:"event"`
				Headline string `json:"headline"`
				Severity string `json:"severity"`
				Ends     string `json:"ends"`
				Expires  string `json:"expires"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := w.getJSON(ctx, fmt.Sprintf("%s/alerts/active?point=%.4f,%.4f", nwsAPI, p.lat, p.lon), &al); err == nil {
		for _, f := range al.Features {
			a := WeatherAlert{Event: f.Properties.Event, Headline: f.Properties.Headline, Severity: f.Properties.Severity}
			end := f.Properties.Ends
			if end == "" {
				end = f.Properties.Expires
			}
			if t, err := time.Parse(time.RFC3339, end); err == nil {
				a.Ends = t.UnixMilli()
			}
			out.Alerts = append(out.Alerts, a)
		}
	}
	return out, nil
}

// observation is the latest reading at the nearest station that has a temperature.
func (w *WeatherService) observation(ctx context.Context, p *nwsPoint) *WeatherNow {
	var st struct {
		Features []struct {
			Properties struct {
				StationIdentifier string `json:"stationIdentifier"`
				Name              string `json:"name"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := w.getJSON(ctx, p.stations+"?limit=3", &st); err != nil {
		return nil
	}
	type reading struct {
		Value *float64 `json:"value"`
	}
	for _, f := range st.Features {
		var ob struct {
			Properties struct {
				Timestamp          string  `json:"timestamp"`
				TextDescription    string  `json:"textDescription"`
				Temperature        reading `json:"temperature"`
				Dewpoint           reading `json:"dewpoint"`
				WindDirection      reading `json:"windDirection"`
				WindSpeed          reading `json:"windSpeed"`
				WindGust           reading `json:"windGust"`
				BarometricPressure reading `json:"barometricPressure"`
				Visibility         reading `json:"visibility"`
				RelativeHumidity   reading `json:"relativeHumidity"`
				WindChill          reading `json:"windChill"`
				HeatIndex          reading `json:"heatIndex"`
			} `json:"properties"`
		}
		id := f.Properties.StationIdentifier
		if err := w.getJSON(ctx, nwsAPI+"/stations/"+id+"/observations/latest", &ob); err != nil {
			continue
		}
		o := ob.Properties
		if o.Temperature.Value == nil {
			continue
		}
		fahrenheit := func(r reading) *int {
			if r.Value == nil {
				return nil
			}
			v := int(math.Round(*r.Value*9/5 + 32))
			return &v
		}
		mph := func(r reading) *int {
			if r.Value == nil {
				return nil
			}
			v := int(math.Round(*r.Value / 1.609344))
			return &v
		}
		now := &WeatherNow{Station: f.Properties.Name, Conditions: o.TextDescription,
			Temp: fahrenheit(o.Temperature), Dewpoint: fahrenheit(o.Dewpoint), Wind: mph(o.WindSpeed), Gust: mph(o.WindGust)}
		if t, err := time.Parse(time.RFC3339, o.Timestamp); err == nil {
			now.Time = t.UnixMilli()
		}
		if o.WindChill.Value != nil {
			now.FeelsLike = fahrenheit(o.WindChill)
		} else if o.HeatIndex.Value != nil {
			now.FeelsLike = fahrenheit(o.HeatIndex)
		}
		if o.RelativeHumidity.Value != nil {
			v := int(math.Round(*o.RelativeHumidity.Value))
			now.Humidity = &v
		}
		if o.WindDirection.Value != nil {
			now.WindDir = compass(*o.WindDirection.Value)
		}
		if o.BarometricPressure.Value != nil {
			v := math.Round(*o.BarometricPressure.Value/3386.389*100) / 100
			now.Pressure = &v
		}
		if o.Visibility.Value != nil {
			v := math.Round(*o.Visibility.Value/1609.344*10) / 10
			now.Visibility = &v
		}
		hour := time.Now().Hour()
		now.Icon = conditionIcon(o.TextDescription, hour >= 6 && hour < 18)
		return now
	}
	return nil
}

// compass is a bearing's point of the compass ("NE").
func compass(deg float64) string {
	points := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	i := int(math.Round(math.Mod(deg+360, 360)/22.5)) % 16
	return points[i]
}
