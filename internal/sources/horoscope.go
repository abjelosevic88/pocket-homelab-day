package sources

import (
	"context"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Horoscope ----------------

// Horoscope reads the day's sky and horoscope: the Moon, your day's
// headline and scores, the sky events (new and full Moons, eclipses,
// planets turning) and your mood check-in.
type Horoscope struct{ Conn pocket.Conn }

// NewHoroscope creates a fetcher over a connection.
func NewHoroscope(conn pocket.Conn) *Horoscope { return &Horoscope{Conn: conn} }

// Name implements Fetcher.
func (h *Horoscope) Name() string { return "horoscope" }

// Test implements Fetcher.
func (h *Horoscope) Test(ctx context.Context) (string, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Horoscope " + v, nil
}

// HoroscopeEvent is a sky event on the day, at its local time.
type HoroscopeEvent struct {
	Kind  string `json:"kind"`
	Time  string `json:"time"` // HH:MM
	Title string `json:"title"`
	Icon  string `json:"icon"`
}

// HoroscopeCheckin is the day's mood check-in.
type HoroscopeCheckin struct {
	Mood   int      `json:"mood"`
	Energy int      `json:"energy"`
	Note   string   `json:"note"`
	Tags   []string `json:"tags"`
}

// HoroscopeDay is the day in Pocket Horoscope.
type HoroscopeDay struct {
	MoonSign     string            `json:"moon_sign"`
	Phase        string            `json:"phase"`
	Illumination float64           `json:"illumination"`
	Headline     string            `json:"headline"`
	Summary      string            `json:"summary"`
	Overall      int               `json:"overall"`
	Scores       map[string]int    `json:"scores"`
	Transits     []string          `json:"transits"`
	Events       []HoroscopeEvent  `json:"events"`
	Checkin      *HoroscopeCheckin `json:"checkin,omitempty"`
	Reading      string            `json:"reading,omitempty"`
	URL          string            `json:"url"`
}

var phaseWords = map[string]string{"new": "New Moon", "waxing_crescent": "waxing crescent", "first_quarter": "first quarter", "waxing_gibbous": "waxing gibbous",
	"full": "Full Moon", "waning_gibbous": "waning gibbous", "last_quarter": "last quarter", "waning_crescent": "waning crescent"}

// MoonWords is "waxing gibbous Moon in Taurus".
func (d HoroscopeDay) MoonWords() string {
	if d.MoonSign == "" {
		return ""
	}
	sign := strings.ToUpper(d.MoonSign[:1]) + d.MoonSign[1:]
	switch d.Phase {
	case "new", "full":
		return phaseWords[d.Phase] + " in " + sign
	}
	if w := phaseWords[d.Phase]; w != "" {
		return w + " Moon in " + sign
	}
	return "Moon in " + sign
}

// Fetch implements Fetcher.
func (h *Horoscope) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		URL  string `json:"url"`
		Moon struct {
			Sign         string  `json:"sign"`
			Phase        string  `json:"phase"`
			Illumination float64 `json:"illumination"`
		} `json:"moon"`
		Headline string         `json:"headline"`
		Summary  string         `json:"summary"`
		Overall  int            `json:"overall"`
		Scores   map[string]int `json:"scores"`
		Transits []string       `json:"transits"`
		Reading  string         `json:"reading"`
		Events   []struct {
			Kind  string `json:"kind"`
			Time  string `json:"time"`
			Title string `json:"title"`
			Icon  string `json:"icon"`
		} `json:"events"`
		Journal *struct {
			Mood   int      `json:"mood"`
			Energy int      `json:"energy"`
			Note   string   `json:"note"`
			Tags   []string `json:"tags"`
		} `json:"journal"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := HoroscopeDay{MoonSign: page.Moon.Sign, Phase: page.Moon.Phase, Illumination: page.Moon.Illumination, Headline: page.Headline, Summary: page.Summary,
		Overall: page.Overall, Scores: page.Scores, Transits: page.Transits, Events: []HoroscopeEvent{}, Reading: page.Reading, URL: page.URL}
	if d.Transits == nil {
		d.Transits = []string{}
	}
	if d.Scores == nil {
		d.Scores = map[string]int{}
	}
	for _, e := range page.Events {
		t := ""
		if ts, ok := ParseAny(e.Time); ok {
			t = LocalClock(ts, loc)
		}
		d.Events = append(d.Events, HoroscopeEvent{Kind: e.Kind, Time: t, Title: e.Title, Icon: e.Icon})
	}
	if j := page.Journal; j != nil {
		d.Checkin = &HoroscopeCheckin{Mood: j.Mood, Energy: j.Energy, Note: j.Note, Tags: j.Tags}
	}
	parts := []string{}
	if m := d.MoonWords(); m != "" {
		parts = append(parts, m)
	}
	for _, e := range d.Events {
		if e.Kind == "eclipse" || e.Kind == "station" {
			parts = append(parts, e.Title)
		}
	}
	if d.Headline != "" {
		parts = append(parts, strings.TrimSuffix(d.Headline, "."))
	}
	return &Result{Payload: d, Headline: float64(d.Overall), Summary: strings.Join(parts, " · ")}, nil
}
