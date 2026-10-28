package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// fetchers builds one Fetcher per source that is connected for the person.
// A source in Pocket Core mode is included only while its link is on, and
// asks Core again on every fetch (a fresh token for a Pocket app, the
// address and key for a service).
func (s *Server) fetchers(ctx context.Context, u *store.User, st store.Settings) map[string]sources.Fetcher {
	states := s.sourceStates(ctx, u, st)
	out := map[string]sources.Fetcher{}
	for name, src := range st.Sources {
		ss := states[name]
		if !ss.Active {
			continue
		}
		if name == "dawarich" {
			if f := s.locationSource(u, src, ss); f != nil {
				out[name] = f
			}
			continue
		}
		if name == "audiobookshelf" {
			if f := s.audiobooksSource(u, src, ss); f != nil {
				out[name] = f
			}
			continue
		}
		if name == "jellyseerr" {
			if f := s.requestsSource(u, src, ss); f != nil {
				out[name] = f
			}
			continue
		}
		if name == "maloja" {
			if f := s.musicSource(u, src, ss); f != nil {
				out[name] = f
			}
			continue
		}
		if _, ok := pocketSources[name]; ok {
			out[name] = pocketFetcher(name, s.pocketConn(u, name, src, ss.Mode))
			continue
		}
		if _, ok := serviceSources[name]; ok {
			out[name] = s.serviceFetcher(u, name, src, ss.Mode)
			continue
		}
		switch name {
		case "weather":
			out[name] = sources.NewWeather(st.HomeLat, st.HomeLon, st.HomePlace, s.cfg.UserAgent)
		}
	}
	return out
}

// userByID loads an account for the background work (nil when gone).
func (s *Server) userByID(ctx context.Context, userID int64) *store.User {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil
	}
	return u
}

// immich returns the user's Immich client when they connected it (thumbnails),
// asking Pocket Core for the key in Pocket Core mode (the SDK reuses it for a
// minute, so a page of thumbnails asks once).
func (s *Server) immich(ctx context.Context, u *store.User) *sources.Immich {
	st, _ := s.store.GetSettings(ctx, u.ID)
	src := st.Sources["immich"]
	c, err := s.serviceConn(ctx, u, "immich", src, connMode("immich", src, s.core.Configured()))
	if err != nil {
		return nil
	}
	return sources.NewImmich(c.URL, c.Key)
}

// frozen reports whether a day's facts are final: anything before yesterday.
func frozen(date string) bool {
	return date < store.AddDays(store.Today(), -1)
}

// pullDay fetches every source the user configured for one date and stores
// the facts as theirs. Frozen days are skipped unless force is set or a source
// has nothing yet.
func (s *Server) pullDay(ctx context.Context, userID int64, date string, force bool) (map[string]error, bool) {
	return s.pullDayOnly(ctx, userID, date, force, nil)
}

// pullDayOnly is pullDay limited to some sources (nil = all).
func (s *Server) pullDayOnly(ctx context.Context, userID int64, date string, force bool, only []string) (map[string]error, bool) {
	if userID <= 0 {
		return nil, true
	}
	key := fmt.Sprintf("%d:%s", userID, date)
	s.pullMu.Lock()
	if s.pulling[key] {
		s.pullMu.Unlock()
		return nil, false
	}
	s.pulling[key] = true
	s.pullMu.Unlock()
	defer func() {
		s.pullMu.Lock()
		delete(s.pulling, key)
		s.pullMu.Unlock()
	}()

	u := s.userByID(ctx, userID)
	if u == nil {
		return nil, true
	}
	st, _ := s.store.GetSettings(ctx, userID)
	fs := s.fetchers(ctx, u, st)
	if len(only) > 0 {
		keep := map[string]bool{}
		for _, o := range only {
			keep[o] = true
		}
		for name := range fs {
			if !keep[name] {
				delete(fs, name)
			}
		}
	}
	if len(fs) == 0 {
		return nil, true
	}
	have := s.store.FactSources(ctx, userID, date)
	loc := time.Now().Location()
	errs := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, f := range fs {
		if !force && frozen(date) && have[name] {
			continue
		}
		wg.Add(1)
		go func(name string, f sources.Fetcher) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			res, err := f.Fetch(cctx, date, loc)
			mu.Lock()
			defer mu.Unlock()
			if errors.Is(err, pocket.ErrNotConnected) {
				// the Pocket Core link went off since the states were read (or the
				// person has no account of their own in the service): skipped, not failed
				s.log.Debug("fact source not connected", "date", date, "source", name, "err", err)
				return
			}
			if err != nil {
				errs[name] = err
				_ = s.store.PutFactError(ctx, userID, date, name, err.Error())
				s.log.Debug("fact fetch failed", "date", date, "source", name, "err", err)
				return
			}
			if name == "pinepods" {
				s.adjustListening(ctx, userID, date, name, res)
			}
			b, _ := json.Marshal(res.Payload)
			_ = s.store.PutFact(ctx, userID, &store.Fact{Date: date, Source: name, Payload: b, Headline: res.Headline, Summary: res.Summary})
		}(name, f)
	}
	wg.Wait()
	if _, ok := fs["habits"]; ok {
		s.syncHabitsEntry(ctx, userID, date)
	}
	// Weather at the place the day actually happened, when Dawarich knows it.
	if wf, ok := fs["weather"].(*sources.Weather); ok {
		if lat, lon, place := s.dayLocation(ctx, userID, date, st); lat != 0 || lon != 0 {
			if res, err := wf.FetchAt(ctx, date, lat, lon, place); err == nil {
				b, _ := json.Marshal(res.Payload)
				_ = s.store.PutFact(ctx, userID, &store.Fact{Date: date, Source: "weather", Payload: b, Headline: res.Headline, Summary: res.Summary})
			}
		}
	}
	return errs, true
}

// dayLocation returns where the day was spent when the Dawarich facts say it
// was clearly somewhere else than home: the centre of the day's bounds.
func (s *Server) dayLocation(ctx context.Context, userID int64, date string, st store.Settings) (float64, float64, string) {
	facts, err := s.store.FactsForDay(ctx, userID, date)
	if err != nil {
		return 0, 0, ""
	}
	f, ok := facts["dawarich"]
	if !ok || f.Error != "" {
		return 0, 0, ""
	}
	var w sources.Where
	if json.Unmarshal(f.Payload, &w) != nil || w.Bounds == nil {
		return 0, 0, ""
	}
	lat := (w.Bounds[0] + w.Bounds[2]) / 2
	lon := (w.Bounds[1] + w.Bounds[3]) / 2
	// within ~40 km of home: use home (its name is nicer and the weather is the same)
	if st.HomeLat != 0 || st.HomeLon != 0 {
		dlat := (lat - st.HomeLat) * 111
		dlon := (lon - st.HomeLon) * 78
		if dlat*dlat+dlon*dlon < 40*40 {
			return 0, 0, ""
		}
	}
	place := ""
	if p, ok := facts["immich"]; ok && p.Error == "" {
		var ph sources.Photos
		if json.Unmarshal(p.Payload, &ph) == nil && len(ph.Places) > 0 {
			place = ph.Places[0]
		}
	}
	if place == "" {
		for _, e := range w.Entries {
			if e.Kind == "visit" && e.Name != "" {
				place = e.Name
				break
			}
		}
	}
	if place == "" {
		place = "away"
	}
	return lat, lon, place
}

// pullDays pulls several dates for one user, oldest first.
func (s *Server) pullDays(ctx context.Context, userID int64, dates []string, force bool) {
	sort.Strings(dates)
	for _, d := range dates {
		if ctx.Err() != nil {
			return
		}
		s.pullDay(ctx, userID, d, force)
	}
}

// ---- backfill ----

// backfillState is one person's backfill: each runs over their own sources.
type backfillState struct {
	Running bool   `json:"running"`
	From    string `json:"from"`
	To      string `json:"to"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current"`
	Last    string `json:"last_run"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) backfillStatus(userID int64) backfillState {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	if b := s.backfills[userID]; b != nil {
		return *b
	}
	return backfillState{}
}

// anyBackfillRunning is for the metrics endpoint.
func (s *Server) anyBackfillRunning() bool {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	for _, b := range s.backfills {
		if b.Running {
			return true
		}
	}
	return false
}

func (s *Server) updateBackfill(userID int64, fn func(b *backfillState)) {
	s.backfillMu.Lock()
	defer s.backfillMu.Unlock()
	if s.backfills == nil {
		s.backfills = map[int64]*backfillState{}
	}
	b := s.backfills[userID]
	if b == nil {
		b = &backfillState{}
		s.backfills[userID] = b
	}
	fn(b)
}

// runBackfillAll backfills every account, one after the other, each with
// their own sources (an account without sources is skipped).
func (s *Server) runBackfillAll(ctx context.Context, from, to string, force bool) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		s.runBackfill(ctx, u.ID, from, to, force)
	}
}

// runBackfill pulls one user's facts for every day in from..to (default: their
// configured number of days back from yesterday), skipping frozen days that
// already have every configured source unless force is set. One day at a
// time, with a pause, so the other apps are not hammered.
func (s *Server) runBackfill(ctx context.Context, userID int64, from, to string, force bool, only ...string) {
	if userID <= 0 {
		return
	}
	u := s.userByID(ctx, userID)
	if u == nil {
		return
	}
	st, _ := s.store.GetSettings(ctx, userID)
	want := len(s.fetchers(ctx, u, st))
	if want == 0 {
		return
	}
	today := store.Today()
	if to == "" || to > today {
		to = today
	}
	if from == "" {
		from = store.AddDays(today, -st.BackfillDays)
	}
	if from > to {
		return
	}
	var dates []string
	for d := from; d <= to; d = store.AddDays(d, 1) {
		dates = append(dates, d)
	}
	started := false
	s.updateBackfill(userID, func(b *backfillState) {
		if b.Running {
			return
		}
		started = true
		b.Running, b.From, b.To, b.Done, b.Total, b.Error = true, from, to, 0, len(dates), ""
	})
	if !started {
		return
	}
	defer s.updateBackfill(userID, func(b *backfillState) { b.Running, b.Current, b.Last = false, "", store.Now() })
	for _, d := range dates {
		if ctx.Err() != nil {
			return
		}
		s.updateBackfill(userID, func(b *backfillState) { b.Current = d })
		if !force && frozen(d) && len(s.store.FactSources(ctx, userID, d)) >= want {
			s.updateBackfill(userID, func(b *backfillState) { b.Done++ })
			continue
		}
		s.pullDayOnly(ctx, userID, d, force, only)
		s.updateBackfill(userID, func(b *backfillState) { b.Done++ })
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
	s.log.Info("backfill finished", "user", userID, "from", from, "to", to, "days", len(dates))
}

// handleBackfill starts a backfill of the caller's own facts from their own sources.
func (s *Server) handleBackfill(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in struct {
		From    string   `json:"from"`
		To      string   `json:"to"`
		Force   bool     `json:"force"`
		Sources []string `json:"sources"`
	}
	_ = decode(r, &in)
	if (in.From != "" && !validDate(in.From)) || (in.To != "" && !validDate(in.To)) {
		writeErr(w, http.StatusBadRequest, "dates must be YYYY-MM-DD")
		return
	}
	if s.backfillStatus(u.ID).Running {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "a backfill is already running", "status": s.backfillStatus(u.ID)})
		return
	}
	go s.runBackfill(context.Background(), u.ID, in.From, in.To, in.Force, in.Sources...)
	time.Sleep(200 * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.backfillStatus(u.ID)})
}

func (s *Server) handleFactsStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	st, _ := s.store.GetSettings(ctx, u.ID)
	states := s.sourceStates(ctx, u, st)
	configured := []string{}
	for _, n := range store.SourceNames {
		if states[n].Active {
			configured = append(configured, n)
		}
	}
	dates, _ := s.store.FactDates(ctx, u.ID)
	first, last := "", ""
	if len(dates) > 0 {
		last, first = dates[0], dates[len(dates)-1]
	}
	writeJSON(w, http.StatusOK, map[string]any{"backfill": s.backfillStatus(u.ID), "configured": configured, "days_with_facts": len(dates), "first": first, "last": last})
}

func (s *Server) handleRefreshDay(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if date > store.Today() {
		writeErr(w, http.StatusBadRequest, "that day has not happened yet")
		return
	}
	u := userFrom(r.Context())
	errs, ran := s.pullDay(r.Context(), u.ID, date, true)
	if !ran {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "already refreshing"})
		return
	}
	out := map[string]string{}
	for k, e := range errs {
		out[k] = e.Error()
	}
	facts, _ := s.store.FactsForDay(r.Context(), u.ID, date)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "errors": out, "facts": facts})
}

func (s *Server) handleDayFact(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	facts, err := s.store.FactsForDay(r.Context(), userFrom(r.Context()).ID, date)
	if err != nil {
		s.fail(w, err, "facts")
		return
	}
	f, ok := facts[chiParam(r, "source")]
	if !ok {
		writeErr(w, http.StatusNotFound, "no facts from that source for that day")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	im := s.immich(r.Context(), userFrom(r.Context()))
	if im == nil {
		writeErr(w, http.StatusNotFound, "immich is not configured")
		return
	}
	if err := im.Thumbnail(r.Context(), w, chiParam(r, "id"), r.URL.Query().Get("size")); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

// ensureToday makes sure today's facts exist before the page is shown the
// first time (the janitor keeps them fresh afterwards).
func (s *Server) ensureDay(ctx context.Context, userID int64, date string) {
	if date > store.Today() {
		return
	}
	if len(s.store.FactSources(ctx, userID, date)) > 0 {
		return
	}
	u := s.userByID(ctx, userID)
	if u == nil {
		return
	}
	st, _ := s.store.GetSettings(ctx, userID)
	if len(s.fetchers(ctx, u, st)) == 0 {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	s.pullDay(cctx, userID, date, false)
}

var _ = fmt.Sprintf

// BackfillSync runs a backfill in the foreground (CLI) for every account,
// each with their own sources.
func (s *Server) BackfillSync(ctx context.Context, from, to string, force bool) {
	users, _ := s.store.ListUsers(ctx)
	for _, u := range users {
		s.runBackfill(ctx, u.ID, from, to, force)
		st := s.backfillStatus(u.ID)
		if st.Total == 0 {
			fmt.Printf("%s: no sources configured, skipped\n", u.Email)
			continue
		}
		fmt.Printf("%s: backfilled %d of %d days (%s .. %s)\n", u.Email, st.Done, st.Total, st.From, st.To)
	}
}
