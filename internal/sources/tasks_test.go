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

func TestTasksPagesAndLocalDay(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer person" || r.URL.Query().Get("status") != "done" {
			t.Error("missing scoped request")
		}
		stamp := "2026-10-19T23:30:00Z"
		if calls == 2 {
			if r.URL.Query().Get("offset") != "1" {
				t.Error("did not advance cursor")
			}
			stamp = "2026-10-20T23:30:00Z"
		}
		json.NewEncoder(w).Encode(map[string]any{"tasks": []map[string]any{{"id": calls, "title": "task", "completed_at": stamp}}, "total": 2})
	}))
	defer srv.Close()
	got, err := NewTasks(pocket.Fixed(srv.URL, "person", time.Second)).Fetch(context.Background(), "2026-10-20", time.FixedZone("local", 2*3600))
	if err != nil || calls != 2 || got.Headline != 1 {
		t.Fatal(got, err, calls)
	}
}
