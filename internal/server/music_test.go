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

// Pocket Music is the default music provider only with Pocket Core; a Maloja
// URL does not change that, and a person who picks Maloja keeps it.
func TestMusicProvider(t *testing.T) {
	envMaloja := store.Source{URL: "http://maloja", Enabled: true}
	cases := []struct {
		src    store.Source
		coreOK bool
		want   string
	}{
		{envMaloja, true, musPocket},
		{envMaloja, false, musMaloja},
		{store.Source{Provider: musMaloja}, true, musMaloja},
		{store.Source{Provider: musPocket}, false, musMaloja},
		{store.Source{Provider: musPocket}, true, musPocket},
	}
	for i, c := range cases {
		if got := musicProvider(c.src, c.coreOK); got != c.want {
			t.Errorf("case %d: %s, want %s", i, got, c.want)
		}
	}
}

// Pocket Music's day view becomes the same payload Maloja gives: songs in
// order with local times, minutes, top artists.
func TestPocketMusicFetch(t *testing.T) {
	var gotTZ string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-10-20" {
			http.NotFound(w, r)
			return
		}
		gotTZ = r.URL.Query().Get("tz")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totals": map[string]any{"plays": 3, "seconds": 600},
			"listens": []map[string]any{
				{"title": "Blue Hour", "artist": "Ana Band", "album": "Night Drive", "played_at": "2026-10-20T18:00:00Z", "duration": 200, "ms_played": 200000},
				{"title": "Neon Rain", "artist": "Ana Band", "album": "Night Drive", "played_at": "2026-10-20T18:04:00Z", "duration": 220},
				{"title": "Iron Will", "artist": "Ben Metal", "album": "Forge", "played_at": "2026-10-20T19:00:00Z"},
			},
		})
	}))
	defer srv.Close()
	conn := func(context.Context) (*pocket.Client, error) { return pocket.New(srv.URL, "tok", 5*time.Second), nil }
	loc := time.FixedZone("CEST", 2*3600)
	res, err := sources.NewPocketMusic(conn).Fetch(context.Background(), "2026-10-20", loc)
	if err != nil {
		t.Fatal(err)
	}
	m := res.Payload.(sources.Music)
	if gotTZ != "120" || m.Listens != 3 || m.First != "20:00" || m.Minutes != (200+220+240)/60 || m.Artists[0].Name != "Ana Band" || res.Summary != "3 listens · mostly Ana Band" {
		t.Fatalf("tz=%s music=%+v summary=%q", gotTZ, m, res.Summary)
	}
}
