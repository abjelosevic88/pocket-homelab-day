package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// Movement reads the day from Pocket Movement (the person's own location
// history) through its day view: journeys with their mode and distance,
// stops with the names of their places. It fills the same Where payload as
// Dawarich, so every card, the timeline and the briefs read it unchanged.
// Only the day view is used (movement:read); raw points are never fetched.
type Movement struct{ Conn pocket.Conn }

// NewMovement creates a client over a connection (a Pocket Core link).
func NewMovement(conn pocket.Conn) *Movement { return &Movement{Conn: conn} }

// Name implements Fetcher: the location source keeps Dawarich's id, so the
// days already recorded and everything reading them stay as they are.
func (p *Movement) Name() string { return "dawarich" }

// Test implements Fetcher: today's day view, the same call a pull makes.
func (p *Movement) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var out map[string]any
	if err := c.Get(ctx, "/api/v1/day/"+time.Now().Format("2006-01-02"), nil, &out); err != nil {
		return "", err
	}
	return "Pocket Movement reachable", nil
}

// movementDay is the part of GET /api/v1/day/{date} Day uses. Distances are
// metres, times unix seconds.
type movementDay struct {
	Summary *struct {
		Distance float64            `json:"distance"`
		MovingS  int64              `json:"moving_s"`
		Modes    map[string]float64 `json:"modes"`
	} `json:"summary"`
	Entries []struct {
		Type     string    `json:"type"` // visit | journey
		StartTS  int64     `json:"start_ts"`
		EndTS    int64     `json:"end_ts"`
		Name     string    `json:"name"`
		Status   string    `json:"status"`
		Lat      *float64  `json:"lat"`
		Lng      *float64  `json:"lng"`
		Distance float64   `json:"distance"`
		Mode     string    `json:"mode"`
		Start    []float64 `json:"start"`
		End      []float64 `json:"end"`
	} `json:"entries"`
}

// Fetch implements Fetcher.
func (p *Movement) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	if _, _, err := DayBounds(date, loc); err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var d movementDay
	if err := c.Get(ctx, "/api/v1/day/"+date, nil, &d); err != nil {
		return nil, err
	}
	w := movementWhere(d, loc)
	return &Result{Payload: w, Headline: w.DistanceKm, Summary: whereSummary(w)}, nil
}

func movementWhere(d movementDay, loc *time.Location) Where {
	w := Where{Modes: map[string]float64{}, Entries: []Visit{}, Provider: "movement"}
	var b *[4]float64
	grow := func(lat, lng float64) {
		if lat == 0 && lng == 0 {
			return
		}
		if b == nil {
			b = &[4]float64{lat, lng, lat, lng}
			return
		}
		b[0], b[1] = min(b[0], lat), min(b[1], lng)
		b[2], b[3] = max(b[2], lat), max(b[3], lng)
	}
	journeyKm, journeyS := 0.0, int64(0)
	for _, e := range d.Entries {
		st, en := time.Unix(e.StartTS, 0), time.Unix(e.EndTS, 0)
		v := Visit{Kind: e.Type, Start: LocalClock(st, loc), End: LocalClock(en, loc), Minutes: int(e.EndTS-e.StartTS) / 60}
		v.StartDate, v.EndDate = LocalDate(st, loc), LocalDate(en, loc)
		switch e.Type {
		case "journey":
			v.Mode, v.Distance = e.Mode, Round1(e.Distance/1000)
			w.Journeys++
			journeyKm += e.Distance / 1000
			journeyS += e.EndTS - e.StartTS
			if w.FirstMove == "" {
				w.FirstMove = v.Start
			}
			w.LastMove = v.End
			for _, pt := range [][]float64{e.Start, e.End} {
				if len(pt) == 2 {
					grow(pt[0], pt[1])
				}
			}
		case "visit":
			if e.Status == "declined" {
				continue
			}
			v.Name, v.Status, v.Lat, v.Lng = e.Name, e.Status, e.Lat, e.Lng
			w.Places++
			w.StillMinutes += v.Minutes
			if e.Lat != nil && e.Lng != nil {
				grow(*e.Lat, *e.Lng)
			}
		default:
			continue
		}
		w.Entries = append(w.Entries, v)
	}
	if d.Summary != nil {
		w.DistanceKm = d.Summary.Distance / 1000
		w.MovingMinutes = int(d.Summary.MovingS / 60)
		for k, m := range d.Summary.Modes {
			if m > 0 {
				w.Modes[k] = Round1(m / 1000)
			}
		}
	}
	// A day not summarised yet still has its journeys.
	if w.DistanceKm == 0 {
		w.DistanceKm = journeyKm
	}
	if w.MovingMinutes == 0 {
		w.MovingMinutes = int(journeyS / 60)
	}
	if len(w.Modes) == 0 {
		for _, e := range w.Entries {
			if e.Kind == "journey" && e.Distance > 0 {
				w.Modes[firstNonBlank(e.Mode, "unknown")] += e.Distance
			}
		}
	}
	w.DistanceKm = Round1(w.DistanceKm)
	w.Bounds = b
	return w
}

// whereSummary is the one line under a location card.
func whereSummary(w Where) string {
	if w.DistanceKm <= 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("%.1f km", w.DistanceKm)}
	if w.MovingMinutes > 0 {
		parts = append(parts, HoursMinutes(w.MovingMinutes)+" moving")
	}
	if w.Places > 0 {
		parts = append(parts, Plural(w.Places, "place", "places"))
	}
	return strings.Join(parts, " · ")
}

func firstNonBlank(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
