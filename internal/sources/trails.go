package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Trails ----------------

// Trails reads the day in Pocket Trails: the hikes, walks, runs and rides
// planned or done that day (how far, how much up, how long, with whom) — and,
// for today, the outdoors: who is out now and by when they want to be back,
// the next outing within a week and the ones done lately. The tracks, the
// packing lists and the safety contact stay in Pocket Trails.
type Trails struct{ Conn pocket.Conn }

// NewTrails creates a fetcher over a connection.
func NewTrails(conn pocket.Conn) *Trails { return &Trails{Conn: conn} }

// Name implements Fetcher.
func (t *Trails) Name() string { return "trails" }

// Test implements Fetcher.
func (t *Trails) Test(ctx context.Context) (string, error) {
	cl, err := t.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Trails " + v, nil
}

// TrailsOuting is an outing on the day.
type TrailsOuting struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Status    string  `json:"status"` // planned, out, done
	Activity  string  `json:"activity,omitempty"`
	Date      string  `json:"date,omitempty"` // the recent ones (the day's go without)
	Time      string  `json:"time,omitempty"` // HH:MM setting off
	DistanceM float64 `json:"distance_m,omitempty"`
	GainM     float64 `json:"gain_m,omitempty"`
	Minutes   int     `json:"minutes,omitempty"`
	Rating    int     `json:"rating,omitempty"`
	People    string  `json:"people,omitempty"`
	Text      string  `json:"text,omitempty"` // "12.4 km · 980 m up · 4 h 30 min · with Ana", as Trails writes it
	Route     string  `json:"route,omitempty"`
	URL       string  `json:"url,omitempty"`
}

// TrailsAway is an outing someone is out on now, or the next one.
type TrailsAway struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Activity string `json:"activity,omitempty"`
	Date     string `json:"date"`
	Time     string `json:"time,omitempty"`    // HH:MM setting off
	BackBy   string `json:"back_by,omitempty"` // YYYY-MM-DDTHH:MM
	Days     int    `json:"days,omitempty"`    // the next one: days from today
	Minutes  int    `json:"minutes,omitempty"`
	URL      string `json:"url,omitempty"`
}

// TrailsToday is the outdoors today: out now, the next outing within a
// week, the ones done in the last two weeks and how many wait for a rating.
type TrailsToday struct {
	Out    []TrailsAway   `json:"out"`
	Next   *TrailsAway    `json:"next,omitempty"`
	Recent []TrailsOuting `json:"recent"`
	ToRate int            `json:"to_rate,omitempty"`
}

// TrailsDay is the day in Pocket Trails.
type TrailsDay struct {
	Date    string         `json:"date"`
	Outings []TrailsOuting `json:"outings"`
	Today   *TrailsToday   `json:"today,omitempty"` // only on today
}

// trailsRow is an outing as Trails' day, outings and summary give it.
type trailsRow struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	Activity  string  `json:"activity"`
	Date      string  `json:"date"`
	StartTime string  `json:"start_time"`
	BackBy    string  `json:"back_by"`
	DistanceM float64 `json:"distance_m"`
	GainM     float64 `json:"gain_m"`
	Minutes   int     `json:"minutes"`
	Rating    int     `json:"rating"`
	People    any     `json:"people"` // a line on the day, a list on an outing
	Text      string  `json:"text"`
	Route     string  `json:"route"`
	RouteName string  `json:"route_name"`
	Days      int     `json:"days"`
	URL       string  `json:"url"`
}

// trailsActivity names an outing without a title the way Trails does.
var trailsActivity = map[string]string{"hike": "Hike", "walk": "Walk", "run": "Run", "bike": "Ride", "gravel": "Gravel ride", "mtb": "Mountain-bike ride", "ski": "Ski tour", "other": "Outing"}

func (r trailsRow) title() string {
	t := strings.TrimSpace(r.Title)
	if t == "" {
		t = firstNonEmptyStr(trailsActivity[r.Activity], "Outing")
		if rn := strings.TrimSpace(firstNonEmptyStr(r.RouteName, r.Route)); rn != "" {
			t += ": " + rn
		}
	}
	return clipWords(t, 120)
}

func (r trailsRow) people() string {
	switch p := r.People.(type) {
	case string:
		return strings.TrimSpace(p)
	case []any:
		var names []string
		for _, x := range p {
			if m, ok := x.(map[string]any); ok {
				if n, _ := m["name"].(string); strings.TrimSpace(n) != "" {
					names = append(names, strings.TrimSpace(n))
				} else if e, _ := m["email"].(string); e != "" {
					names = append(names, e)
				}
			}
		}
		return strings.Join(names, ", ")
	}
	return ""
}

func (r trailsRow) outing() TrailsOuting {
	return TrailsOuting{ID: r.ID, Title: r.title(), Status: r.Status, Activity: r.Activity, Date: r.Date, Time: r.StartTime, DistanceM: r.DistanceM, GainM: r.GainM,
		Minutes: r.Minutes, Rating: r.Rating, People: r.people(), Text: clipWords(strings.TrimSpace(r.Text), 160), Route: strings.TrimSpace(firstNonEmptyStr(r.Route, r.RouteName)), URL: strings.TrimSpace(r.URL)}
}

func (r trailsRow) away() TrailsAway {
	return TrailsAway{ID: r.ID, Title: r.title(), Activity: r.Activity, Date: r.Date, Time: r.StartTime, BackBy: r.BackBy, Days: r.Days, Minutes: r.Minutes, URL: strings.TrimSpace(r.URL)}
}

// Fetch implements Fetcher.
func (t *Trails) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := t.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Outings []trailsRow `json:"outings"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := TrailsDay{Date: date, Outings: []TrailsOuting{}}
	for _, o := range page.Outings {
		if o.Status == "cancelled" {
			continue
		}
		d.Outings = append(d.Outings, o.outing())
	}
	if loc == nil {
		loc = time.Local
	}
	if date == time.Now().In(loc).Format("2006-01-02") {
		// the outdoors today; the day's outings stand without it
		if td, err := trailsToday(ctx, cl, date); err == nil {
			d.Today = td
		}
	}
	return &Result{Payload: d, Headline: float64(trailsBeenOut(d.Outings)), Summary: trailsSummary(d)}, nil
}

// trailsBeenOut counts the outings that happened (done, or out on the way).
func trailsBeenOut(os []TrailsOuting) int {
	n := 0
	for _, o := range os {
		if o.Status == "done" || o.Status == "out" {
			n++
		}
	}
	return n
}

// trailsToday reads GET /api/v1/summary (out now, the next outing) and the
// outings done in the two weeks before today (GET /api/v1/outings).
func trailsToday(ctx context.Context, cl *pocket.Client, date string) (*TrailsToday, error) {
	var sum struct {
		Out    []trailsRow `json:"out"`
		Next   *trailsRow  `json:"next"`
		ToRate int         `json:"to_rate"`
	}
	if err := cl.Get(ctx, "/api/v1/summary", nil, &sum); err != nil {
		return nil, err
	}
	td := &TrailsToday{Out: []TrailsAway{}, Recent: []TrailsOuting{}, ToRate: sum.ToRate}
	out := map[int64]bool{}
	for _, o := range sum.Out {
		out[o.ID] = true
		td.Out = append(td.Out, o.away())
	}
	day, _ := time.Parse("2006-01-02", date)
	week := day.AddDate(0, 0, 7).Format("2006-01-02")
	next := sum.Next
	if next != nil && (out[next.ID] || next.Status == "out") {
		// Trails' next is the one you are out on: the next planned one instead
		next = nil
		var list []trailsRow
		if err := cl.Get(ctx, "/api/v1/outings", url.Values{"status": {"planned"}, "from": {date}, "to": {week}, "limit": {"1"}}, &list); err == nil && len(list) > 0 {
			next = &list[0]
			if d, err := time.Parse("2006-01-02", next.Date); err == nil {
				next.Days = int(d.Sub(day).Hours() / 24)
			}
		}
	}
	if next != nil && next.Date != "" && next.Date <= week && next.Date >= date {
		a := next.away()
		td.Next = &a
	}
	var recent []trailsRow
	from := day.AddDate(0, 0, -14).Format("2006-01-02")
	yesterday := day.AddDate(0, 0, -1).Format("2006-01-02")
	if err := cl.Get(ctx, "/api/v1/outings", url.Values{"status": {"done"}, "from": {from}, "to": {yesterday}, "order": {"desc"}, "limit": {"3"}}, &recent); err == nil {
		for _, o := range recent {
			r := o.outing()
			r.Text = trailsLine(o)
			td.Recent = append(td.Recent, r)
		}
	}
	return td, nil
}

// trailsLine: "12.4 km · 980 m up · 4 h 30 min" from the numbers, for an
// outing Trails lists without its line.
func trailsLine(o trailsRow) string {
	var p []string
	if o.DistanceM > 0 {
		p = append(p, fmt.Sprintf("%.1f km", o.DistanceM/1000))
	}
	if o.GainM >= 1 {
		p = append(p, fmt.Sprintf("%.0f m up", o.GainM))
	}
	if o.Minutes > 0 {
		h, m := o.Minutes/60, o.Minutes%60
		switch {
		case h > 0 && m > 0:
			p = append(p, fmt.Sprintf("%d h %d min", h, m))
		case h > 0:
			p = append(p, fmt.Sprintf("%d h", h))
		default:
			p = append(p, fmt.Sprintf("%d min", m))
		}
	}
	return strings.Join(p, " · ")
}

// trailsOne: "Hike: Trebević (12.4 km · 980 m up · 4 h 30 min · with Ana)"
// for one done, "Run by the river at 18:00 (planned)" for one still to come.
func trailsOne(o TrailsOuting) string {
	s := o.Title
	switch o.Status {
	case "done":
		if o.Text != "" {
			s += " (" + o.Text + ")"
		}
	case "out":
		if o.Time != "" {
			s += " at " + o.Time
		}
		s += " (out)"
	default:
		if o.Time != "" {
			s += " at " + o.Time
		}
		s += " (planned)"
	}
	return s
}

// trailsSummary: the day's outings, the ones done first; "and 2 more".
func trailsSummary(d TrailsDay) string {
	var parts []string
	order := append(append([]TrailsOuting{}, trailsWith(d.Outings, true)...), trailsWith(d.Outings, false)...)
	for i, o := range order {
		if i == 3 {
			break
		}
		parts = append(parts, trailsOne(o))
	}
	s := strings.Join(parts, "; ")
	if n := len(order) - 3; n > 0 {
		s += fmt.Sprintf(" and %d more", n)
	}
	return s
}

func trailsWith(os []TrailsOuting, done bool) []TrailsOuting {
	var out []TrailsOuting
	for _, o := range os {
		if (o.Status == "done") == done {
			out = append(out, o)
		}
	}
	return out
}

// TrailsLines is every outing of the day for the AI's brief, at most 8.
func TrailsLines(d TrailsDay) []string {
	var ln []string
	for _, o := range d.Outings {
		ln = append(ln, trailsOne(o))
	}
	if len(ln) > 8 {
		ln = ln[:8]
	}
	return ln
}

// TrailsNow: "out now on Hike: Trebević (back by 17:30); next: Run by the
// river in 2 days (Sat 08:00)" — or "" when nobody is out and nothing is
// planned within a week (or it is not today).
func TrailsNow(t *TrailsToday) string {
	if t == nil {
		return ""
	}
	var p []string
	for _, o := range t.Out {
		s := "out now on " + o.Title
		if b := trailsBackBy(o); b != "" {
			s += " (back by " + b + ")"
		}
		p = append(p, s)
	}
	if n := t.Next; n != nil {
		when := "today"
		switch {
		case n.Days == 1:
			when = "tomorrow"
		case n.Days > 1:
			when = fmt.Sprintf("in %d days", n.Days)
		}
		if d, err := time.Parse("2006-01-02", n.Date); err == nil && n.Days > 1 {
			when += " (" + d.Format("Mon")
			if n.Time != "" {
				when += " " + n.Time
			}
			when += ")"
		} else if n.Time != "" {
			when += " at " + n.Time
		}
		p = append(p, "next: "+n.Title+" "+when)
	}
	return strings.Join(p, "; ")
}

// trailsBackBy: "17:30", or "Sat 17:30" when it is not the day set off.
func trailsBackBy(o TrailsAway) string {
	b, err := time.Parse("2006-01-02T15:04", o.BackBy)
	if err != nil {
		return ""
	}
	if b.Format("2006-01-02") == o.Date {
		return b.Format("15:04")
	}
	return b.Format("Mon 15:04")
}
