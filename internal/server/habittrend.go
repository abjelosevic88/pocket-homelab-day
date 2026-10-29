package server

import (
	"context"
	"net/http"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// handleHabitTrend returns the habits and trackers over whole months
// (?from=YYYY-MM&to=YYYY-MM, at most 24) for the calendar's habit charts.
// It asks Pocket Habits so days filled in later are there too, and falls
// back to the check-ins stored with the facts when Habits can't be asked.
func (s *Server) handleHabitTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if !validDate(from+"-01") || !validDate(to+"-01") || to < from {
		writeErr(w, http.StatusBadRequest, "pass from=YYYY-MM&to=YYYY-MM")
		return
	}
	months := []string{}
	for m := from; m <= to; m = store.AddDays(m+"-01", 32)[:7] {
		months = append(months, m)
	}
	if len(months) > 24 {
		writeErr(w, http.StatusBadRequest, "at most 24 months")
		return
	}
	st, _ := s.store.GetSettings(ctx, u.ID)
	if h, ok := s.fetchers(ctx, u, st)["habits"].(*sources.Habits); ok {
		tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if t, err := h.Months(tctx, months); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "live": true, "habits": t.Habits, "trackers": t.Trackers})
			return
		}
	}
	last := store.AddDays(store.AddDays(to+"-01", 32)[:7]+"-01", -1)
	fs, err := s.store.FactsRange(ctx, u.ID, from+"-01", last, "habits")
	if err != nil {
		s.fail(w, err, "habit trend")
		return
	}
	days := map[string]sources.Checkin{}
	for _, f := range fs {
		var c sources.Checkin
		if len(f.Payload) > 4 && jsonUnmarshal(f.Payload, &c) == nil {
			days[f.Date] = c
		}
	}
	t := sources.TrendFromCheckins(days)
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "live": false, "habits": t.Habits, "trackers": t.Trackers})
}
