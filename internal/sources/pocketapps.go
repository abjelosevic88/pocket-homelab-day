package sources

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Habits ----------------

// Habits reads the day's check-in: habits, trackers, the journal line.
type Habits struct{ Conn pocket.Conn }

// NewHabits creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewHabits(conn pocket.Conn) *Habits { return &Habits{Conn: conn} }

// Name implements Fetcher.
func (h *Habits) Name() string { return "habits" }

// Test implements Fetcher.
func (h *Habits) Test(ctx context.Context) (string, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Habits " + v, nil
}

// HabitRow is one habit on the day.
type HabitRow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Status    int    `json:"status"` // 1 done, -1 missed, 0 open
	Scheduled bool   `json:"scheduled"`
	Streak    int    `json:"streak"`
}

// TrackerRow is one tracker value on the day.
type TrackerRow struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Unit           string   `json:"unit"`
	Kind           string   `json:"kind"` // scale | number | yesno
	Value          *float64 `json:"value"`
	Min            *float64 `json:"min,omitempty"`
	Max            *float64 `json:"max,omitempty"`
	HigherIsBetter *bool    `json:"higher_is_better,omitempty"`
	Previous       *float64 `json:"previous,omitempty"`
	Defaulted      bool     `json:"defaulted,omitempty"` // the tracker's default, not something entered
}

// Checkin is the day in Pocket Habits.
type Checkin struct {
	Done      int          `json:"done"`
	Total     int          `json:"total"`
	Habits    []HabitRow   `json:"habits"`
	Trackers  []TrackerRow `json:"trackers"`
	Journal   string       `json:"journal"`
	Bad       string       `json:"journal_bad,omitempty"`
	Important string       `json:"important,omitempty"`
}

// Fetch implements Fetcher.
func (h *Habits) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Done    int    `json:"done"`
		Total   int    `json:"total"`
		Journal string `json:"journal"`
		Bad     string `json:"journal_bad"`
		Imp     string `json:"important"`
		Habits  []struct {
			ID        int64  `json:"id"`
			Name      string `json:"name"`
			Color     string `json:"color"`
			Status    int    `json:"status"`
			Scheduled bool   `json:"scheduled"`
			Streak    int    `json:"streak"`
		} `json:"habits"`
		Trackers []struct {
			ID             int64    `json:"id"`
			Name           string   `json:"name"`
			Unit           string   `json:"unit"`
			Kind           string   `json:"kind"`
			Value          *float64 `json:"value"`
			Min            *float64 `json:"min_value"`
			Max            *float64 `json:"max_value"`
			HigherIsBetter *bool    `json:"higher_is_better"`
			Previous       *float64 `json:"previous"`
			Defaulted      bool     `json:"defaulted"`
		} `json:"trackers"`
	}
	if err := cl.Get(ctx, "/api/v1/days/"+date, nil, &page); err != nil {
		return nil, err
	}
	c := Checkin{Done: page.Done, Total: page.Total, Journal: page.Journal, Bad: page.Bad, Important: page.Imp, Habits: []HabitRow{}, Trackers: []TrackerRow{}}
	for _, x := range page.Habits {
		c.Habits = append(c.Habits, HabitRow{ID: x.ID, Name: x.Name, Color: x.Color, Status: x.Status, Scheduled: x.Scheduled, Streak: x.Streak})
	}
	for _, x := range page.Trackers {
		c.Trackers = append(c.Trackers, TrackerRow{ID: x.ID, Name: x.Name, Unit: x.Unit, Kind: x.Kind, Value: x.Value, Min: x.Min, Max: x.Max, HigherIsBetter: x.HigherIsBetter, Previous: x.Previous, Defaulted: x.Defaulted})
	}
	sum := ""
	if c.Total > 0 {
		sum = fmt.Sprintf("%d of %d habits", c.Done, c.Total)
	}
	if c.Journal != "" {
		sum = JoinNonEmpty(" · ", sum, "“"+trim(c.Journal, 60)+"”")
	}
	headline := 0.0
	if c.Total > 0 {
		headline = Round1(float64(c.Done) / float64(c.Total) * 100)
	}
	return &Result{Payload: c, Headline: headline, Summary: sum}, nil
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// ---------------- Pocket Bookshelf ----------------

// Bookshelf reads what was being read, started or finished on the day.
type Bookshelf struct{ Conn pocket.Conn }

// NewBookshelf creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewBookshelf(conn pocket.Conn) *Bookshelf { return &Bookshelf{Conn: conn} }

// Name implements Fetcher.
func (b *Bookshelf) Name() string { return "bookshelf" }

// Test implements Fetcher.
func (b *Bookshelf) Test(ctx context.Context) (string, error) {
	cl, err := b.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Bookshelf " + v, nil
}

// Book is one book on the day.
type Book struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Authors  string  `json:"authors"`
	Pages    int     `json:"pages,omitempty"`
	Page     int     `json:"page,omitempty"`
	Percent  int     `json:"percent,omitempty"`
	Rating   float64 `json:"rating,omitempty"`
	Status   string  `json:"status"` // reading | started | finished
	Cover    string  `json:"cover,omitempty"`
	Finished string  `json:"finished_on,omitempty"`
	Started  string  `json:"started_on,omitempty"`
}

// Reading is the day on the bookshelf.
type Reading struct {
	Reading  []Book `json:"reading"`
	Started  []Book `json:"started"`
	Finished []Book `json:"finished"`
	// Sittings with a book on the e-reader (KOReader / CrossPoint), oldest first.
	Sessions []ReadingSession `json:"sessions,omitempty"`
	Pages    int              `json:"pages,omitempty"`   // read in those sessions
	Minutes  int              `json:"minutes,omitempty"` // spent in them
}

// ReadingSession is one sitting with a book, as Bookshelf works it out from the e-reader's syncs.
type ReadingSession struct {
	BookID    int64  `json:"book_id"`
	Title     string `json:"title"`
	Authors   string `json:"authors"`
	Time      string `json:"time"` // HH:MM
	End       string `json:"end"`  // HH:MM
	Minutes   int    `json:"minutes"`
	Pages     int    `json:"pages"`
	FromPage  int    `json:"from_page,omitempty"`
	ToPage    int    `json:"to_page,omitempty"`
	Percent   int    `json:"percent,omitempty"` // of the book read, when it has no page count
	Device    string `json:"device,omitempty"`
	Estimated bool   `json:"estimated,omitempty"` // minutes partly guessed from the pace
}

// readingSessions asks Bookshelf for the day's e-reader sessions; an older
// Bookshelf without them answers nothing.
func readingSessions(ctx context.Context, cl *pocket.Client, date string, loc *time.Location) []ReadingSession {
	var raw struct {
		Sessions []struct {
			BookID    int64   `json:"book_id"`
			Title     string  `json:"title"`
			Authors   string  `json:"authors"`
			Start     string  `json:"start"`
			End       string  `json:"end"`
			Minutes   int     `json:"minutes"`
			Pages     int     `json:"pages"`
			From      float64 `json:"from"`
			To        float64 `json:"to"`
			FromPage  int     `json:"from_page"`
			ToPage    int     `json:"to_page"`
			Device    string  `json:"device"`
			Estimated bool    `json:"estimated"`
		} `json:"sessions"`
	}
	if err := cl.Get(ctx, "/api/v1/reading/sessions", url.Values{"from": {date}, "to": {date}}, &raw); err != nil {
		return nil
	}
	var out []ReadingSession
	for i := len(raw.Sessions) - 1; i >= 0; i-- { // newest first → oldest first
		r := raw.Sessions[i]
		st, _ := time.Parse(time.RFC3339, r.Start)
		en, _ := time.Parse(time.RFC3339, r.End)
		rs := ReadingSession{BookID: r.BookID, Title: r.Title, Authors: r.Authors, Time: LocalClock(st, loc), End: LocalClock(en, loc),
			Minutes: r.Minutes, Pages: r.Pages, FromPage: r.FromPage, ToPage: r.ToPage, Device: r.Device, Estimated: r.Estimated}
		if r.Pages == 0 && r.To > r.From {
			rs.Percent = max(1, int((r.To-r.From)*100+0.5))
		}
		out = append(out, rs)
	}
	return out
}

type bookRow struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Authors     string  `json:"authors"`
	Pages       int     `json:"pages"`
	CurrentPage int     `json:"current_page"`
	Rating      float64 `json:"rating"`
	Status      string  `json:"status"`
	Cover       string  `json:"cover"`
	StartedOn   string  `json:"started_on"`
	FinishedOn  string  `json:"finished_on"`
	UpdatedAt   string  `json:"updated_at"`
}

func decodeBooks(c *pocket.Client, ctx context.Context, q url.Values) ([]bookRow, error) {
	var raw []bookRow
	if err := c.Get(ctx, "/api/v1/books", q, &raw); err != nil {
		var wrap struct {
			Books []bookRow `json:"books"`
		}
		if err2 := c.Get(ctx, "/api/v1/books", q, &wrap); err2 != nil {
			return nil, err
		}
		raw = wrap.Books
	}
	return raw, nil
}

func toBook(r bookRow, status string) Book {
	b := Book{ID: r.ID, Title: r.Title, Authors: r.Authors, Pages: r.Pages, Page: r.CurrentPage, Rating: r.Rating, Status: status, Cover: r.Cover, Started: r.StartedOn, Finished: r.FinishedOn}
	if r.Pages > 0 && r.CurrentPage > 0 {
		b.Percent = int(float64(r.CurrentPage) / float64(r.Pages) * 100)
	}
	return b
}

// Fetch implements Fetcher.
func (b *Bookshelf) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := b.Conn(ctx)
	if err != nil {
		return nil, err
	}
	rd := Reading{Reading: []Book{}, Started: []Book{}, Finished: []Book{}}
	today := time.Now().In(loc).Format("2006-01-02")
	year := date[:4]
	// Finished (or started) exactly on that day, from the year's finished list and the reading list.
	fin, err := decodeBooks(cl, ctx, url.Values{"year": {year}, "limit": {"500"}})
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, r := range fin {
		if r.FinishedOn == date {
			rd.Finished = append(rd.Finished, toBook(r, "finished"))
			seen[r.ID] = true
		}
		if r.StartedOn == date && r.FinishedOn != date {
			rd.Started = append(rd.Started, toBook(r, "started"))
			seen[r.ID] = true
		}
	}
	cur, err := decodeBooks(cl, ctx, url.Values{"status": {"reading"}, "limit": {"100"}})
	if err != nil {
		return nil, err
	}
	for _, r := range cur {
		if seen[r.ID] {
			continue
		}
		if r.StartedOn == date {
			rd.Started = append(rd.Started, toBook(r, "started"))
			continue
		}
		// A book in progress counts for days between its start and today.
		if date == today || (r.StartedOn != "" && r.StartedOn <= date && date <= today) {
			rd.Reading = append(rd.Reading, toBook(r, "reading"))
		}
	}
	rd.Sessions = readingSessions(ctx, cl, date, loc)
	inList := map[int64]bool{}
	for _, l := range [][]Book{rd.Reading, rd.Started, rd.Finished} {
		for _, x := range l {
			inList[x.ID] = true
		}
	}
	for _, x := range rd.Sessions {
		rd.Pages += x.Pages
		rd.Minutes += x.Minutes
		if !inList[x.BookID] { // read that day, since finished or put down
			rd.Reading = append(rd.Reading, Book{ID: x.BookID, Title: x.Title, Authors: x.Authors, Status: "reading"})
			inList[x.BookID] = true
		}
	}
	var parts []string
	if rd.Minutes > 0 {
		if rd.Pages > 0 {
			parts = append(parts, fmt.Sprintf("read %d pages in %s", rd.Pages, HoursMinutes(rd.Minutes)))
		} else {
			parts = append(parts, "read for "+HoursMinutes(rd.Minutes))
		}
	}
	if len(rd.Finished) > 0 {
		parts = append(parts, "finished "+rd.Finished[0].Title)
	}
	if len(rd.Started) > 0 {
		parts = append(parts, "started "+rd.Started[0].Title)
	}
	if len(rd.Sessions) > 0 && len(rd.Finished) == 0 && len(rd.Started) == 0 {
		parts = append(parts, rd.Sessions[len(rd.Sessions)-1].Title)
	} else if len(parts) == 0 && len(rd.Reading) > 0 {
		parts = append(parts, "reading "+rd.Reading[0].Title)
	}
	return &Result{Payload: rd, Headline: float64(len(rd.Finished)), Summary: strings.Join(parts, " · ")}, nil
}

// ---------------- Pocket Travel ----------------

// Travel finds the trip (if any) the day fell in.
type Travel struct{ Conn pocket.Conn }

// NewTravel creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewTravel(conn pocket.Conn) *Travel { return &Travel{Conn: conn} }

// Name implements Fetcher.
func (t *Travel) Name() string { return "travel" }

// Test implements Fetcher.
func (t *Travel) Test(ctx context.Context) (string, error) {
	cl, err := t.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Travel " + v, nil
}

// Trip is a trip the day belongs to.
type Trip struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	DayNo       int    `json:"day_no"`
	Days        int    `json:"days"`
	Color       string `json:"color"`
	Status      string `json:"status"`
}

// Trips is the day's travel context.
type Trips struct {
	Trips []Trip `json:"trips"`
}

// Fetch implements Fetcher.
func (t *Travel) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := t.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Destination string `json:"destination"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		Color       string `json:"color"`
		Status      string `json:"status"`
		Archived    bool   `json:"archived"`
	}
	if err := cl.Get(ctx, "/api/v1/trips", url.Values{"all": {"1"}}, &raw); err != nil {
		return nil, err
	}
	out := Trips{Trips: []Trip{}}
	for _, r := range raw {
		if r.StartDate == "" || r.EndDate == "" || date < r.StartDate || date > r.EndDate {
			continue
		}
		st, _ := time.Parse("2006-01-02", r.StartDate)
		en, _ := time.Parse("2006-01-02", r.EndDate)
		d, _ := time.Parse("2006-01-02", date)
		out.Trips = append(out.Trips, Trip{ID: r.ID, Name: r.Name, Destination: r.Destination, StartDate: r.StartDate, EndDate: r.EndDate, DayNo: int(d.Sub(st).Hours()/24) + 1, Days: int(en.Sub(st).Hours()/24) + 1, Color: r.Color, Status: r.Status})
	}
	sum := ""
	if len(out.Trips) > 0 {
		tr := out.Trips[0]
		sum = fmt.Sprintf("Day %d of %d · %s", tr.DayNo, tr.Days, JoinNonEmpty(" – ", tr.Name, tr.Destination))
	}
	return &Result{Payload: out, Headline: float64(len(out.Trips)), Summary: sum}, nil
}

// ---------------- Pocket Kitchen ----------------

// Kitchen reads what was cooked on the day.
type Kitchen struct{ Conn pocket.Conn }

// NewKitchen creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewKitchen(conn pocket.Conn) *Kitchen { return &Kitchen{Conn: conn} }

// Name implements Fetcher.
func (k *Kitchen) Name() string { return "kitchen" }

// Test implements Fetcher.
func (k *Kitchen) Test(ctx context.Context) (string, error) {
	cl, err := k.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Kitchen " + v, nil
}

// Cook is one dish cooked.
type Cook struct {
	ID       int64  `json:"id"`
	RecipeID *int64 `json:"recipe_id"`
	Title    string `json:"title"`
	Servings int    `json:"servings"`
	Rating   int    `json:"rating"`
	Notes    string `json:"notes"`
	By       string `json:"by,omitempty"`
}

// Cooked is the day in the kitchen.
type Cooked struct {
	Cooks []Cook `json:"cooks"`
}

// Fetch implements Fetcher.
func (k *Kitchen) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := k.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID          int64  `json:"id"`
		RecipeID    *int64 `json:"recipe_id"`
		Date        string `json:"date"`
		Servings    int    `json:"servings"`
		Notes       string `json:"notes"`
		Rating      int    `json:"rating"`
		RecipeTitle string `json:"recipe_title"`
		UserName    string `json:"user_name"`
	}
	if err := cl.Get(ctx, "/api/v1/cooklog", url.Values{"limit": {"500"}}, &raw); err != nil {
		return nil, err
	}
	out := Cooked{Cooks: []Cook{}}
	for _, r := range raw {
		if r.Date != date {
			continue
		}
		title := r.RecipeTitle
		if title == "" {
			title = r.Notes
		}
		if title == "" {
			title = "Something"
		}
		out.Cooks = append(out.Cooks, Cook{ID: r.ID, RecipeID: r.RecipeID, Title: title, Servings: r.Servings, Rating: r.Rating, Notes: r.Notes, By: r.UserName})
	}
	sort.SliceStable(out.Cooks, func(i, j int) bool { return out.Cooks[i].ID < out.Cooks[j].ID })
	sum := ""
	if len(out.Cooks) > 0 {
		names := make([]string, 0, len(out.Cooks))
		for _, c := range out.Cooks {
			names = append(names, c.Title)
		}
		sum = "cooked " + strings.Join(names, ", ")
	}
	return &Result{Payload: out, Headline: float64(len(out.Cooks)), Summary: sum}, nil
}

// ---------------- Pocket Garage ----------------

// Garage reads the day's fill-ups and service records.
type Garage struct{ Conn pocket.Conn }

// NewGarage creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewGarage(conn pocket.Conn) *Garage { return &Garage{Conn: conn} }

// Name implements Fetcher.
func (g *Garage) Name() string { return "garage" }

// Test implements Fetcher.
func (g *Garage) Test(ctx context.Context) (string, error) {
	cl, err := g.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Garage " + v, nil
}

// Fill is one fuel entry or service record.
type Fill struct {
	Vehicle  string  `json:"vehicle"`
	Kind     string  `json:"kind"` // fuel | service | repair | …
	Title    string  `json:"title"`
	Volume   float64 `json:"volume,omitempty"`
	Cost     int64   `json:"cost"` // minor units
	Currency string  `json:"currency"`
	Odometer float64 `json:"odometer,omitempty"`
	Station  string  `json:"station,omitempty"`
}

// Car is the day in the garage.
type Car struct {
	Items []Fill `json:"items"`
}

// Fetch implements Fetcher.
func (g *Garage) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := g.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var vehicles []struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Make  string `json:"make"`
		Model string `json:"model"`
	}
	if err := cl.Get(ctx, "/api/v1/vehicles", url.Values{"bare": {"1"}}, &vehicles); err != nil {
		return nil, err
	}
	out := Car{Items: []Fill{}}
	for _, v := range vehicles {
		name := v.Name
		if name == "" {
			name = JoinNonEmpty(" ", v.Make, v.Model)
		}
		var fuel []struct {
			Date     string  `json:"date"`
			Volume   float64 `json:"volume"`
			Cost     int64   `json:"cost"`
			Currency string  `json:"currency"`
			Odometer float64 `json:"odometer"`
			Station  string  `json:"station"`
		}
		if err := cl.Get(ctx, fmt.Sprintf("/api/v1/vehicles/%d/fuel", v.ID), nil, &fuel); err == nil {
			for _, f := range fuel {
				if f.Date == date {
					out.Items = append(out.Items, Fill{Vehicle: name, Kind: "fuel", Title: fmt.Sprintf("%.1f l", f.Volume), Volume: f.Volume, Cost: f.Cost, Currency: f.Currency, Odometer: f.Odometer, Station: f.Station})
				}
			}
		}
		var recs []struct {
			Date     string  `json:"date"`
			Kind     string  `json:"kind"`
			Title    string  `json:"title"`
			Cost     int64   `json:"cost"`
			Currency string  `json:"currency"`
			Odometer float64 `json:"odometer"`
			Vendor   string  `json:"vendor"`
		}
		if err := cl.Get(ctx, fmt.Sprintf("/api/v1/vehicles/%d/records", v.ID), url.Values{"from": {date}, "to": {date}}, &recs); err == nil {
			for _, r := range recs {
				if r.Date == date {
					out.Items = append(out.Items, Fill{Vehicle: name, Kind: r.Kind, Title: r.Title, Cost: r.Cost, Currency: r.Currency, Odometer: r.Odometer, Station: r.Vendor})
				}
			}
		}
	}
	sum := ""
	if len(out.Items) > 0 {
		var parts []string
		for _, it := range out.Items {
			if it.Kind == "fuel" {
				parts = append(parts, "fuel "+it.Title)
			} else {
				parts = append(parts, it.Kind+" "+it.Title)
			}
		}
		sum = strings.Join(parts, ", ")
	}
	return &Result{Payload: out, Headline: float64(len(out.Items)), Summary: sum}, nil
}
