package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Rentals ----------------

// Rentals reads the day in Pocket Rentals: the rent that came in, leases that
// started or ended, repairs reported or done and the inspections that day —
// and, for today, what needs you: rent late or due today and urgent repairs.
// The tenants' details and the books stay in Pocket Rentals.
type Rentals struct{ Conn pocket.Conn }

// NewRentals creates a fetcher over a connection.
func NewRentals(conn pocket.Conn) *Rentals { return &Rentals{Conn: conn} }

// Name implements Fetcher.
func (r *Rentals) Name() string { return "rentals" }

// Test implements Fetcher.
func (r *Rentals) Test(ctx context.Context) (string, error) {
	cl, err := r.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Rentals " + v, nil
}

// RentalsItem is one thing that happened in your rentals that day.
type RentalsItem struct {
	Kind     string `json:"kind"` // payment, lease, repair, inspection
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url,omitempty"`
}

// RentalsRent is a rent late or due today.
type RentalsRent struct {
	LeaseID  int64  `json:"lease_id"`
	Lease    string `json:"lease"`
	Place    string `json:"place,omitempty"`
	Tenants  string `json:"tenants,omitempty"`
	Title    string `json:"title,omitempty"`
	DueDate  string `json:"due_date"`
	Owed     int64  `json:"owed"` // minor units still to pay
	Currency string `json:"currency"`
	Days     int    `json:"days,omitempty"` // days late
}

// RentalsRepair is an urgent repair still open.
type RentalsRepair struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Place    string `json:"place,omitempty"`
	Status   string `json:"status,omitempty"`
	Reported string `json:"reported_on,omitempty"`
}

// RentalsToday is what needs you today in Pocket Rentals.
type RentalsToday struct {
	Late    []RentalsRent   `json:"late"`
	Due     []RentalsRent   `json:"due"`
	Urgent  []RentalsRepair `json:"urgent"`
	Repairs int             `json:"repairs"` // open repairs Rentals listed
}

// RentalsDay is the day in Pocket Rentals.
type RentalsDay struct {
	Date  string        `json:"date"`
	Items []RentalsItem `json:"items"`
	Today *RentalsToday `json:"today,omitempty"` // only on today
}

// Fetch implements Fetcher.
func (r *Rentals) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := r.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			Kind     string `json:"kind"`
			Title    string `json:"title"`
			Subtitle string `json:"subtitle"`
			URL      string `json:"url"`
		} `json:"items"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := RentalsDay{Date: date, Items: []RentalsItem{}}
	for _, it := range page.Items {
		t := strings.TrimSpace(it.Title)
		if t == "" {
			continue
		}
		d.Items = append(d.Items, RentalsItem{Kind: strings.TrimSpace(it.Kind), Title: clipWords(t, 120), Subtitle: clipWords(strings.TrimSpace(it.Subtitle), 120), URL: strings.TrimSpace(it.URL)})
	}
	if loc == nil {
		loc = time.Local
	}
	if date == time.Now().In(loc).Format("2006-01-02") {
		// what needs you today; the day's items stand without it
		if t, err := rentalsToday(ctx, cl, date); err == nil {
			d.Today = t
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Items)), Summary: rentalsSummary(d)}, nil
}

// rentalsToday reads GET /api/v1/today and keeps the rent late, the rent due
// by today and the urgent repairs still open.
func rentalsToday(ctx context.Context, cl *pocket.Client, date string) (*RentalsToday, error) {
	type due struct {
		LeaseID int64  `json:"lease_id"`
		Lease   string `json:"lease"`
		Place   string `json:"place"`
		Tenants []struct {
			Name string `json:"name"`
		} `json:"tenants"`
		Title    string `json:"title"`
		DueDate  string `json:"due_date"`
		Amount   int64  `json:"amount"`
		Paid     int64  `json:"paid"`
		Currency string `json:"currency"`
		Days     int    `json:"days"`
	}
	var page struct {
		Late    []due `json:"late"`
		Due     []due `json:"due"`
		Repairs []struct {
			ID           int64  `json:"id"`
			Title        string `json:"title"`
			Priority     string `json:"priority"`
			Status       string `json:"status"`
			ReportedOn   string `json:"reported_on"`
			PropertyName string `json:"property_name"`
			UnitName     string `json:"unit_name"`
		} `json:"repairs"`
	}
	if err := cl.Get(ctx, "/api/v1/today", nil, &page); err != nil {
		return nil, err
	}
	rent := func(x due) RentalsRent {
		var names []string
		for _, t := range x.Tenants {
			if n := strings.TrimSpace(t.Name); n != "" {
				names = append(names, n)
			}
		}
		owed := x.Amount - x.Paid
		if owed < 0 {
			owed = 0
		}
		return RentalsRent{LeaseID: x.LeaseID, Lease: strings.TrimSpace(x.Lease), Place: strings.TrimSpace(x.Place), Tenants: strings.Join(names, ", "), Title: strings.TrimSpace(x.Title),
			DueDate: x.DueDate, Owed: owed, Currency: x.Currency, Days: max(x.Days, 0)}
	}
	t := &RentalsToday{Late: []RentalsRent{}, Due: []RentalsRent{}, Urgent: []RentalsRepair{}, Repairs: len(page.Repairs)}
	for _, x := range page.Late {
		t.Late = append(t.Late, rent(x))
	}
	for _, x := range page.Due {
		if x.DueDate != "" && x.DueDate <= date { // due by today, not the coming days
			t.Due = append(t.Due, rent(x))
		}
	}
	for _, rq := range page.Repairs {
		if rq.Priority != "urgent" {
			continue
		}
		// as Rentals names a place: "Flat 2, Obala 12", or only the property
		pl, unit := strings.TrimSpace(rq.PropertyName), strings.TrimSpace(rq.UnitName)
		if unit != "" && !strings.EqualFold(unit, pl) && !strings.EqualFold(unit, "Whole place") {
			pl = strings.Trim(unit+", "+pl, ", ")
		}
		t.Urgent = append(t.Urgent, RentalsRepair{ID: rq.ID, Title: firstNonEmptyStr(clipWords(strings.TrimSpace(rq.Title), 120), "A repair"), Place: pl,
			Status: strings.ReplaceAll(rq.Status, "_", " "), Reported: rq.ReportedOn})
	}
	return t, nil
}

// rentalsSummary: "Rent in: €650.00 (Flat 2 — Ana Marić); Repair reported:
// Leaking tap (Flat 1) and 2 more". The day's items by title; what needs you
// today is in the card, not the journal line.
func rentalsSummary(d RentalsDay) string {
	var parts []string
	for i, it := range d.Items {
		if i == 3 {
			break
		}
		s := it.Title
		if it.Subtitle != "" {
			s += " (" + it.Subtitle + ")"
		}
		parts = append(parts, s)
	}
	s := strings.Join(parts, "; ")
	if n := len(d.Items) - 3; n > 0 {
		s += fmt.Sprintf(" and %d more", n)
	}
	return s
}

// RentalsNeedsYou: "2 rents late, 1 due today, 1 urgent repair" — or "" when
// nothing needs you (or it is not today).
func RentalsNeedsYou(t *RentalsToday) string {
	if t == nil {
		return ""
	}
	var p []string
	if n := len(t.Late); n > 0 {
		p = append(p, fmt.Sprintf("%d %s late", n, pluralWord(n, "rent", "rents")))
	}
	if n := len(t.Due); n > 0 {
		p = append(p, fmt.Sprintf("%d due today", n))
	}
	if n := len(t.Urgent); n > 0 {
		p = append(p, fmt.Sprintf("%d urgent %s", n, pluralWord(n, "repair", "repairs")))
	}
	return strings.Join(p, ", ")
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
