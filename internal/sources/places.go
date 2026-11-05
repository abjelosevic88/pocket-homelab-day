package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Places ----------------

// Places reads the day in Pocket Places: where the person went (with whom,
// how they liked it, what it cost), what they planned for the day and the
// places they saved.
type Places struct{ Conn pocket.Conn }

// NewPlaces creates a fetcher over a connection.
func NewPlaces(conn pocket.Conn) *Places { return &Places{Conn: conn} }

// Name implements Fetcher.
func (p *Places) Name() string { return "places" }

// Test implements Fetcher.
func (p *Places) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Places " + v, nil
}

// PlacesVisit is a visit that day, or one planned for it.
type PlacesVisit struct {
	ID       int64   `json:"id"`
	PlaceID  int64   `json:"place_id"`
	Place    string  `json:"place"`
	Kind     string  `json:"kind"`
	City     string  `json:"city,omitempty"`
	Trip     string  `json:"trip,omitempty"`
	Time     string  `json:"time,omitempty"` // HH:MM
	With     string  `json:"with,omitempty"`
	Rating   float64 `json:"rating,omitempty"` // 0.5–5
	Minutes  int     `json:"minutes,omitempty"`
	Spent    int64   `json:"spent,omitempty"` // minor units
	Currency string  `json:"currency,omitempty"`
	Notes    string  `json:"notes,omitempty"`
	Line     string  `json:"line"` // "Dinner at Kibe Mahala with Ana, ★5, 68.00 BAM"
}

// PlacesSaved is a place saved that day.
type PlacesSaved struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Cuisine string `json:"cuisine,omitempty"`
	City    string `json:"city,omitempty"`
	Want    bool   `json:"want,omitempty"` // saved to try, not been yet
	Line    string `json:"line"`
}

// PlacesSpent is what the day's visits cost in one currency.
type PlacesSpent struct {
	Currency string `json:"currency"`
	Total    int64  `json:"total"` // minor units
	Visits   int    `json:"visits"`
}

// PlacesDay is the day in Pocket Places.
type PlacesDay struct {
	Visits  []PlacesVisit `json:"visits"`
	Planned []PlacesVisit `json:"planned"`
	Saved   []PlacesSaved `json:"saved"`
	Spent   []PlacesSpent `json:"spent"`
}

type placesVisitRow struct {
	ID        int64   `json:"id"`
	PlaceID   int64   `json:"place_id"`
	Time      string  `json:"time"`
	Rating    float64 `json:"rating"`
	Notes     string  `json:"notes"`
	WithWhom  string  `json:"with_whom"`
	Minutes   int     `json:"minutes"`
	Spent     int64   `json:"spent"`
	Currency  string  `json:"currency"`
	PlaceName string  `json:"place_name"`
	PlaceKind string  `json:"place_kind"`
	City      string  `json:"city"`
	TripName  string  `json:"trip_name"`
}

func (r placesVisitRow) visit() PlacesVisit {
	name := strings.TrimSpace(r.PlaceName)
	if name == "" {
		name = "a place"
	}
	return PlacesVisit{ID: r.ID, PlaceID: r.PlaceID, Place: name, Kind: r.PlaceKind, City: r.City, Trip: r.TripName, Time: clockOf(r.Time),
		With: strings.TrimSpace(r.WithWhom), Rating: r.Rating, Minutes: r.Minutes, Spent: r.Spent, Currency: r.Currency, Notes: strings.TrimSpace(r.Notes)}
}

// Fetch implements Fetcher.
func (p *Places) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Visits  []placesVisitRow `json:"visits"`
		Planned []placesVisitRow `json:"planned"`
		Added   []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Kind    string `json:"kind"`
			Cuisine string `json:"cuisine"`
			City    string `json:"city"`
			Status  string `json:"status"`
		} `json:"added"`
		Spent []PlacesSpent `json:"spent"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := PlacesDay{Visits: []PlacesVisit{}, Planned: []PlacesVisit{}, Saved: []PlacesSaved{}, Spent: []PlacesSpent{}}
	var parts []string
	went := map[int64]bool{} // places with a visit that day: the visit says more than "saved"
	for _, r := range page.Visits {
		v := r.visit()
		v.Line = placeVisitLine(v)
		d.Visits = append(d.Visits, v)
		parts = append(parts, v.Line)
		went[v.PlaceID] = true
	}
	for _, r := range page.Planned {
		v := r.visit()
		v.Line = placePlannedLine(v)
		d.Planned = append(d.Planned, v)
		parts = append(parts, v.Line)
		went[v.PlaceID] = true
	}
	var saved []string
	for _, a := range page.Added {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		s := PlacesSaved{ID: a.ID, Name: name, Kind: a.Kind, Cuisine: a.Cuisine, City: a.City, Want: a.Status == "want", Line: "Saved: " + name}
		if s.Want {
			s.Line = "Saved to try: " + name
		}
		d.Saved = append(d.Saved, s)
		if !went[a.ID] {
			saved = append(saved, name)
		}
	}
	if len(saved) > 0 {
		list := strings.Join(saved, ", ")
		if len(saved) > 3 {
			list = fmt.Sprintf("%s and %d more", strings.Join(saved[:3], ", "), len(saved)-3)
		}
		parts = append(parts, "Saved to Pocket Places: "+list)
	}
	for _, s := range page.Spent {
		if s.Total > 0 {
			d.Spent = append(d.Spent, s)
		}
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Visits)), Summary: sum}, nil
}

// placeVisitLine: "Dinner at Kibe Mahala with Ana, ★5, 68.00 BAM",
// "Coffee at Kafeterija (48 min)".
func placeVisitLine(v PlacesVisit) string {
	s := placeLead(v.Kind, v.Time) + " " + v.Place
	if v.With != "" {
		s += " with " + v.With
	}
	if v.Minutes > 0 {
		s += " (" + placeMinutes(v.Minutes) + ")"
	}
	if v.Rating > 0 {
		s += ", ★" + strconv.FormatFloat(v.Rating, 'f', -1, 64)
	}
	if v.Spent > 0 {
		s += ", " + minorAmount(v.Spent, v.Currency)
	}
	return s
}

// placePlannedLine: "Planned: dinner at Kibe Mahala with Ana",
// "Planned: Vrelo Bosne".
func placePlannedLine(v PlacesVisit) string {
	s := "Planned: "
	if what := placeWhat(v.Kind, v.Time); what != "" {
		s += what + " at "
	}
	s += v.Place
	if v.With != "" {
		s += " with " + v.With
	}
	return s
}

// placeWhat is what a visit was, from the kind of place and the time:
// dinner at a restaurant in the evening, coffee at a café; "" when the
// kind and time do not say.
func placeWhat(kind, clock string) string {
	switch kind {
	case "restaurant", "street_food":
		h, err := strconv.Atoi(strings.SplitN(clock, ":", 2)[0])
		switch {
		case clock == "" || err != nil || h < 5:
			return ""
		case h < 11:
			return "breakfast"
		case h < 16:
			return "lunch"
		}
		return "dinner"
	case "cafe":
		return "coffee"
	case "bar":
		return "drinks"
	case "dessert":
		return "dessert"
	}
	return ""
}

// placeLead starts a visit's line: "Dinner at", "Visited", "Stayed at".
func placeLead(kind, clock string) string {
	if what := placeWhat(kind, clock); what != "" {
		return strings.ToUpper(what[:1]) + what[1:] + " at"
	}
	switch kind {
	case "restaurant", "street_food":
		return "Ate at"
	case "bakery":
		return "Stopped by"
	case "market", "shop":
		return "Shopped at"
	case "museum", "sight", "park", "viewpoint", "beach", "nature":
		return "Visited"
	case "hotel":
		return "Stayed at"
	}
	return "Went to"
}

// placeMinutes: "48 min", "1 h 30 min", "2 h".
func placeMinutes(m int) string {
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

// clockOf is the HH:MM of a time Places stored ("19:30", "19:30:00"), ""
// when there is none.
func clockOf(t string) string {
	t = strings.TrimSpace(t)
	if len(t) >= 5 && t[2] == ':' {
		return t[:5]
	}
	return ""
}
