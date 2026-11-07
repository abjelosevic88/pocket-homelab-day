package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Tables ----------------

// Tables reads the day in Pocket Tables: the rows in your own small databases
// with a date on that day (a visit, a deadline, a delivery, a birthday), each
// with its table, its base and the date field, linking to the row. The rest of
// the row stays in Pocket Tables.
type Tables struct{ Conn pocket.Conn }

// NewTables creates a fetcher over a connection.
func NewTables(conn pocket.Conn) *Tables { return &Tables{Conn: conn} }

// Name implements Fetcher.
func (t *Tables) Name() string { return "tables" }

// Test implements Fetcher.
func (t *Tables) Test(ctx context.Context) (string, error) {
	cl, err := t.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Tables " + v, nil
}

// TablesRow is a row with a date on the day.
type TablesRow struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Table string `json:"table"`
	Icon  string `json:"table_icon,omitempty"`
	Base  string `json:"base,omitempty"`
	Field string `json:"field,omitempty"` // the date field ("Due", "Visit")
	Time  string `json:"time,omitempty"`  // HH:MM, when the date has a time
	URL   string `json:"url,omitempty"`   // the row in Pocket Tables
}

// TablesDay is the day in Pocket Tables.
type TablesDay struct {
	Date string      `json:"date"`
	Rows []TablesRow `json:"rows"`
}

// Fetch implements Fetcher.
func (t *Tables) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := t.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID        int64  `json:"id"`
			Title     string `json:"title"`
			Table     string `json:"table"`
			TableIcon string `json:"table_icon"`
			Base      string `json:"base"`
			Field     string `json:"field"`
			Date      string `json:"date"`
			URL       string `json:"url"`
		} `json:"items"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := TablesDay{Date: date, Rows: []TablesRow{}}
	seen := map[string]bool{} // a row with two dates on the day comes once per date
	for _, it := range page.Items {
		r := TablesRow{ID: it.ID, Title: clipWords(strings.TrimSpace(it.Title), 120), Table: strings.TrimSpace(it.Table), Icon: strings.TrimSpace(it.TableIcon),
			Base: strings.TrimSpace(it.Base), Field: strings.TrimSpace(it.Field), URL: strings.TrimSpace(it.URL)}
		r.Title = firstNonEmptyStr(r.Title, "A row")
		// "2026-11-07T14:30" (or with seconds / a zone) → "14:30"
		if len(it.Date) >= 16 && (it.Date[10] == 'T' || it.Date[10] == ' ') {
			r.Time = it.Date[11:16]
		}
		k := fmt.Sprintf("%d|%s|%s", r.ID, r.Field, r.Time)
		if seen[k] {
			continue
		}
		seen[k] = true
		d.Rows = append(d.Rows, r)
	}
	return &Result{Payload: d, Headline: float64(len(d.Rows)), Summary: tablesSummary(d)}, nil
}

// TablesRowName: "Ana Marić at 14:00 (Clients: Visit)" — the row, its time,
// its table and which date of it falls on the day.
func TablesRowName(r TablesRow) string {
	s := r.Title
	if r.Time != "" {
		s += " at " + r.Time
	}
	where := r.Table
	if r.Field != "" && where != "" {
		where += ": " + r.Field
	}
	if where != "" {
		s += " (" + where + ")"
	}
	return s
}

// tablesSummary: "Ana Marić at 14:00 (Clients: Visit); Logo (Projects: Due)
// and 2 more rows". The rows by name; what else they hold stays in Pocket Tables.
func tablesSummary(d TablesDay) string {
	var parts []string
	for i, r := range d.Rows {
		if i == 3 {
			break
		}
		parts = append(parts, TablesRowName(r))
	}
	s := strings.Join(parts, "; ")
	if n := len(d.Rows) - 3; n == 1 {
		s += " and 1 more row"
	} else if n > 1 {
		s += fmt.Sprintf(" and %d more rows", n)
	}
	return s
}
