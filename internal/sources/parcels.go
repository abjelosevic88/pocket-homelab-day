package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Parcels ----------------

// Parcels reads what happened with deliveries that day: what arrived, what
// was sent, what was ordered, what waits at a pick-up point and what went
// wrong. Parcels only expected that day are kept for the card, not the
// summary: the journal is what happened.
type Parcels struct{ Conn pocket.Conn }

// NewParcels creates a fetcher over a connection.
func NewParcels(conn pocket.Conn) *Parcels { return &Parcels{Conn: conn} }

// Name implements Fetcher.
func (p *Parcels) Name() string { return "parcels" }

// Test implements Fetcher.
func (p *Parcels) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Parcels " + v, nil
}

// ParcelsItem is one parcel in the day.
type ParcelsItem struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Shop     string `json:"shop,omitempty"`
	Carrier  string `json:"carrier,omitempty"`
	Out      bool   `json:"out,omitempty"`  // a parcel the person sent
	Days     *int   `json:"days,omitempty"` // how long it took (delivered)
	Price    *int64 `json:"price,omitempty"`
	Currency string `json:"currency,omitempty"`
	Time     string `json:"time,omitempty"` // HH:MM local, "" when not known
	Line     string `json:"line"`           // "Delivered: Running shoes (Decathlon)"
}

// ParcelsStep is a step worth a line: ready to pick up, a problem, sent back.
type ParcelsStep struct {
	ParcelID int64  `json:"parcel_id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Text     string `json:"text,omitempty"`
	Location string `json:"location,omitempty"`
	Time     string `json:"time,omitempty"`
	Line     string `json:"line"`
}

// ParcelsDay is the day in Pocket Parcels.
type ParcelsDay struct {
	Delivered []ParcelsItem `json:"delivered"`
	Sent      []ParcelsItem `json:"sent"`
	Ordered   []ParcelsItem `json:"ordered"`
	Steps     []ParcelsStep `json:"steps"`
	Expected  []ParcelsItem `json:"expected"`
}

type parcelRow struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Shop        string `json:"shop"`
	CarrierName string `json:"carrier_name"`
	Direction   string `json:"direction"`
	ContactName string `json:"contact_name"`
	OrderedOn   string `json:"ordered_on"`
	DeliveredAt string `json:"delivered_at"`
	Days        *int   `json:"days"`
	Price       *int64 `json:"price"`
	Currency    string `json:"currency"`
}

// Fetch implements Fetcher.
func (p *Parcels) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Delivered []parcelRow `json:"delivered"`
		Sent      []parcelRow `json:"sent"`
		Added     []parcelRow `json:"added"`
		Expected  []parcelRow `json:"expected"`
		Steps     []struct {
			ParcelID    int64  `json:"parcel_id"`
			Title       string `json:"title"`
			At          string `json:"at"`
			Status      string `json:"status"`
			Description string `json:"description"`
			Location    string `json:"location"`
		} `json:"steps"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	if loc == nil {
		loc = time.Local
	}
	item := func(r parcelRow, line string) ParcelsItem {
		it := ParcelsItem{ID: r.ID, Title: r.Title, Shop: r.Shop, Carrier: r.CarrierName, Out: r.Direction == "out", Price: r.Price, Currency: r.Currency, Line: line}
		if r.DeliveredAt != "" {
			it.Time = clockIn(r.DeliveredAt, loc)
		}
		return it
	}
	d := ParcelsDay{Delivered: []ParcelsItem{}, Sent: []ParcelsItem{}, Ordered: []ParcelsItem{}, Steps: []ParcelsStep{}, Expected: []ParcelsItem{}}
	for _, r := range page.Delivered {
		it := item(r, parcelLine("Delivered", r))
		if r.Direction == "out" {
			it.Line = "Arrived: " + r.Title
			if r.ContactName != "" && !strings.Contains(r.Title, r.ContactName) {
				it.Line += " (" + r.ContactName + ")"
			}
		}
		it.Days = r.Days
		d.Delivered = append(d.Delivered, it)
	}
	for _, r := range page.Sent {
		it := item(r, "Sent: "+r.Title)
		it.Time = ""
		d.Sent = append(d.Sent, it)
	}
	for _, r := range page.Added {
		// a parcel added later than it was ordered is not news of that day
		if r.OrderedOn != "" && r.OrderedOn != date {
			continue
		}
		it := item(r, parcelLine("Ordered", r))
		it.Time = ""
		d.Ordered = append(d.Ordered, it)
	}
	for _, r := range page.Expected {
		it := item(r, parcelLine("Expected", r))
		it.Time = ""
		d.Expected = append(d.Expected, it)
	}
	seen := map[string]bool{}
	for _, s := range page.Steps {
		var line string
		switch s.Status {
		case "pickup":
			line = "Ready to pick up: " + s.Title
			if s.Location != "" {
				line += " (" + s.Location + ")"
			}
		case "exception":
			line = "Problem with " + s.Title
			if s.Description != "" {
				line += ": " + lowerFirst(strings.TrimSpace(s.Description))
			}
		case "returned":
			line = "Sent back: " + s.Title
		default:
			continue
		}
		key := fmt.Sprintf("%d|%s", s.ParcelID, s.Status)
		if seen[key] {
			continue
		}
		seen[key] = true
		d.Steps = append(d.Steps, ParcelsStep{ParcelID: s.ParcelID, Title: s.Title, Status: s.Status, Text: s.Description, Location: s.Location, Time: clockIn(s.At, loc), Line: line})
	}
	var parts []string
	for _, it := range d.Delivered {
		parts = append(parts, it.Line)
	}
	for _, it := range d.Sent {
		parts = append(parts, it.Line)
	}
	for _, s := range d.Steps {
		parts = append(parts, s.Line)
	}
	for _, it := range d.Ordered {
		parts = append(parts, it.Line)
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Delivered)), Summary: sum}, nil
}

// parcelLine is "<what>: <title> (<shop>)", the shop left out when the
// title already says it.
func parcelLine(what string, r parcelRow) string {
	line := what + ": " + r.Title
	if r.Shop != "" && !strings.Contains(strings.ToLower(r.Title), strings.ToLower(r.Shop)) {
		line += " (" + r.Shop + ")"
	}
	return line
}

// clockIn is the HH:MM of an RFC 3339 time in loc, "" when it is not one.
func clockIn(at string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, at)
	}
	if err != nil {
		return ""
	}
	return t.In(loc).Format("15:04")
}
