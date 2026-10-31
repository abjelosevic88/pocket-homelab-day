package sources

import (
	"context"
	"encoding/json"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBirthdaysOfTheDay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v1/birthdays" || q.Get("from") != "2026-11-03" || q.Get("days") != "1" || r.Header.Get("Authorization") != "Bearer person" {
			t.Errorf("unexpected request %s", r.URL)
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{"contact": map[string]any{"id": 12, "fn": "Marko Petrović"}, "kind": "birthday", "date": "2026-11-03", "age": 38},
			{"contact": map[string]any{"id": 13, "fn": "Ana i Ivan"}, "kind": "anniversary", "date": "2026-11-03", "age": 10},
			{"contact": map[string]any{"id": 14, "fn": "Tomorrow"}, "kind": "birthday", "date": "2026-11-04", "age": nil},
		})
	}))
	defer srv.Close()
	got, err := NewBirthdays(pocket.Fixed(srv.URL, "person", time.Second)).Fetch(context.Background(), "2026-11-03", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	act := got.Payload.(Activity)
	if got.Headline != 2 || len(act.Items) != 2 {
		t.Fatalf("want 2 items, got %+v", act.Items)
	}
	if act.Items[0].Title != "🎂 Marko Petrović" || act.Items[0].Note != "turns 38" || act.Items[1].Title != "💍 Ana i Ivan" || act.Items[1].Note != "10 years" {
		t.Fatalf("unexpected items %+v", act.Items)
	}
}
