package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// Pocket Watch is the default watched provider only with Pocket Core; a
// Jellystat URL does not change that, and a person who picks Jellystat keeps it.
func TestWatchProvider(t *testing.T) {
	envJellystat := store.Source{URL: "http://jellystat", Enabled: true}
	cases := []struct {
		src    store.Source
		coreOK bool
		want   string
	}{
		{envJellystat, true, watchPocket},
		{envJellystat, false, watchJellystat},
		{store.Source{Provider: watchJellystat}, true, watchJellystat},
		{store.Source{Provider: watchPocket}, false, watchJellystat},
		{store.Source{Provider: watchPocket}, true, watchPocket},
	}
	for i, c := range cases {
		if got := watchProvider(c.src, c.coreOK); got != c.want {
			t.Errorf("case %d: %s, want %s", i, got, c.want)
		}
	}
}

// Pocket Watch's history becomes the same payload Jellystat gives: the
// day's plays in local time, oldest first, short looks and bulk marks left out.
func TestPocketWatchFetch(t *testing.T) {
	var gotFrom, gotTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/history" {
			http.NotFound(w, r)
			return
		}
		gotFrom, gotTo = r.URL.Query().Get("from"), r.URL.Query().Get("to")
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 6, "items": []map[string]any{ // newest first
			{"item_id": "late", "kind": "movie", "title": "Next day film", "watched_at": "2026-10-20T22:30:00Z", "seconds": 6000},
			{"item_id": "e2", "kind": "episode", "title": "Half Loop", "series": "Severance", "watched_at": "2026-10-20T20:00:00Z", "seconds": 2400, "device": "Chrome on Mac"},
			{"item_id": "e1", "kind": "episode", "title": "Good News About Hell", "series": "Severance", "watched_at": "2026-10-20T19:00:00Z", "seconds": 3000},
			{"item_id": "peek", "kind": "movie", "title": "Trailer", "watched_at": "2026-10-20T18:00:00Z", "seconds": 60},
			{"item_id": "bulk", "kind": "movie", "title": "Marked", "watched_at": "2026-10-20T17:00:00Z", "seconds": 0},
			{"item_id": "m1", "kind": "movie", "title": "Dune", "watched_at": "2026-10-20T08:00:00Z", "seconds": 9300},
			{"item_id": "early", "kind": "movie", "title": "Day before", "watched_at": "2026-10-19T21:00:00Z", "seconds": 5000},
		}})
	}))
	defer srv.Close()
	conn := func(context.Context) (*pocket.Client, error) { return pocket.New(srv.URL, "tok", 5*time.Second), nil }
	loc := time.FixedZone("CEST", 2*3600)
	res, err := sources.NewPocketWatch(conn).Fetch(context.Background(), "2026-10-20", loc)
	if err != nil {
		t.Fatal(err)
	}
	if gotFrom != "2026-10-19" || gotTo != "2026-10-22" {
		t.Errorf("asked for %s..%s", gotFrom, gotTo)
	}
	w := res.Payload.(sources.Watched)
	if w.Provider != "watch" || w.Count != 3 || w.Movies != 1 || w.Episodes != 2 || w.Minutes != 155+50+40 || res.Headline != 245 {
		t.Fatalf("watched: %+v", w)
	}
	dune, ep1, ep2 := w.Items[0], w.Items[1], w.Items[2]
	if dune.Title != "Dune" || dune.Kind != "movie" || dune.Time != "10:00" || dune.Minutes != 155 {
		t.Errorf("film: %+v", dune)
	}
	if ep1.Series != "Severance" || ep1.Kind != "episode" || ep1.Time != "21:00" || ep2.Client != "Chrome on Mac" {
		t.Errorf("episodes: %+v %+v", ep1, ep2)
	}
	if res.Summary != "2 episodes, 1 film · 4h05" {
		t.Errorf("summary = %q", res.Summary)
	}
}
