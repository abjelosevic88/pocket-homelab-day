package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// fetchers builds one Fetcher per configured, enabled source.
func (s *Server) fetchers(st store.Settings) map[string]sources.Fetcher {
	out := map[string]sources.Fetcher{}
	for name, src := range st.Sources {
		if !src.Configured(name) {
			continue
		}
		switch name {
		case "dawarich":
			out[name] = sources.NewDawarich(src.URL, src.Key)
		case "immich":
			out[name] = sources.NewImmich(src.URL, src.Key)
		case "maloja":
			out[name] = sources.NewMaloja(src.URL)
		case "jellystat":
			out[name] = sources.NewJellystat(src.URL, src.Key, src.Extra)
		case "firefly":
			out[name] = sources.NewFirefly(src.URL, src.Key)
		case "habits":
			out[name] = sources.NewHabits(src.URL, src.Key)
		case "bookshelf":
			out[name] = sources.NewBookshelf(src.URL, src.Key)
		case "travel":
			out[name] = sources.NewTravel(src.URL, src.Key)
		case "kitchen":
			out[name] = sources.NewKitchen(src.URL, src.Key)
		case "garage":
			out[name] = sources.NewGarage(src.URL, src.Key)
		case "pinepods":
			out[name] = sources.NewPinePods(src.URL, src.Key)
		case "audiobookshelf":
			out[name] = sources.NewAudiobookshelf(src.URL, src.Key, src.Extra)
		case "jellyseerr":
			out[name] = sources.NewJellyseerr(src.URL, src.Key)
		case "forgejo":
			out[name] = sources.NewForgejo(src.URL, src.Key, src.Extra)
		case "nextcloud":
			out[name] = sources.NewNextcloud(src.URL, src.Key, src.Extra)
		case "paperless":
			out[name] = sources.NewPaperless(src.URL, src.Key, src.PublicURL)
		case "invoicing":
			out[name] = sources.NewInvoicing(src.URL, src.Key)
		case "split":
			out[name] = sources.NewSplit(src.URL, src.Key)
		case "inventory":
			out[name] = sources.NewInventory(src.URL, src.Key)
		case "weather":
			if st.HomeLat != 0 || st.HomeLon != 0 {
				out[name] = sources.NewWeather(st.HomeLat, st.HomeLon, st.HomePlace, s.cfg.UserAgent)
			}
		}
	}
	return out
}

// immich returns the Immich client when configured (thumbnails).
func (s *Server) immich(ctx context.Context) *sources.Immich {
	st, _ := s.store.GetSettings(ctx)
	src := st.Sources["immich"]
	if !src.Configured("immich") {
		return nil
	}
	return sources.NewImmich(src.URL, src.Key)
}

// frozen reports whether a day's facts are final: anything before yesterday.
func frozen(date string) bool {
	return date < store.AddDays(store.Today(), -1)
}

// pullDay fetches every configured source for one date and stores the facts.
// Frozen days are skipped unless force is set or a source has nothing yet.
func (s *Server) pullDay(ctx context.Context, date string, force bool) (map[string]error, bool) {
	return s.pullDayOnly(ctx, date, force, nil)
}

// pullDayOnly is pullDay limited to some sources (nil = all).
func (s *Server) pullDayOnly(ctx context.Context, date string, force bool, only []string) (map[string]error, bool) {
	s.pullMu.Lock()
	if s.pulling[date] {
		s.pullMu.Unlock()
		return nil, false
	}
	s.pulling[date] = true
	s.pullMu.Unlock()
	defer func() {
		s.pullMu.Lock()
		delete(s.pulling, date)
		s.pullMu.Unlock()
	}()

	st, _ := s.store.GetSettings(ctx)
	fs := s.fetchers(st)
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
	have := s.store.FactSources(ctx, date)
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
			if err != nil {
				errs[name] = err
				_ = s.store.PutFactError(ctx, date, name, err.Error())
				s.log.Debug("fact fetch failed", "date", date, "source", name, "err", err)
				return
			}
			if name == "pinepods" {
				s.adjustListening(ctx, date, name, res)
			}
			b, _ := json.Marshal(res.Payload)
			_ = s.store.PutFact(ctx, &store.Fact{Date: date, Source: name, Payload: b, Headline: res.Headline, Summary: res.Summary})
		}(name, f)
	}
	wg.Wait()
	if _, ok := fs["habits"]; ok {
		s.syncHabitsEntry(ctx, date)
	}
	// Weather at the place the day actually happened, when Dawarich knows it.
	if wf, ok := fs["weather"].(*sources.Weather); ok {
		if lat, lon, place := s.dayLocation(ctx, date, st); lat != 0 || lon != 0 {
			if res, err := wf.FetchAt(ctx, date, lat, lon, place); err == nil {
				b, _ := json.Marshal(res.Payload)
				_ = s.store.PutFact(ctx, &store.Fact{Date: date, Source: "weather", Payload: b, Headline: res.Headline, Summary: res.Summary})
			}
		}
	}
	return errs, true
}

// dayLocation returns where the day was spent when the Dawarich facts say it
// was clearly somewhere else than home: the centre of the day's bounds.
func (s *Server) dayLocation(ctx context.Context, date string, st store.Settings) (float64, float64, string) {
	facts, err := s.store.FactsForDay(ctx, date)
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

// pullDays pulls several dates, oldest first.
func (s *Server) pullDays(ctx context.Context, dates []string, force bool) {
	sort.Strings(dates)
	for _, d := range dates {
		if ctx.Err() != nil {
			return
		}
		s.pullDay(ctx, d, force)
	}
}

// ---- backfill ----

type backfillState struct {
	mu      sync.Mutex
	Running bool   `json:"running"`
	From    string `json:"from"`
	To      string `json:"to"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current"`
	Last    string `json:"last_run"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) backfillStatus() backfillState {
	s.backfill.mu.Lock()
	defer s.backfill.mu.Unlock()
	return backfillState{Running: s.backfill.Running, From: s.backfill.From, To: s.backfill.To, Done: s.backfill.Done, Total: s.backfill.Total, Current: s.backfill.Current, Last: s.backfill.Last, Error: s.backfill.Error}
}

// runBackfill pulls facts for every day in from..to (default: the configured
// number of days back from yesterday), skipping frozen days that already
// have every configured source unless force is set. One day at a time, with a
// pause, so the other apps are not hammered.
func (s *Server) runBackfill(ctx context.Context, from, to string, force bool, only ...string) {
	s.backfill.mu.Lock()
	if s.backfill.Running {
		s.backfill.mu.Unlock()
		return
	}
	st, _ := s.store.GetSettings(ctx)
	if len(s.fetchers(st)) == 0 {
		s.backfill.mu.Unlock()
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
		s.backfill.mu.Unlock()
		return
	}
	s.backfill.Running, s.backfill.From, s.backfill.To, s.backfill.Done, s.backfill.Error = true, from, to, 0, ""
	var dates []string
	for d := from; d <= to; d = store.AddDays(d, 1) {
		dates = append(dates, d)
	}
	s.backfill.Total = len(dates)
	s.backfill.mu.Unlock()
	defer func() {
		s.backfill.mu.Lock()
		s.backfill.Running, s.backfill.Current, s.backfill.Last = false, "", store.Now()
		s.backfill.mu.Unlock()
	}()
	want := len(s.fetchers(st))
	for _, d := range dates {
		if ctx.Err() != nil {
			return
		}
		s.backfill.mu.Lock()
		s.backfill.Current = d
		s.backfill.mu.Unlock()
		if !force && frozen(d) && len(s.store.FactSources(ctx, d)) >= want {
			s.backfill.mu.Lock()
			s.backfill.Done++
			s.backfill.mu.Unlock()
			continue
		}
		s.pullDayOnly(ctx, d, force, only)
		s.backfill.mu.Lock()
		s.backfill.Done++
		s.backfill.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
	s.log.Info("backfill finished", "from", from, "to", to, "days", len(dates))
}

func (s *Server) handleBackfill(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
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
	if s.backfillStatus().Running {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "a backfill is already running", "status": s.backfillStatus()})
		return
	}
	go s.runBackfill(context.Background(), in.From, in.To, in.Force, in.Sources...)
	time.Sleep(200 * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.backfillStatus()})
}

func (s *Server) handleFactsStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx)
	configured := []string{}
	for _, n := range store.SourceNames {
		if st.Sources[n].Configured(n) && (n != "weather" || st.HomeLat != 0 || st.HomeLon != 0) {
			configured = append(configured, n)
		}
	}
	dates, _ := s.store.FactDates(ctx)
	first, last := "", ""
	if len(dates) > 0 {
		last, first = dates[0], dates[len(dates)-1]
	}
	writeJSON(w, http.StatusOK, map[string]any{"backfill": s.backfillStatus(), "configured": configured, "days_with_facts": len(dates), "first": first, "last": last})
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
	errs, ran := s.pullDay(r.Context(), date, true)
	if !ran {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "already refreshing"})
		return
	}
	out := map[string]string{}
	for k, e := range errs {
		out[k] = e.Error()
	}
	facts, _ := s.store.FactsForDay(r.Context(), date)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "errors": out, "facts": facts})
}

func (s *Server) handleDayFact(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	facts, err := s.store.FactsForDay(r.Context(), date)
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
	im := s.immich(r.Context())
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
func (s *Server) ensureDay(ctx context.Context, date string) {
	if date > store.Today() {
		return
	}
	if len(s.store.FactSources(ctx, date)) > 0 {
		return
	}
	st, _ := s.store.GetSettings(ctx)
	if len(s.fetchers(st)) == 0 {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	s.pullDay(cctx, date, false)
}

var _ = fmt.Sprintf

// BackfillSync runs a backfill in the foreground (CLI).
func (s *Server) BackfillSync(ctx context.Context, from, to string, force bool) {
	s.runBackfill(ctx, from, to, force)
	st := s.backfillStatus()
	fmt.Printf("backfilled %d of %d days (%s .. %s)\n", st.Done, st.Total, st.From, st.To)
}
