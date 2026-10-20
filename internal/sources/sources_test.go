package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestMalojaKeepsOnlyTheLocalDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
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
			{"NowPlayingItemId": "m", "NowPlayingItemName": "Heat", "PlaybackDuration": 90, "ActivityDateInserted": "2026-10-17T12:00:00Z"},
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

func TestHoursMinutes(t *testing.T) {
	if HoursMinutes(45) != "45 min" || HoursMinutes(125) != "2h05" {
		t.Fatal(HoursMinutes(45), HoursMinutes(125))
	}
}

func TestActivityMoneyPerCurrency(t *testing.T) {
	a := Activity{Items: []ActivityItem{{Title: "a", Amount: 9200, Currency: "EUR"}, {Title: "b", Amount: 400, Currency: "USD"}, {Title: "c", Amount: 8400, Currency: "EUR"}}}
	a.Finish()
	if a.Cur != "EUR" || a.Amount != 17600 {
		t.Fatalf("got %s %.2f, want EUR 17600", a.Cur, a.Amount)
	}
}

// Pocket Documents: the list filtered by added date (whole UTC dates), only
// the local day kept, imports from Paperless left out, links relative.
func TestDocumentsAddedOnTheLocalDay(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Berlin")
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pd_test" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/api/v1/documents" {
			w.WriteHeader(404)
			return
		}
		query = r.URL.RawQuery
		q := r.URL.Query()
		all := []map[string]any{
			{"id": 1, "title": "Day before", "added_at": "2026-10-16T21:30:00Z", "source": "web"},
			{"id": 2, "title": "Water bill", "added_at": "2026-10-16T22:30:00.123456Z", "source": "mail", "correspondent_name": "ViK", "document_type_name": "Invoice", "tags": []int{3}},
			{"id": 3, "title": "", "original_name": "scan-0042.pdf", "added_at": "2026-10-17T09:15:00Z", "source": "consume"},
			{"id": 4, "title": "Old lease", "added_at": "2026-10-17T10:00:00Z", "source": "paperless", "document_type_name": "Contract"},
			{"id": 5, "title": "Payslip", "added_at": "2026-10-17T15:00:00Z", "source": "web", "document_type_name": "Invoice"},
			{"id": 6, "title": "Next day", "added_at": "2026-10-17T22:00:00Z", "source": "web"},
		}
		out := []map[string]any{}
		for _, d := range all { // the API's filter: added_at between whole UTC dates
			at := d["added_at"].(string)
			if at >= q.Get("added_from") && at <= q.Get("added_to")+"T23:59:59.999Z" {
				out = append(out, d)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out, "count": len(out)})
	}))
	defer srv.Close()
	res, err := NewDocuments(pocket.Fixed(srv.URL, "pd_test", 5*time.Second)).Fetch(context.Background(), "2026-10-17", loc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"added_from=2026-10-16", "added_to=2026-10-17", "sort=added", "no_facets=1", "limit=500"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q lacks %s", query, want)
		}
	}
	a := res.Payload.(Activity)
	if a.Count != 3 || len(a.Items) != 3 {
		t.Fatalf("activity = %+v", a)
	}
	first := a.Items[0]
	if first.Title != "Water bill" || first.Time != "00:30" || first.Sub != "ViK · Invoice" || first.Group != "Invoice" || first.Ref != "2" || first.URL != "/documents/2" {
		t.Errorf("first = %+v", first)
	}
	if a.Items[1].Title != "scan-0042.pdf" || a.Items[1].Time != "11:15" || a.Items[2].Title != "Payslip" {
		t.Errorf("items = %+v", a.Items)
	}
	if len(a.Top) != 1 || a.Top[0].Name != "Invoice" || a.Top[0].Count != 2 {
		t.Errorf("top = %+v", a.Top)
	}
	if res.Headline != 3 || res.Summary != "3 documents · Water bill" {
		t.Errorf("headline %v summary %q", res.Headline, res.Summary)
	}

	// a rejected token is an error, not an empty day
	if _, err := NewDocuments(pocket.Fixed(srv.URL, "pd_wrong", 5*time.Second)).Fetch(context.Background(), "2026-10-17", loc); err == nil {
		t.Error("a rejected token gave no error")
	}
}
