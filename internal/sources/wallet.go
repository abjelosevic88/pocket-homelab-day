package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Wallet ----------------

// Wallet reads the day in Pocket Wallet: the tickets and boarding passes for
// it — the concert, the match, the film, the flight — the past ones too, so a
// day's journal keeps what the person went to.
type Wallet struct{ Conn pocket.Conn }

// NewWallet creates a fetcher over a connection.
func NewWallet(conn pocket.Conn) *Wallet { return &Wallet{Conn: conn} }

// Name implements Fetcher.
func (w *Wallet) Name() string { return "wallet" }

// Test implements Fetcher.
func (w *Wallet) Test(ctx context.Context) (string, error) {
	cl, err := w.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Wallet " + v, nil
}

// WalletPass is a ticket or boarding pass for the day.
type WalletPass struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"` // ticket | boarding
	Name        string `json:"name"`
	Issuer      string `json:"issuer,omitempty"`
	Path        string `json:"path"`            // "/cards/14", in Pocket Wallet
	Time        string `json:"time,omitempty"`  // HH:MM it starts
	Until       string `json:"until,omitempty"` // HH:MM it ends, when the same day
	Venue       string `json:"venue,omitempty"`
	Destination string `json:"destination,omitempty"`
	Seat        string `json:"seat,omitempty"`
	Gate        string `json:"gate,omitempty"`
	Flight      string `json:"flight,omitempty"`  // "OS 784"
	Booking     string `json:"booking,omitempty"` // a booking reference that is not a flight number
	Holder      string `json:"holder,omitempty"`
	Trip        string `json:"trip,omitempty"`
	Line        string `json:"line"` // "Flight OS 784 Sarajevo → Vienna, seat 14C"
}

// WalletDay is the day in Pocket Wallet.
type WalletDay struct {
	Passes []WalletPass `json:"passes"`
	URL    string       `json:"url,omitempty"` // Pocket Wallet's address, for the links back
}

// Fetch implements Fetcher.
func (w *Wallet) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := w.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Passes []struct {
			ID          int64  `json:"id"`
			Kind        string `json:"kind"`
			Name        string `json:"name"`
			Issuer      string `json:"issuer"`
			Path        string `json:"path"`
			StartsAt    string `json:"starts_at"`
			EndsAt      string `json:"ends_at"`
			Venue       string `json:"venue"`
			Destination string `json:"destination"`
			Seat        string `json:"seat"`
			Gate        string `json:"gate"`
			Ref         string `json:"ref"`
			Holder      string `json:"holder"`
			TripName    string `json:"trip_name"`
		} `json:"passes"`
		URL string `json:"url"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := WalletDay{Passes: []WalletPass{}, URL: strings.TrimRight(strings.TrimSpace(page.URL), "/")}
	var parts []string
	for _, r := range page.Passes {
		p := WalletPass{ID: r.ID, Kind: r.Kind, Name: strings.TrimSpace(r.Name), Issuer: strings.TrimSpace(r.Issuer), Path: r.Path,
			Time: passClock(r.StartsAt, ""), Until: passClock(r.EndsAt, date), Venue: strings.TrimSpace(r.Venue), Destination: strings.TrimSpace(r.Destination),
			Seat: strings.TrimSpace(r.Seat), Gate: strings.TrimSpace(r.Gate), Holder: strings.TrimSpace(r.Holder), Trip: strings.TrimSpace(r.TripName)}
		if p.Path == "" {
			p.Path = fmt.Sprintf("/cards/%d", r.ID)
		}
		if ref := strings.TrimSpace(r.Ref); ref != "" {
			if r.Kind == "boarding" && flightNo.MatchString(ref) {
				p.Flight = ref
			} else {
				p.Booking = ref
			}
		}
		p.Line = walletLine(p)
		d.Passes = append(d.Passes, p)
		parts = append(parts, p.Line)
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Passes)), Summary: sum}, nil
}

// flightNo is a flight number: "OS 784", "W62841", "LH1234A".
var flightNo = regexp.MustCompile(`^[A-Z0-9]{2}[A-Z]? ?\d{1,4}[A-Z]?$`)

// walletLine says a pass in plain words: "Flight OS 784 Sarajevo → Vienna,
// seat 14C", "Dubioza kolektiv at Zetra Olympic Hall", "Ticket:
// Sarajevo → Mostar, seat 32" (a train or a bus).
func walletLine(p WalletPass) string {
	name := p.Name
	route := strings.Contains(name, "→")
	var s string
	switch {
	case p.Kind == "boarding":
		s = "Flight"
		if p.Flight != "" {
			s += " " + p.Flight
		}
		switch {
		case name != "" && (route || p.Destination == ""):
			s += " " + name
		case p.Destination != "":
			s += " to " + p.Destination
		}
	case route || (p.Destination != "" && p.Venue == ""):
		// a ticket from one place to another: a train, a bus, a ferry
		s = "Ticket: " + firstNonEmptyStr(name, p.Issuer)
		if !route && p.Destination != "" {
			s = strings.TrimSpace(s) + " to " + p.Destination
		}
	default:
		s = firstNonEmptyStr(name, p.Issuer, "A ticket")
		if p.Venue != "" && !strings.Contains(strings.ToLower(s), strings.ToLower(p.Venue)) {
			s += " at " + p.Venue
		} else if p.Issuer != "" && name != "" && !strings.EqualFold(p.Issuer, name) {
			s += " (" + p.Issuer + ")"
		}
	}
	if p.Seat != "" {
		if seatWord.MatchString(p.Seat) {
			s += ", " + strings.ToLower(p.Seat[:1]) + p.Seat[1:]
		} else {
			s += ", seat " + p.Seat
		}
	}
	return s
}

// seatWord: the seat already says what it is ("Row 5, seat 12", "Block C").
var seatWord = regexp.MustCompile(`(?i)^(seat|row|block|sector|stand|tribune|red|sjedi|mjesto|platz|reihe)\b`)

// passClock is the HH:MM of a pass's time ("2026-11-05T19:30"); with day
// set, only when the time is on that day.
func passClock(t, day string) string {
	t = strings.TrimSpace(t)
	if len(t) < 16 || t[10] != 'T' || (day != "" && t[:10] != day) {
		return ""
	}
	return clockOf(t[11:])
}
