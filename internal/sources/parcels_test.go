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

func TestParcelsDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-05" {
			http.NotFound(w, r)
			return
		}
		three := 3
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-05",
			"delivered": []map[string]any{
				{"id": 1, "title": "Running shoes", "shop": "Decathlon", "carrier_name": "GLS", "direction": "in", "delivered_at": "2026-11-05T12:20:00Z", "days": three, "price": 7999, "currency": "EUR"},
				{"id": 2, "title": "Birthday present", "shop": "Ana", "direction": "out", "contact_name": "Ana Petrović", "delivered_at": "2026-11-05T09:00:00Z"},
			},
			"sent":     []map[string]any{{"id": 3, "title": "Book for Marko", "direction": "out", "ordered_on": "2026-11-05"}},
			"added":    []map[string]any{{"id": 4, "title": "USB-C hub", "shop": "AliExpress", "direction": "in", "ordered_on": "2026-11-05"}, {"id": 5, "title": "Old order", "shop": "Amazon.de", "direction": "in", "ordered_on": "2026-10-01"}},
			"expected": []map[string]any{{"id": 6, "title": "Coffee grinder", "shop": "Amazon.de", "direction": "in"}},
			"steps": []map[string]any{
				{"parcel_id": 7, "title": "Phone case", "at": "2026-11-05T08:10:00Z", "status": "pickup", "description": "Ready for pick-up", "location": "Pošta Sarajevo"},
				{"parcel_id": 8, "title": "LED strip", "at": "2026-11-05T10:00:00Z", "status": "exception", "description": "Delivery attempt failed — nobody home"},
				{"parcel_id": 8, "title": "LED strip", "at": "2026-11-05T11:00:00Z", "status": "exception", "description": "Second attempt failed"},
				{"parcel_id": 1, "title": "Running shoes", "at": "2026-11-05T07:00:00Z", "status": "out_for_delivery", "description": "Out for delivery"},
			},
		})
	}))
	defer ts.Close()
	p := NewParcels(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	loc := time.FixedZone("CET", 3600)
	res, err := p.Fetch(context.Background(), "2026-11-05", loc)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Delivered: Running shoes (Decathlon); Arrived: Birthday present (Ana Petrović); Sent: Book for Marko and 3 more"; res.Headline != 2 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(ParcelsDay)
	if len(d.Delivered) != 2 || d.Delivered[0].Time != "13:20" || d.Delivered[0].Days == nil || *d.Delivered[0].Days != 3 || !d.Delivered[1].Out {
		t.Fatalf("delivered %+v", d.Delivered)
	}
	if len(d.Ordered) != 1 || d.Ordered[0].Line != "Ordered: USB-C hub (AliExpress)" {
		t.Fatalf("ordered %+v (an order from October added today is not news)", d.Ordered)
	}
	if len(d.Steps) != 2 || d.Steps[0].Line != "Ready to pick up: Phone case (Pošta Sarajevo)" || d.Steps[0].Time != "09:10" ||
		d.Steps[1].Line != "Problem with LED strip: delivery attempt failed — nobody home" {
		t.Fatalf("steps %+v (one line per parcel and status; out for delivery left out)", d.Steps)
	}
	if len(d.Expected) != 1 || d.Expected[0].Line != "Expected: Coffee grinder (Amazon.de)" {
		t.Fatalf("expected %+v", d.Expected)
	}
}
