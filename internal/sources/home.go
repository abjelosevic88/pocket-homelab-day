package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Home ----------------

// Home reads what was done on the house that day and the meters read.
type Home struct{ Conn pocket.Conn }

// NewHome creates a fetcher over a connection.
func NewHome(conn pocket.Conn) *Home { return &Home{Conn: conn} }

// Name implements Fetcher.
func (h *Home) Name() string { return "home" }

// Test implements Fetcher.
func (h *Home) Test(ctx context.Context) (string, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Home " + v, nil
}

// HomeWork is one log entry of the day: a repair, a service, a purchase.
type HomeWork struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Thing    string `json:"thing,omitempty"`
	Home     string `json:"home,omitempty"`
	Pro      string `json:"pro,omitempty"`
	Cost     int64  `json:"cost"` // minor units
	Currency string `json:"currency"`
}

// HomeReading is a meter read that day.
type HomeReading struct {
	MeterID int64    `json:"meter_id"`
	Meter   string   `json:"meter"`
	Kind    string   `json:"kind"`
	Unit    string   `json:"unit"`
	Value   float64  `json:"value"`
	Usage   *float64 `json:"usage,omitempty"`
	Days    int      `json:"days,omitempty"`
}

// HomeDay is the day in Pocket Home.
type HomeDay struct {
	Work     []HomeWork    `json:"work"`
	Readings []HomeReading `json:"readings"`
}

// Fetch implements Fetcher.
func (h *Home) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Records []struct {
			ID        int64  `json:"id"`
			Kind      string `json:"kind"`
			Title     string `json:"title"`
			ThingName string `json:"thing_name"`
			HomeName  string `json:"home_name"`
			ProName   string `json:"pro_name"`
			Cost      int64  `json:"cost"`
			Currency  string `json:"currency"`
		} `json:"records"`
		Readings []HomeReading `json:"readings"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := HomeDay{Work: []HomeWork{}, Readings: page.Readings}
	if d.Readings == nil {
		d.Readings = []HomeReading{}
	}
	var parts []string
	for _, r := range page.Records {
		d.Work = append(d.Work, HomeWork{ID: r.ID, Kind: r.Kind, Title: r.Title, Thing: r.ThingName, Home: r.HomeName, Pro: r.ProName, Cost: r.Cost, Currency: r.Currency})
		parts = append(parts, r.Title)
	}
	for _, r := range d.Readings {
		parts = append(parts, "read the "+strings.ToLower(r.Meter)+" meter")
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Work)), Summary: sum}, nil
}
