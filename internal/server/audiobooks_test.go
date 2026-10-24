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

// Pocket Audiobooks is the default audiobooks provider only with Pocket Core;
// an Audiobookshelf URL does not change that, and a person who picks
// Audiobookshelf keeps it.
func TestAudiobooksProvider(t *testing.T) {
	envABS := store.Source{URL: "http://abs", Enabled: true}
	cases := []struct {
		src    store.Source
		coreOK bool
		want   string
	}{
		{envABS, true, abPocket},
		{envABS, false, abABS},
		{store.Source{Provider: abABS}, true, abABS},
		{store.Source{Provider: abPocket}, false, abABS},
		{store.Source{Provider: abPocket}, true, abPocket},
	}
	for i, c := range cases {
		if got := audiobooksProvider(c.src, c.coreOK); got != c.want {
			t.Errorf("case %d: %s, want %s", i, got, c.want)
		}
	}
}

// Pocket Audiobooks' day view becomes the same payload Audiobookshelf's
// sessions give: one item per book with minutes, local start, place and length.
func TestPocketAudiobooksFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-10-20" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"seconds": 3900,
			"books": []map[string]any{
				{"item_id": "b1", "title": "Dracula", "author": "Bram Stoker", "seconds": 3600, "duration": 55440, "to": 9000, "first": "2026-10-20T18:00:00Z"},
				{"item_id": "b2", "title": "The Time Machine", "author": "H. G. Wells", "seconds": 270, "duration": 12240, "to": 12240, "first": "2026-10-20T07:30:00Z", "finished": true},
				{"item_id": "b3", "title": "Scrub", "author": "X", "seconds": 30, "first": "2026-10-20T09:00:00Z"},
			},
		})
	}))
	defer srv.Close()
	conn := func(context.Context) (*pocket.Client, error) { return pocket.New(srv.URL, "tok", 5*time.Second), nil }
	loc := time.FixedZone("CEST", 2*3600)
	res, err := sources.NewPocketAudiobooks(conn).Fetch(context.Background(), "2026-10-20", loc)
	if err != nil {
		t.Fatal(err)
	}
	a := res.Payload.(sources.Activity)
	// in the order of the day: the morning's finished book first
	if len(a.Items) != 2 || a.Items[1].Title != "Dracula" || a.Items[1].Minutes != 60 || a.Items[1].Time != "20:00" || a.Items[1].Length != 55440 || !a.Items[0].Done || a.Items[0].Time != "09:30" {
		t.Fatalf("activity=%+v", a)
	}
}
