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

func TestWealthDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-04" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
			{"type": "activity", "id": 1, "kind": "buy", "title": "Bought 2 World fund", "amount": 25000, "currency": "EUR", "asset_id": 4},
			{"type": "valuation", "id": 2, "title": "Pension updated", "amount": 1960000, "currency": "EUR", "asset_id": 6},
		}})
	}))
	defer ts.Close()
	w := NewWealth(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := w.Fetch(context.Background(), "2026-11-04", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if res.Headline != 1 || res.Summary != "Bought 2 World fund (€250.00); Pension updated" {
		t.Fatalf("got %v %q", res.Headline, res.Summary)
	}
}
