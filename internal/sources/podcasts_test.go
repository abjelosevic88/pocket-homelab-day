package sources

import (
	"context"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPodcastsUsesDailySecondsAndKnownTimes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test" || r.URL.Path != "/api/v1/listens" || r.URL.Query().Get("date") != "2026-10-21" {
			t.Errorf("request: %v", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"listens":[{"episode_id":1,"title":"First","podcast":"Show","seconds":90,"duration":3600,"first_at":"2026-10-21T05:15:00Z","last_at":"2026-10-21T05:16:30Z"},{"episode_id":2,"title":"Legacy","podcast":"Show","seconds":30,"duration":7200},{"episode_id":3,"title":"Seek only","seconds":0,"duration":1000}]}`))
	}))
	defer ts.Close()
	loc, err := time.LoadLocation("Europe/Sarajevo")
	if err != nil {
		t.Fatal(err)
	}
	p := NewPodcasts(pocket.Fixed(ts.URL, "test", time.Second))
	res, err := p.Fetch(context.Background(), "2026-10-21", loc)
	if err != nil {
		t.Fatal(err)
	}
	a := res.Payload.(Activity)
	if a.Count != 2 || a.Minutes != 2 || res.Headline != 2 || a.Items[0].Time != "07:15" || a.Items[0].End != "07:16" || a.Items[1].Time != "" {
		t.Fatalf("activity: %+v", a)
	}
}
