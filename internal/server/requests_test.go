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

// Pocket Requests is the default requests provider only with Pocket Core; a
// Jellyseerr URL does not change that, and a person who picks Jellyseerr keeps it.
func TestRequestsProvider(t *testing.T) {
	envSeerr := store.Source{URL: "http://jellyseerr", Enabled: true}
	cases := []struct {
		src    store.Source
		coreOK bool
		want   string
	}{
		{envSeerr, true, reqPocket},
		{envSeerr, false, reqSeerr},
		{store.Source{Provider: reqSeerr}, true, reqSeerr},
		{store.Source{Provider: reqPocket}, false, reqSeerr},
		{store.Source{Provider: reqPocket}, true, reqPocket},
	}
	for i, c := range cases {
		if got := requestsProvider(c.src, c.coreOK); got != c.want {
			t.Errorf("case %d: %s, want %s", i, got, c.want)
		}
	}
}

// Pocket Requests' day view becomes the same payload Jellyseerr gives:
// requests and arrivals with local times, counted by kind, albums included.
func TestPocketRequestsFetch(t *testing.T) {
	var gotTZ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-10-20" {
			http.NotFound(w, r)
			return
		}
		gotTZ = r.URL.Query().Get("tz")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-10-20",
			"requested": []map[string]any{
				{"id": 4, "kind": "series", "title": "Silo", "status": "approved", "created_at": "2026-10-20T18:00:00Z", "open_url": "https://get.x/requests/4", "poster": "https://img/silo.jpg"},
				{"id": 5, "kind": "album", "title": "In Rainbows", "subtitle": "Radiohead", "status": "declined", "created_at": "2026-10-20T19:30:00Z"},
			},
			"arrived": []map[string]any{
				{"id": 2, "kind": "movie", "title": "Barbarian", "status": "approved", "created_at": "2026-10-18T10:00:00Z", "available_at": "2026-10-20T07:15:00Z"},
			},
		})
	}))
	defer srv.Close()
	conn := func(context.Context) (*pocket.Client, error) { return pocket.New(srv.URL, "tok", 5*time.Second), nil }
	loc := time.FixedZone("CEST", 2*3600)
	res, err := sources.NewPocketRequests(conn).Fetch(context.Background(), "2026-10-20", loc)
	if err != nil {
		t.Fatal(err)
	}
	if gotTZ != "120" {
		t.Errorf("tz sent = %q", gotTZ)
	}
	act := res.Payload.(sources.Activity)
	if act.Count != 3 || act.Done != 1 || res.Headline != 3 {
		t.Fatalf("activity: %+v", act)
	}
	first, silo, album := act.Items[0], act.Items[1], act.Items[2]
	if first.Title != "Barbarian" || first.Time != "09:15" || !first.Done || first.Sub != "now available" || first.Group != "film" {
		t.Errorf("arrival: %+v", first)
	}
	if silo.Time != "20:00" || silo.Sub != "requested series" || silo.URL != "https://get.x/requests/4" || silo.Image != "https://img/silo.jpg" {
		t.Errorf("series request: %+v", silo)
	}
	if album.Sub != "requested album · Radiohead · declined" || album.Group != "album" {
		t.Errorf("album request: %+v", album)
	}
	if res.Summary != "2 requests · 1 now available · Barbarian" {
		t.Errorf("summary = %q", res.Summary)
	}
}
