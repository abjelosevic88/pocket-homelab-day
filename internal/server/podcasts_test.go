package server

import (
	"context"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPodcastReconciliationRefreshesFrozenDates(t *testing.T) {
	e := newLinkEnv(t, false)
	ctx := context.Background()
	revision := "first"
	seconds := 60
	fetches := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/listens/dates" {
			writeJSON(w, 200, map[string]any{"dates": []map[string]string{{"date": day0, "updated_at": revision}}})
			return
		}
		if r.URL.Path == "/api/v1/listens" {
			fetches++
			writeJSON(w, 200, map[string]any{"listens": []map[string]any{{"episode_id": 1, "title": "Episode", "podcast": "Show", "seconds": seconds}}})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()
	st, _ := e.st.GetSettings(ctx, e.owner.ID)
	st.Sources["podcasts"] = store.Source{Enabled: true, Mode: "direct", URL: ts.URL, Key: "test"}
	if err := e.st.SaveSettings(ctx, e.owner.ID, st); err != nil {
		t.Fatal(err)
	}
	if err := e.st.PutFact(ctx, e.owner.ID, &store.Fact{Date: day0, Source: "pinepods", Headline: 12, Summary: "legacy"}); err != nil {
		t.Fatal(err)
	}
	e.s.syncPodcastHistory(ctx, e.owner)
	if f := e.facts(e.owner, day0)["podcasts"]; f.Headline != 1 {
		t.Fatalf("first: %+v", f)
	}
	e.s.syncPodcastHistory(ctx, e.owner)
	if fetches != 1 {
		t.Fatal("unchanged day fetched again")
	}
	revision = "second"
	seconds = 180
	e.s.syncPodcastHistory(ctx, e.owner)
	facts := e.facts(e.owner, day0)
	if facts["podcasts"].Headline != 3 || facts["pinepods"].Headline != 12 || fetches != 2 {
		t.Fatalf("late upload or legacy preservation: %+v", facts)
	}
	src := st.Sources["podcasts"]
	src.Mode = "off"
	st.Sources["podcasts"] = src
	_ = e.st.SaveSettings(ctx, e.owner.ID, st)
	revision = "third"
	e.s.syncPodcastHistory(ctx, e.owner)
	if fetches != 2 {
		t.Fatal("disabled source fetched")
	}
}
