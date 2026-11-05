package sources

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Weather ----------------

// PocketWeather reads a day's weather from Pocket Weather (weather:read): the
// person's own home place there, the weather diary (with their notes) and the
// warnings of the day. It fills the weather source, so the days, the
// calendar, the year page and the briefs read it exactly like Open-Meteo's.
type PocketWeather struct{ Conn pocket.Conn }

// NewPocketWeather creates a fetcher over a connection.
func NewPocketWeather(conn pocket.Conn) *PocketWeather { return &PocketWeather{Conn: conn} }

// Name implements Fetcher.
func (p *PocketWeather) Name() string { return "weather" }

// WeatherWarning is a warning Pocket Weather has out for the day.
type WeatherWarning struct {
	Level  string `json:"level"` // advisory | warning
	Kind   string `json:"kind,omitempty"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Place  string `json:"place,omitempty"`
	From   string `json:"from,omitempty"`  // HH:MM it starts, when the same day
	Until  string `json:"until,omitempty"` // HH:MM it ends, when the same day
}

// weatherDayAnswer is Pocket Weather's GET /api/v1/day/{date}.
type weatherDayAnswer struct {
	Date   string `json:"date"`
	Source string `json:"source"`
	Day    struct {
		Code      *int     `json:"code"`
		Condition string   `json:"condition"`
		TMax      *float64 `json:"t_max"`
		TMin      *float64 `json:"t_min"`
		Precip    *float64 `json:"precip"`
		Wind      *float64 `json:"wind"`
		SunshineH *float64 `json:"sunshine_h"`
		DaylightH *float64 `json:"daylight_h"`
	} `json:"day"`
	Text     string `json:"text"`
	Note     string `json:"note"`
	Location *struct {
		Name string  `json:"name"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	} `json:"location"`
}

// Test implements Fetcher.
func (p *PocketWeather) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var a weatherDayAnswer
	if err := cl.Get(ctx, "/api/v1/day/"+time.Now().Format("2006-01-02"), nil, &a); err != nil {
		if err == pocket.ErrNotFound {
			return "", fmt.Errorf("Pocket Weather answers, but has no home place for you yet: add one in Pocket Weather")
		}
		return "", err
	}
	msg := "Pocket Weather answers"
	if a.Text != "" {
		msg += ": " + a.Text
	}
	if a.Location != nil && a.Location.Name != "" {
		msg += " at " + a.Location.Name
	}
	return msg, nil
}

// Fetch implements Fetcher: the weather at the person's home place in Pocket Weather.
func (p *PocketWeather) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	return p.FetchAt(ctx, date, 0, 0, "")
}

// FetchAt fetches the weather for a date at a place (lat/lon 0 = the
// person's home place in Pocket Weather). Today's comes with the warnings out
// for today.
func (p *PocketWeather) FetchAt(ctx context.Context, date string, lat, lon float64, place string) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var q url.Values
	if lat != 0 || lon != 0 {
		q = url.Values{"lat": {fmt.Sprintf("%.4f", lat)}, "lon": {fmt.Sprintf("%.4f", lon)}}
	}
	var a weatherDayAnswer
	if err := cl.Get(ctx, "/api/v1/day/"+date, q, &a); err != nil {
		return nil, err
	}
	d := weatherFromPocket(a, lat, lon, place)
	if d == nil {
		return nil, fmt.Errorf("pocket weather has nothing for that day yet")
	}
	if date == time.Now().Format("2006-01-02") {
		d.Warnings = p.warnings(ctx, cl, date)
	}
	return &Result{Payload: d, Headline: dayHeadline(d), Summary: daySummary(d)}, nil
}

// warnings are Pocket Weather's active warnings for the day; none when it
// does not answer (the day's weather does not wait on them).
func (p *PocketWeather) warnings(ctx context.Context, cl *pocket.Client, date string) []WeatherWarning {
	var a struct {
		Active []struct {
			Kind     string `json:"kind"`
			Level    string `json:"level"`
			Day      string `json:"day"`
			StartsAt string `json:"starts_at"`
			EndsAt   string `json:"ends_at"`
			Title    string `json:"title"`
			Detail   string `json:"detail"`
			Place    string `json:"place"`
		} `json:"active"`
	}
	if err := cl.Get(ctx, "/api/v1/warnings", nil, &a); err != nil {
		return nil
	}
	var out []WeatherWarning
	for _, w := range a.Active {
		if w.Day != date || strings.TrimSpace(w.Title) == "" {
			continue
		}
		out = append(out, WeatherWarning{Level: w.Level, Kind: w.Kind, Title: strings.TrimSpace(w.Title), Detail: strings.TrimSpace(w.Detail), Place: strings.TrimSpace(w.Place),
			From: sameDayClock(w.StartsAt, date), Until: sameDayClock(w.EndsAt, date)})
	}
	return out
}

// sameDayClock is "HH:MM" of a Pocket Weather time on the day, else "".
func sameDayClock(s, date string) string {
	s = strings.TrimSpace(s)
	if len(s) < 16 || s[:10] != date || (s[10] != 'T' && s[10] != ' ') {
		return ""
	}
	return s[11:16]
}

// weatherFromPocket maps Pocket Weather's day into Day's weather payload.
func weatherFromPocket(a weatherDayAnswer, lat, lon float64, place string) *Day {
	if a.Day.Code == nil && a.Day.TMax == nil && a.Day.Condition == "" {
		return nil
	}
	round := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		r := math.Round(*v*10) / 10
		return &r
	}
	code := -1
	if a.Day.Code != nil {
		code = *a.Day.Code
	}
	cond := ConditionFromCode(code)
	if cond == "" {
		cond = conditionFromPocket(a.Day.Condition)
	}
	d := &Day{Code: code, Condition: cond, TMax: round(a.Day.TMax), TMin: round(a.Day.TMin), PrecipMM: round(a.Day.Precip),
		SunshineH: round(a.Day.SunshineH), DaylightH: round(a.Day.DaylightH), WindMax: round(a.Day.Wind),
		Place: place, Lat: lat, Lon: lon, Note: strings.TrimSpace(a.Note), From: "pocket", Diary: a.Source == "diary"}
	if a.Location != nil {
		if d.Place == "" {
			d.Place = a.Location.Name
		}
		if lat == 0 && lon == 0 {
			d.Lat, d.Lon = a.Location.Lat, a.Location.Lon
		}
	}
	return d
}

// conditionFromPocket maps Pocket Weather's condition words to Day's.
func conditionFromPocket(c string) string {
	switch c {
	case "clear", "cloudy", "fog", "rain", "snow", "storm":
		return c
	case "mostly-clear", "partly":
		return "partly"
	case "drizzle", "freezing":
		return "rain"
	}
	return ""
}

// dayHeadline is a weather day's headline number: the high.
func dayHeadline(d *Day) float64 {
	if d.TMax != nil {
		return *d.TMax
	}
	return 0
}

// daySummary is a weather day's line: "Rain, 4–9 °C, 12 mm"; warnings of
// the day come first.
func daySummary(d *Day) string {
	sum := ConditionLabel(d.Condition)
	if d.TMax != nil && d.TMin != nil {
		sum += fmt.Sprintf(", %.0f–%.0f °C", *d.TMin, *d.TMax)
	}
	if d.PrecipMM != nil && *d.PrecipMM >= 1 {
		sum += fmt.Sprintf(", %.0f mm", *d.PrecipMM)
	}
	if len(d.Warnings) > 0 {
		var w []string
		for _, x := range d.Warnings {
			w = append(w, x.Title)
		}
		lvl := "Advisory"
		for _, x := range d.Warnings {
			if x.Level == "warning" {
				lvl = "Warning"
			}
		}
		sum = lvl + ": " + strings.Join(w, ", ") + "; " + sum
	}
	return sum
}
