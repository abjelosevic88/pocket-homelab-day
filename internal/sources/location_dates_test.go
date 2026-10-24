package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLocationSourcesPreserveLocalDates(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Sarajevo")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, start, end, startDate, endDate, startClock, endClock string }{
		{"overnight", "2026-10-23T21:20:00Z", "2026-10-24T15:21:00Z", "2026-10-23", "2026-10-24", "23:20", "17:21"},
		{"local midnight differs from UTC", "2026-10-23T22:20:00Z", "2026-10-23T23:21:00Z", "2026-10-24", "2026-10-24", "00:20", "01:21"},
		{"DST fallback", "2026-10-24T21:20:00Z", "2026-10-25T16:21:00Z", "2026-10-24", "2026-10-25", "23:20", "17:21"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, _ := time.Parse(time.RFC3339, tc.start)
			en, _ := time.Parse(time.RFC3339, tc.end)
			duration := int(en.Sub(st).Seconds())
			var d movementDay
			raw, _ := json.Marshal(map[string]any{"entries": []any{map[string]any{"type": "visit", "start_ts": st.Unix(), "end_ts": en.Unix()}}})
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			movement := movementWhere(d, loc)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"days": []any{map[string]any{"date": tc.endDate, "entries": []any{map[string]any{"type": "visit", "started_at": tc.start, "ended_at": tc.end, "duration": duration}}}}})
			}))
			defer srv.Close()
			result, err := NewDawarich(srv.URL, "test-key").Fetch(context.Background(), tc.endDate, loc)
			if err != nil {
				t.Fatal(err)
			}
			for _, where := range []Where{movement, result.Payload.(Where)} {
				if len(where.Entries) != 1 {
					t.Fatalf("entries = %+v", where.Entries)
				}
				v := where.Entries[0]
				if v.StartDate != tc.startDate || v.EndDate != tc.endDate || v.Start != tc.startClock || v.End != tc.endClock || v.Minutes != duration/60 {
					t.Fatalf("%s lost local dates or duration: %+v", where.Provider, v)
				}
			}
		})
	}
}
