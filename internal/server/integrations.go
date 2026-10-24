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
// Pocket app and third-party service sources also say how they are reached
// (mode), whether that works (via: core | direct | "") and why not (detail);
// a service through Pocket Core says whose key it comes with (key_source:
// personal | box).
type publicSource struct {
	URL        string `json:"url"`
	Key        string `json:"key"`
	PublicURL  string `json:"public_url"`
	Extra      string `json:"extra"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	NeedsKey   bool   `json:"needs_key"`
	Pocket     bool   `json:"pocket"`
	Service    bool   `json:"service"`
	App        string `json:"app,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Via        string `json:"via"`
	Detail     string `json:"detail,omitempty"`
	KeySource  string `json:"key_source,omitempty"`
	// The location source only (movement.go).
	Location *publicLocation `json:"location,omitempty"`
	Music    *publicMusic    `json:"music,omitempty"`
}

// publicLocation is where the location source's days come from: the saved
// choice ("" = not chosen), the provider in effect, and how Pocket Movement
// and Dawarich each stand.
type publicLocation struct {
	Choice         string `json:"choice"`
	Provider       string `json:"provider"`
	MovementVia    string `json:"movement_via"`
	MovementDetail string `json:"movement_detail,omitempty"`
	DawarichVia    string `json:"dawarich_via"`
}

// publicMusic is where the music source's days come from: the saved choice
// ("" = not chosen), the provider in effect, and how Pocket Music and Maloja
// each stand.
type publicMusic struct {
	Choice      string `json:"choice"`
	Provider    string `json:"provider"`
	MusicVia    string `json:"music_via"`
	MusicDetail string `json:"music_detail,omitempty"`
	MalojaVia   string `json:"maloja_via"`
}

func (s *Server) publicSettings(ctx context.Context, u *store.User, st store.Settings) map[string]any {
	states := s.sourceStates(ctx, u, st)
	s.probeKeySources(ctx, u, st, states)
	srcs := map[string]publicSource{}
	for _, n := range store.SourceNames {
		src := st.Sources[n]
		ss := states[n]
		ps := publicSource{URL: src.URL, Key: mask(src.Key), PublicURL: src.PublicURL, Extra: src.Extra, Enabled: src.Enabled, Configured: ss.Active, NeedsKey: store.NeedsKey(n)}
		if ct, ok := connectable(n); ok {
			ps.Pocket, ps.Service, ps.App, ps.Mode, ps.Via, ps.Detail, ps.KeySource = !ct.service, ct.service, ct.app, ss.Mode, ss.Via, ss.Detail, ss.KeySource
		}
		if n == "maloja" {
			ps.Music = &publicMusic{Choice: src.Provider, Provider: ss.Provider, MusicVia: ss.MusicVia, MusicDetail: ss.MusicDetail, MalojaVia: ss.MalojaVia}
		}
		if n == "dawarich" {
			ps.Location = &publicLocation{Choice: src.Provider, Provider: ss.Provider, MovementVia: ss.MovementVia, MovementDetail: ss.MovementDetail, DawarichVia: ss.DawarichVia}
		}
		srcs[n] = ps
	}
	_, aiStatus := s.inferenceFor(ctx, u.ID, st.AIProvider, st.AIKey != "")
	return map[string]any{
		"ai_provider": aiStatus.Provider, "ai_pocket_model": st.AIPocketModel, "ai_status": aiStatus, "ai_configured": aiStatus.Ready, "ai_transcribe_configured": st.AIKey != "",
		"title": st.Title, "date_format": st.DateFormat, "time_format": st.TimeFormat, "week_start": st.WeekStart, "home_currency": st.HomeCurrency, "distance_unit": st.DistanceUnit,
		"home_lat": st.HomeLat, "home_lon": st.HomeLon, "home_place": st.HomePlace, "prompt": st.Prompt, "backfill_days": st.BackfillDays,
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_configured": st.NotifyURL != "",
		"notify_evening": st.NotifyEvening, "evening_hour": st.EveningHour, "notify_morning": st.NotifyMorning, "morning_hour": st.MorningHour,
		"ai_key": mask(st.AIKey), "ai_model": st.AIModel, "ai_base_url": st.AIBaseURL, "ai_transcribe_model": st.AITranscribeModel, "ai_language": st.AILanguage,
		"ai_question": st.AIQuestion, "ai_weekly_story": st.AIWeeklyStory, "ai_send_text": st.AISendText,
		"ai_default_model": ai.DefaultModel, "ai_default_transcribe": ai.DefaultTranscribe,
		"sources": srcs, "source_names": store.SourceNames, "core_configured": s.core.Configured(), "core_connected": s.coreConnected(), "setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL, "timezone": time.Now().Location().String(),
	}
}

// handleGetSettings returns the caller's own settings (their sources, AI key,
// notifications…) plus the box-wide title.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	st, err := s.store.GetSettings(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), u, st))
}

// handlePutSettings saves the caller's own settings. The box-wide title is
// admin-only; a member sending it gets 403.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	cur, err := s.store.GetSettings(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	modesBefore := s.personModes(u.Email, cur)
	var in struct {
		store.Settings
		Sources map[string]struct {
			URL       *string `json:"url"`
			Key       *string `json:"key"`
			PublicURL *string `json:"public_url"`
			Extra     *string `json:"extra"`
			Enabled   *bool   `json:"enabled"`
			Mode      *string `json:"mode"`
			Provider  *string `json:"provider"`
		} `json:"sources"`
	}
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	boxChanged := false
	if keys["title"] && strings.TrimSpace(in.Title) != cur.Title {
		if u.Role != "admin" {
			writeErr(w, http.StatusForbidden, "only an admin can rename the app")
			return
		}
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
		boxChanged = true
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
	if keys["ai_pocket_model"] {
		if !validPocketModel(in.AIPocketModel) {
			writeErr(w, 400, "invalid Pocket AI model")
			return
		}
		cur.AIPocketModel = in.AIPocketModel
	}
	if keys["ai_provider"] {
		if !validAIProvider(in.AIProvider) {
			writeErr(w, 400, "choose automatic, Pocket AI or OpenAI")
			return
		}
		cur.AIProvider = in.AIProvider
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
		if _, ok := connectable(name); ok && p.Mode != nil && validMode(*p.Mode) {
			src.Mode = *p.Mode
			src.Enabled = src.Mode != modeOff
		}
		if name == "dawarich" && p.Provider != nil && (*p.Provider == "" || validLocationProvider(*p.Provider)) {
			src.Provider = *p.Provider
		}
		if name == "maloja" && p.Provider != nil && (*p.Provider == "" || validMusicProvider(*p.Provider)) {
			src.Provider = *p.Provider
		}
		cur.Sources[name] = src
	}
	if err := s.store.SaveSettings(r.Context(), u.ID, cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if boxChanged {
		if err := s.store.SaveBoxSettings(r.Context(), cur); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	if !sameModes(modesBefore, s.personModes(u.Email, cur)) {
		s.reportModesSoon() // a mode picked, Disconnect, a key pasted or cleared
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), u, cur))
}

// handleIntegrationTest checks one source with the posted credentials or the
// caller's own saved ones (never anybody else's).
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
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
	u := userFrom(r.Context())
	st, _ := s.store.GetSettings(r.Context(), u.ID)
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
	typed := in.Key != "" && !strings.Contains(in.Key, "••")
	if in.Which == "music" {
		msg, err := sources.NewPocketMusic(s.musicConn(u)).Test(ctx)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": strings.TrimPrefix(err.Error(), "not connected: ")})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg + " through Pocket Core"})
		return
	}
	if in.Which == "movement" {
		msg, err := sources.NewMovement(s.movementConn(u)).Test(ctx)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": strings.TrimPrefix(err.Error(), "not connected: ")})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg + " through Pocket Core"})
		return
	}
	if _, ok := pocketSources[in.Which]; ok {
		s.testPocketSource(ctx, w, u, in.Which, saved, url, key, typed)
		return
	}
	if _, ok := serviceSources[in.Which]; ok {
		s.testServiceSource(ctx, w, u, in.Which, saved, url, key, extra, typed)
		return
	}
	switch in.Which {
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

// testPocketSource checks a Pocket app source the way it would be fetched: a
// token typed into the form is tried as is; otherwise the saved mode decides
// (direct with the saved URL and token, Pocket Core with a fresh link token).
func (s *Server) testPocketSource(ctx context.Context, w http.ResponseWriter, u *store.User, name string, saved store.Source, url, key string, typed bool) {
	mode := connMode(name, saved, s.core.Configured())
	if typed {
		mode = modeDirect
	}
	how := ", direct connection"
	src := saved
	switch mode {
	case modeDirect:
		src.URL, src.Key = url, key
	case modeCore:
		how = " through Pocket Core"
	}
	msg, err := pocketFetcher(name, s.pocketConn(u, name, src, mode)).Test(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg + how})
}

// testServiceSource checks a third-party service the same way: a key typed
// into the form is tried directly; otherwise the saved mode decides (direct
// with what the form shows, falling back to the saved values, or Pocket Core
// with the address and key Core hands out right now).
func (s *Server) testServiceSource(ctx context.Context, w http.ResponseWriter, u *store.User, name string, saved store.Source, url, key, extra string, typed bool) {
	mode := connMode(name, saved, s.core.Configured())
	if typed {
		mode = modeDirect
	}
	src := saved
	if mode == modeDirect {
		src.URL, src.Key, src.Extra = url, key, extra
	} else {
		src.Extra = extra // non-credential settings (user filters) as shown in the form
	}
	msg, err := s.serviceFetcher(u, name, src, mode).Test(ctx)
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
	u := userFrom(r.Context())
	dates, _ := s.store.FactDates(r.Context(), u.ID)
	first := ""
	if len(dates) > 0 {
		first = dates[len(dates)-1]
	}
	counts := map[string]int{}
	if u.Role == "admin" { // box-wide row counts (numbers only, no content) are for the admin
		counts = s.store.Counts(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": counts, "days_with_facts": len(dates), "first_fact_day": first, "backfill": s.backfillStatus(u.ID),
	})
}

// coreConnected reports whether Day is connected to a Pocket Core: configured
// and registered with it. The Core-only choices in Settings are greyed out
// otherwise.
func (s *Server) coreConnected() bool {
	if !s.core.Configured() {
		return false
	}
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return s.coreOK || s.core.Token() != "" // a failed re-registration keeps the token
}

// coreStatus summarises the Pocket Core link for the About page.
func (s *Server) coreStatus() map[string]any {
	s.coreMu.Lock()
	defer s.coreMu.Unlock()
	return map[string]any{"configured": s.core.Configured(), "url": s.cfg.CoreURL, "registered": s.coreOK, "error": s.coreErr}
}
