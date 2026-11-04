package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Renewals ----------------

// Renewals reads what was renewed that day and what ran out.
type Renewals struct{ Conn pocket.Conn }

// NewRenewals creates a fetcher over a connection.
func NewRenewals(conn pocket.Conn) *Renewals { return &Renewals{Conn: conn} }

// Name implements Fetcher.
func (r *Renewals) Name() string { return "renewals" }

// Test implements Fetcher.
func (r *Renewals) Test(ctx context.Context) (string, error) {
	cl, err := r.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Renewals " + v, nil
}

// RenewalDone is one renewal made that day: a passport, an insurance, a domain.
type RenewalDone struct {
	ID         int64  `json:"id"`
	RenewalID  int64  `json:"renewal_id"`
	Title      string `json:"title"`
	OldExpires string `json:"old_expires,omitempty"`
	NewExpires string `json:"new_expires,omitempty"`
	Cost       int64  `json:"cost"` // minor units
	Currency   string `json:"currency"`
	Note       string `json:"note,omitempty"`
	Auto       bool   `json:"auto,omitempty"` // renewed by itself
}

// RenewalEnded is something whose date ran out that day.
type RenewalEnded struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Holder   string `json:"holder,omitempty"`
	Provider string `json:"provider,omitempty"`
	Status   string `json:"status,omitempty"`
	URL      string `json:"url,omitempty"`
}

// RenewalsDay is the day in Pocket Renewals.
type RenewalsDay struct {
	Renewed []RenewalDone  `json:"renewed"`
	Expired []RenewalEnded `json:"expired"`
}

// Fetch implements Fetcher.
func (r *Renewals) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := r.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Expires []struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Kind     string `json:"kind"`
			Holder   string `json:"holder"`
			Provider string `json:"provider"`
			Status   string `json:"status"`
			OpenURL  string `json:"open_url"`
		} `json:"expires"`
		Renewed []struct {
			ID         int64  `json:"id"`
			RenewalID  int64  `json:"renewal_id"`
			Title      string `json:"title"`
			OldExpires string `json:"old_expires"`
			NewExpires string `json:"new_expires"`
			Cost       int64  `json:"cost"`
			Currency   string `json:"currency"`
			Note       string `json:"note"`
			Source     string `json:"source"`
		} `json:"renewed"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := RenewalsDay{Renewed: []RenewalDone{}, Expired: []RenewalEnded{}}
	var parts []string
	for _, h := range page.Renewed {
		d.Renewed = append(d.Renewed, RenewalDone{ID: h.ID, RenewalID: h.RenewalID, Title: h.Title, OldExpires: h.OldExpires, NewExpires: h.NewExpires,
			Cost: h.Cost, Currency: h.Currency, Note: h.Note, Auto: h.Source == "auto"})
		parts = append(parts, renewedLine(h.Title, h.NewExpires, h.Cost, h.Currency))
	}
	for _, e := range page.Expires {
		d.Expired = append(d.Expired, RenewalEnded{ID: e.ID, Title: e.Title, Kind: e.Kind, Holder: e.Holder, Provider: e.Provider, Status: e.Status, URL: e.OpenURL})
		line := "Ran out: " + e.Title
		if e.Holder != "" {
			line += " (" + e.Holder + ")"
		}
		parts = append(parts, line)
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Renewed)), Summary: sum}, nil
}

// renewedLine: "Renewed: Car insurance — now until 5 Jan 2027 (€412.00)".
func renewedLine(title, until string, cost int64, cur string) string {
	s := "Renewed: " + title
	if t, err := time.Parse("2006-01-02", until); err == nil {
		s += " — now until " + t.Format("2 Jan 2006")
	}
	if cost > 0 {
		s += " (" + minorAmount(cost, cur) + ")"
	}
	return s
}

// minorAmount writes an amount in minor units with its currency: €412.00, 12.50 CHF.
func minorAmount(v int64, cur string) string {
	n := fmt.Sprintf("%d.%02d", v/100, v%100)
	switch strings.ToUpper(cur) {
	case "":
		return n
	case "EUR":
		return "€" + n
	case "USD":
		return "$" + n
	case "GBP":
		return "£" + n
	}
	return n + " " + strings.ToUpper(cur)
}
