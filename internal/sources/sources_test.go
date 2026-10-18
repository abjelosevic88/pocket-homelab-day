package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMalojaKeepsOnlyTheLocalDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	day := time.Date(2026, 10, 17, 0, 0, 0, 0, loc)
	ts := func(h int) int64 { return day.Add(time.Duration(h) * time.Hour).Unix() }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// newest first, one scrobble the day before and one the day after
		list := []map[string]any{
			{"time": ts(25), "track": map[string]any{"artists": []string{"Late"}, "title": "Next day"}},
			{"time": ts(22), "track": map[string]any{"artists": []string{"Bonobo"}, "title": "Kerala", "length": 240}},
			{"time": ts(9), "track": map[string]any{"artists": []string{"Bonobo"}, "title": "Cirrus"}},
			{"time": ts(-1), "track": map[string]any{"artists": []string{"Early"}, "title": "Day before"}},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "list": list, "pagination": map[string]any{"next_page": nil}})
	}))
	defer srv.Close()
	res, err := NewMaloja(srv.URL).Fetch(context.Background(), "2026-10-17", loc)
	if err != nil {
		t.Fatal(err)
	}
	m := res.Payload.(Music)
	if m.Listens != 2 || m.Items[0].Title != "Cirrus" || m.Items[0].Time != "09:00" || m.Artists[0].Name != "Bonobo" {
		t.Fatalf("music = %+v", m)
	}
	if res.Summary != "2 listens · mostly Bonobo" {
		t.Fatalf("summary = %q", res.Summary)
	}
}

func TestJellystatStopsAtDayStartAndMerges(t *testing.T) {
	loc := time.UTC
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-token") != "k" {
			w.WriteHeader(401)
			return
		}
		series := "Severance"
		_ = json.NewEncoder(w).Encode(map[string]any{"current_page": 1, "pages": 1, "results": []map[string]any{
			{"NowPlayingItemId": "a", "NowPlayingItemName": "Ep 2", "SeriesName": series, "PlaybackDuration": 1500, "ActivityDateInserted": "2026-10-17T21:30:00Z"},
			{"NowPlayingItemId": "a", "NowPlayingItemName": "Ep 2", "SeriesName": series, "PlaybackDuration": 1200, "ActivityDateInserted": "2026-10-17T21:00:00Z"},
			{"NowPlayingItemId": "m", "NowPlayingItemName": "Heat", "PlaybackDuration": 5, "ActivityDateInserted": "2026-10-17T12:00:00Z"},
			{"NowPlayingItemId": "z", "NowPlayingItemName": "Old", "PlaybackDuration": 3000, "ActivityDateInserted": "2026-10-16T20:00:00Z"},
		}})
	}))
	defer srv.Close()
	res, err := NewJellystat(srv.URL, "k", "").Fetch(context.Background(), "2026-10-17", loc)
	if err != nil {
		t.Fatal(err)
	}
	w := res.Payload.(Watched)
	if w.Count != 1 || w.Episodes != 1 || w.Minutes != 45 || w.Items[0].Plays != 2 || w.Items[0].Time != "21:00" {
		t.Fatalf("watched = %+v", w)
	}
}

func TestConditionFromCode(t *testing.T) {
	for code, want := range map[int]string{0: "clear", 2: "partly", 3: "cloudy", 45: "fog", 61: "rain", 81: "rain", 73: "snow", 95: "storm", -1: ""} {
		if got := ConditionFromCode(code); got != want {
			t.Errorf("code %d = %q, want %q", code, got, want)
		}
	}
}

func TestTopCounts(t *testing.T) {
	got := TopCounts(map[string]int{"b": 2, "a": 2, "c": 5, "d": 1}, 3)
	if len(got) != 3 || got[0].Name != "c" || got[1].Name != "a" || got[2].Name != "b" {
		t.Fatalf("top = %+v", got)
	}
}
