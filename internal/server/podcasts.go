package server

import (
	"context"
	"fmt"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
	"time"
)

// Offline listening may arrive weeks later. Reconcile the server's changed
// dates, including frozen days, without re-fetching other journal sources.
func (s *Server) syncPodcastHistory(ctx context.Context, u *store.User) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	st, err := s.store.GetSettings(ctx, u.ID)
	if err != nil {
		return
	}
	ss := s.sourceStates(ctx, u, st)["podcasts"]
	if !ss.Active {
		return
	}
	c, err := s.pocketConn(u, "podcasts", st.Sources["podcasts"], ss.Mode)(ctx)
	if err != nil {
		return
	}
	var out struct {
		Dates []struct {
			Date    string `json:"date"`
			Updated string `json:"updated_at"`
		} `json:"dates"`
	}
	if err = c.Get(ctx, "/api/v1/listens/dates", nil, &out); err != nil {
		s.log.Debug("podcast history dates", "err", err)
		return
	}
	for _, d := range out.Dates {
		if _, err := time.Parse("2006-01-02", d.Date); err != nil || d.Date > store.Today() {
			continue
		}
		key := fmt.Sprintf("podcast-revision:%d:%s", u.ID, d.Date)
		revision, _, ok := s.store.CacheGet(ctx, key)
		facts, err := s.store.FactsForDay(ctx, u.ID, d.Date)
		if err != nil {
			return
		}
		f, have := facts["podcasts"]
		if ok && revision == d.Updated && have && f.Error == "" {
			continue
		}
		errs, ran := s.pullDayOnly(ctx, u.ID, d.Date, true, []string{"podcasts"})
		if !ran || len(errs) > 0 {
			continue
		}
		// A revoked link can skip the fetch without returning an error.
		after, err := s.store.FactsForDay(ctx, u.ID, d.Date)
		if err != nil {
			return
		}
		next, haveNext := after["podcasts"]
		if haveNext && next.Error == "" && (!have || next.FetchedAt != f.FetchedAt) {
			_ = s.store.CachePut(ctx, key, d.Updated)
		}
	}
}

func (s *Server) syncAllPodcastHistory(ctx context.Context) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	for i := range users {
		if ctx.Err() != nil {
			return
		}
		s.syncPodcastHistory(ctx, &users[i])
	}
}
