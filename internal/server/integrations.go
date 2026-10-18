package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

// publicSource is a source with its secret masked and a "configured" flag.
type publicSource struct {
	URL        string `json:"url"`
	Key        string `json:"key"`
	PublicURL  string `json:"public_url"`
	Extra      string `json:"extra"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	NeedsKey   bool   `json:"needs_key"`
}

func (s *Server) publicSettings(st store.Settings) map[string]any {
	srcs := map[string]publicSource{}
	for _, n := range store.SourceNames {
		src := st.Sources[n]
		conf := src.Configured(n)
		if n == "weather" {
			conf = src.Enabled && (st.HomeLat != 0 || st.HomeLon != 0)
		}
		srcs[n] = publicSource{URL: src.URL, Key: mask(src.Key), PublicURL: src.PublicURL, Extra: src.Extra, Enabled: src.Enabled, Configured: conf, NeedsKey: store.NeedsKey(n)}
	}
	return map[string]any{
		"title": st.Title, "date_format": st.DateFormat, "time_format": st.TimeFormat, "week_start": st.WeekStart, "home_currency": st.HomeCurrency, "distance_unit": st.DistanceUnit,
		"home_lat": st.HomeLat, "home_lon": st.HomeLon, "home_place": st.HomePlace, "prompt": st.Prompt, "backfill_days": st.BackfillDays,
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_configured": st.NotifyURL != "",
		"notify_evening": st.NotifyEvening, "evening_hour": st.EveningHour, "notify_morning": st.NotifyMorning, "morning_hour": st.MorningHour,
		"ai_key": mask(st.AIKey), "ai_model": st.AIModel, "ai_base_url": st.AIBaseURL, "ai_transcribe_model": st.AITranscribeModel, "ai_language": st.AILanguage,
		"ai_question": st.AIQuestion, "ai_weekly_story": st.AIWeeklyStory, "ai_send_text": st.AISendText, "ai_configured": st.AIKey != "",
		"ai_default_model": ai.DefaultModel, "ai_default_transcribe": ai.DefaultTranscribe,
		"sources": srcs, "source_names": store.SourceNames, "setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL, "timezone": time.Now().Location().String(),
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(st))
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	cur, err := s.store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	var in struct {
		store.Settings
		Sources map[string]struct {
			URL       *string `json:"url"`
			Key       *string `json:"key"`
			PublicURL *string `json:"public_url"`
			Extra     *string `json:"extra"`
			Enabled   *bool   `json:"enabled"`
		} `json:"sources"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["title"] {
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["week_start"] && (in.WeekStart == "mon" || in.WeekStart == "sun") {
		cur.WeekStart = in.WeekStart
	}
	if keys["home_currency"] && len(in.HomeCurrency) == 3 {
		cur.HomeCurrency = strings.ToUpper(in.HomeCurrency)
	}
	if keys["distance_unit"] && (in.DistanceUnit == "km" || in.DistanceUnit == "mi") {
		cur.DistanceUnit = in.DistanceUnit
	}
	if keys["home_lat"] {
		cur.HomeLat = in.HomeLat
	}
	if keys["home_lon"] {
		cur.HomeLon = in.HomeLon
	}
	if keys["home_place"] {
		cur.HomePlace = strings.TrimSpace(in.HomePlace)
	}
	if keys["prompt"] {
		cur.Prompt = strings.TrimSpace(in.Prompt)
		if cur.Prompt == "" {
			cur.Prompt = store.DefaultPrompt
		}
	}
	if keys["backfill_days"] && in.BackfillDays >= 0 && in.BackfillDays <= 3650 {
		cur.BackfillDays = in.BackfillDays
	}
	if keys["notify_url"] {
		cur.NotifyURL = strings.TrimSpace(in.NotifyURL)
	}
	if keys["notify_format"] && in.NotifyFormat != "" {
		cur.NotifyFormat = in.NotifyFormat
	}
	if keys["notify_evening"] {
		cur.NotifyEvening = in.NotifyEvening
	}
	if keys["evening_hour"] && in.EveningHour >= 0 && in.EveningHour <= 23 {
		cur.EveningHour = in.EveningHour
	}
	if keys["notify_morning"] {
		cur.NotifyMorning = in.NotifyMorning
	}
	if keys["morning_hour"] && in.MorningHour >= 0 && in.MorningHour <= 23 {
		cur.MorningHour = in.MorningHour
	}
	if keys["ai_key"] && !strings.Contains(in.AIKey, "••") {
		cur.AIKey = strings.TrimSpace(in.AIKey)
	}
	if keys["ai_model"] {
		cur.AIModel = strings.TrimSpace(in.AIModel)
	}
	if keys["ai_base_url"] {
		cur.AIBaseURL = strings.TrimRight(strings.TrimSpace(in.AIBaseURL), "/")
	}
	if keys["ai_transcribe_model"] {
		cur.AITranscribeModel = strings.TrimSpace(in.AITranscribeModel)
	}
	if keys["ai_language"] {
		cur.AILanguage = strings.TrimSpace(in.AILanguage)
	}
	if keys["ai_question"] {
		cur.AIQuestion = in.AIQuestion
	}
	if keys["ai_weekly_story"] {
		cur.AIWeeklyStory = in.AIWeeklyStory
	}
	if keys["ai_send_text"] {
		cur.AISendText = in.AISendText
	}
	for name, p := range in.Sources {
		src, ok := cur.Sources[name]
		if !ok {
			continue
		}
		if p.URL != nil {
			src.URL = strings.TrimRight(strings.TrimSpace(*p.URL), "/")
		}
		if p.Key != nil && !strings.Contains(*p.Key, "••") {
			src.Key = strings.TrimSpace(*p.Key)
		}
		if p.PublicURL != nil {
			src.PublicURL = strings.TrimRight(strings.TrimSpace(*p.PublicURL), "/")
		}
		if p.Extra != nil {
			src.Extra = strings.TrimSpace(*p.Extra)
		}
		if p.Enabled != nil {
			src.Enabled = *p.Enabled
		}
		cur.Sources[name] = src
	}
	if err := s.store.SaveSettings(r.Context(), cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(cur))
}

// handleIntegrationTest checks one source with the posted (or saved) credentials.
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var in struct {
		Which  string `json:"which"`
		URL    string `json:"url"`
		Key    string `json:"key"`
		Extra  string `json:"extra"`
		Format string `json:"format"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st, _ := s.store.GetSettings(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	saved := st.Sources[in.Which]
	url := strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if url == "" {
		url = saved.URL
	}
	key := in.Key
	if key == "" || strings.Contains(key, "••") {
		key = saved.Key
	}
	extra := in.Extra
	if extra == "" {
		extra = saved.Extra
	}
	var f sources.Fetcher
	switch in.Which {
	case "dawarich":
		f = sources.NewDawarich(url, key)
	case "immich":
		f = sources.NewImmich(url, key)
	case "maloja":
		f = sources.NewMaloja(url)
	case "jellystat":
		f = sources.NewJellystat(url, key, extra)
	case "firefly":
		f = sources.NewFirefly(url, key)
	case "habits":
		f = sources.NewHabits(url, key)
	case "bookshelf":
		f = sources.NewBookshelf(url, key)
	case "travel":
		f = sources.NewTravel(url, key)
	case "kitchen":
		f = sources.NewKitchen(url, key)
	case "garage":
		f = sources.NewGarage(url, key)
	case "weather":
		f = sources.NewWeather(st.HomeLat, st.HomeLon, st.HomePlace, s.cfg.UserAgent)
	case "ai":
		base, k, model := strings.TrimRight(strings.TrimSpace(in.URL), "/"), in.Key, strings.TrimSpace(in.Extra)
		if base == "" {
			base = st.AIBaseURL
		}
		if k == "" || strings.Contains(k, "••") {
			k = st.AIKey
		}
		if model == "" {
			model = st.AIModel
		}
		c := ai.New(base, k, model, "")
		ids, err := c.Models(ctx)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		for _, id := range ids {
			if id == c.Model {
				writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": fmt.Sprintf("Key works; %d models available, “%s” included", len(ids), c.Model)})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("Key works, but “%s” is not among its %d models", c.Model, len(ids))})
		return
	case "notify":
		fm := in.Format
		if fm == "" {
			fm = st.NotifyFormat
		}
		s.handleNotifyTestWith(w, r, url, fm)
		return
	default:
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
	msg, err := f.Test(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *Server) handleGeocode(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, []sources.Place{})
		return
	}
	places, err := sources.Geocode(r.Context(), s.cfg.UserAgent, q)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "geocoding: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, places)
}

// ---- system ----

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	dates, _ := s.store.FactDates(r.Context())
	first := ""
	if len(dates) > 0 {
		first = dates[len(dates)-1]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.Counts(r.Context()), "days_with_facts": len(dates), "first_fact_day": first, "backfill": s.backfillStatus(),
	})
}
