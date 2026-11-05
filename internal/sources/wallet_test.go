package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestWalletDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-05" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-05",
			"passes": []map[string]any{
				{"id": 3, "kind": "boarding", "kind_label": "Boarding pass", "name": "Sarajevo → Vienna", "issuer": "Austrian Airlines", "path": "/cards/3", "starts_at": "2026-11-05T06:40", "ends_at": "2026-11-05T08:15", "venue": "SJJ Sarajevo", "destination": "VIE Vienna", "seat": "14C", "gate": "A2", "ref": "OS 784", "holder": "Aleksandar", "trip_name": "Vienna weekend", "trip_id": 2, "has_code": true},
				{"id": 14, "kind": "ticket", "kind_label": "Ticket", "name": "Dubioza kolektiv", "issuer": "Entrio", "path": "/cards/14", "starts_at": "2026-11-05T20:00", "ends_at": "2026-11-06T00:30", "venue": "Zetra Olympic Hall", "ref": "ENT-55120", "has_code": true},
				{"id": 15, "kind": "ticket", "kind_label": "Ticket", "name": "Dune: Part Three", "issuer": "Cinema City", "path": "/cards/15", "starts_at": "2026-11-05T22:45", "seat": "Row H, seat 12", "has_code": false},
				{"id": 16, "kind": "ticket", "kind_label": "Ticket", "name": "Sarajevo → Mostar", "issuer": "ŽFBH", "path": "/cards/16", "starts_at": "2026-11-05", "seat": "32", "has_code": true},
			},
			"url": "https://wallet.example.org/",
		})
	}))
	defer ts.Close()
	w := NewWallet(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := w.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Flight OS 784 Sarajevo → Vienna, seat 14C; Dubioza kolektiv at Zetra Olympic Hall; Dune: Part Three (Cinema City), row H, seat 12 and 1 more"; res.Headline != 4 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(WalletDay)
	if d.URL != "https://wallet.example.org" || len(d.Passes) != 4 {
		t.Fatalf("day %+v", d)
	}
	f := d.Passes[0]
	if f.Time != "06:40" || f.Until != "08:15" || f.Flight != "OS 784" || f.Booking != "" || f.Gate != "A2" || f.Trip != "Vienna weekend" || f.Path != "/cards/3" || f.Holder != "Aleksandar" {
		t.Fatalf("flight %+v", f)
	}
	c := d.Passes[1]
	if c.Time != "20:00" || c.Until != "" || c.Booking != "ENT-55120" || c.Flight != "" {
		t.Fatalf("concert (ends after midnight, so no end time; the reference is a booking) %+v", c)
	}
	if tr := d.Passes[3]; tr.Time != "" || tr.Line != "Ticket: Sarajevo → Mostar, seat 32" {
		t.Fatalf("train (a day, no time) %+v", tr)
	}
}

func TestWalletLines(t *testing.T) {
	for _, c := range []struct {
		p    WalletPass
		want string
	}{
		{WalletPass{Kind: "boarding", Name: "Sarajevo → Vienna"}, "Flight Sarajevo → Vienna"},
		{WalletPass{Kind: "boarding", Name: "Austrian Airlines", Destination: "VIE Vienna", Flight: "OS784"}, "Flight OS784 to VIE Vienna"},
		{WalletPass{Kind: "ticket", Name: "FK Sarajevo – FK Željezničar", Venue: "Asim Ferhatović Hase", Seat: "Sector B 14"}, "FK Sarajevo – FK Željezničar at Asim Ferhatović Hase, sector B 14"},
		{WalletPass{Kind: "ticket", Name: "Zemaljski muzej"}, "Zemaljski muzej"},
		{WalletPass{Kind: "ticket", Name: "Concert at Zetra", Venue: "Zetra"}, "Concert at Zetra"},
		{WalletPass{Kind: "ticket", Issuer: "Centrotrans", Destination: "Mostar"}, "Ticket: Centrotrans to Mostar"},
		{WalletPass{Kind: "ticket"}, "A ticket"},
	} {
		if got := walletLine(c.p); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
	for ref, ok := range map[string]bool{"OS 784": true, "W62841": true, "LH1234A": true, "X7Y8Z9": false, "ENT-55120": false} {
		if flightNo.MatchString(ref) != ok {
			t.Errorf("flight number %q: %v", ref, !ok)
		}
	}
}
