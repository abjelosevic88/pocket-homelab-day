// Package server wires the HTTP API, auth and the embedded SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/bcrypt"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/config"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// Version is injected at build time.
var Version = "dev"

// Server holds dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	log      *slog.Logger
	webFS    fs.FS
	started  time.Time
	requests atomic.Int64
	errors   atomic.Int64

	pullMu     sync.Mutex // one pull at a time per user and date
	pulling    map[string]bool
	hookMu     sync.Mutex // (user, date) Pocket Money said changed, pulled after a short pause
	hookDays   map[hookDay]bool
	backfillMu sync.Mutex
	backfills  map[int64]*backfillState // per user
	notifyMu   sync.Mutex
	notified   map[string]string // "<user>:evening"/"<user>:morning"/"<user>:weekly" → last day sent

	oidcMu sync.Mutex
	oidc   *oidcProvider

	core     *core.Client
	coreMu   sync.Mutex
	coreOK   bool
	coreErr  string
	resolved map[string]resolvedURL
	svc      serviceMemo // whose key each service came with through Core (never the key)
	modesMu  sync.Mutex  // one connection-mode report at a time
}

type resolvedURL struct {
	url    string
	public string
	at     time.Time
}

// New creates a server.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS) *Server {
	// box-wide defaults every account starts from (no personal values here)
	st.Defaults = func(d *store.Settings) {
		if cfg.DefaultCurrency != "" {
			d.HomeCurrency = cfg.DefaultCurrency
		}
		if cfg.BackfillDays > 0 {
			d.BackfillDays = cfg.BackfillDays
		}
	}
	return &Server{backfills: map[int64]*backfillState{}, cfg: cfg, store: st, log: log, webFS: webFS, started: time.Now(), pulling: map[string]bool{}, notified: map[string]string{},
		oidc:     &oidcProvider{issuerURL: cfg.OIDCIssuer},
		core:     core.New(cfg.CoreURL, core.Options{DB: st.DB, TokenFile: cfg.DataDir + "/core.token", UserAgent: cfg.UserAgent}),
		resolved: map[string]resolvedURL{}}
}

// Bootstrap seeds the first admin (if configured), the owner's integration
// defaults and demo data, then starts the background work.
func (s *Server) Bootstrap(ctx context.Context) error {
	if err := s.core.Start(ctx); err != nil {
		return err
	}
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n == 0 && s.cfg.AuthMode == "local" && s.cfg.AdminEmail != "" && s.cfg.AdminPassword != "" {
		if len(s.cfg.AdminPassword) < 8 {
			return errors.New("ADMIN_PASSWORD must be at least 8 characters")
		}
		h, _ := bcrypt.GenerateFromPassword([]byte(s.cfg.AdminPassword), bcrypt.DefaultCost)
		u := &store.User{Email: s.cfg.AdminEmail, Name: s.cfg.AdminName, PasswordHash: string(h), Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
		st, _ := s.store.GetSettings(ctx, 0)
		st.SetupComplete = true
		if err := s.store.SaveBoxSettings(ctx, st); err != nil {
			return err
		}
		s.log.Info("admin user created from environment", "email", u.Email)
	}
	if err := s.seedOwner(ctx); err != nil {
		return err
	}
	if s.cfg.DemoData {
		if err := s.seedDemo(ctx); err != nil {
			return err
		}
	}
	_ = s.store.PurgeSessions(ctx)
	go s.janitor(ctx)
	if s.core.Configured() {
		go s.coreLoop(ctx)
	}
	return nil
}

// seedOwner gives the owner (the first admin) the source tokens, AI key,
// notification route and home place from the environment. Those are the
// owner's own accounts, so nobody else gets them: every other account starts
// with no sources and configures their own. Only empty values are filled.
func (s *Server) seedOwner(ctx context.Context) error {
	owner := s.store.OwnerID(ctx)
	if owner == 0 {
		return nil
	}
	if err := s.store.AdoptLegacySettings(ctx, owner); err != nil {
		return err
	}
	st, err := s.store.GetSettings(ctx, owner)
	if err != nil {
		return err
	}
	changed := !s.store.HasPersonalSettings(ctx, owner)
	set := func(dst *string, v string) {
		if *dst == "" && v != "" {
			*dst = v
			changed = true
		}
	}
	set(&st.NotifyURL, s.cfg.NotifyURL)
	set(&st.AIKey, s.cfg.OpenAIKey)
	set(&st.AIModel, s.cfg.OpenAIModel)
	set(&st.AIBaseURL, s.cfg.OpenAIBaseURL)
	if s.cfg.NotifyFormat != "" && st.NotifyURL == s.cfg.NotifyURL && st.NotifyFormat != s.cfg.NotifyFormat {
		st.NotifyFormat = s.cfg.NotifyFormat
		changed = true
	}
	if st.HomeLat == 0 && st.HomeLon == 0 && (s.cfg.HomeLat != 0 || s.cfg.HomeLon != 0) {
		st.HomeLat, st.HomeLon, st.HomePlace = s.cfg.HomeLat, s.cfg.HomeLon, s.cfg.HomePlace
		changed = true
	}
	// A connectable source's values (Pocket apps and third-party services) are
	// copied once: clearing them in Settings (Disconnect, to switch over to
	// Pocket Core) must not bring them back at the next start. A value
	// already there counts as copied.
	once := func(name, field string, dst *string, v string) {
		key := fmt.Sprintf("seeded:%d:src_%s_%s", owner, name, field)
		if _, _, done := s.store.CacheGet(ctx, key); done {
			return
		}
		set(dst, v)
		if *dst != "" {
			_ = s.store.CachePut(ctx, key, "1")
		}
	}
	for name, se := range s.cfg.Sources {
		src := st.Sources[name]
		if _, ok := connectable(name); ok {
			once(name, "url", &src.URL, se.URL)
			once(name, "key", &src.Key, se.Key)
			once(name, "public", &src.PublicURL, se.PublicURL)
			once(name, "extra", &src.Extra, se.Extra)
		} else {
			set(&src.URL, se.URL)
			set(&src.Key, se.Key)
			set(&src.PublicURL, se.PublicURL)
			set(&src.Extra, se.Extra)
		}
		st.Sources[name] = src
	}
	if !changed {
		return nil
	}
	return s.store.SaveSettings(ctx, owner, st)
}

// janitor runs the background work, for every account with their own
// sources: today and yesterday every 30 minutes, the daily pushes once their
// hour has passed, and the initial backfill.
func (s *Server) janitor(ctx context.Context) {
	first := time.NewTimer(10 * time.Second)
	t := time.NewTicker(30 * time.Minute)
	podcasts := time.NewTicker(time.Minute)
	defer podcasts.Stop()
	defer t.Stop()
	tick := func() {
		today := store.Today()
		users, _ := s.store.ListUsers(ctx)
		for i := range users {
			s.pullDays(ctx, users[i].ID, []string{today, store.AddDays(today, -1)}, false)
		}
		for i := range users {
			s.maybeNotify(ctx, &users[i])
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			s.syncAllHabitsEntries(ctx)
			tick()
			go s.runBackfillAll(ctx, "", "", false)
		case <-podcasts.C:
			s.syncAllPodcastHistory(ctx)
		case <-t.C:
			tick()
		}
	}
}

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(180 * time.Second))

	r.Get("/pocket-ask.js", s.core.AskWidget("day"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Get("/auth/oidc/start", s.handleOIDCStart)
		r.Get("/auth/oidc/callback", s.handleOIDCCallback)
		r.Post("/hooks/money", s.handleMoneyHook) // signed, not a session

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			r.Get("/dashboard", s.handleDashboard)
			r.Get("/home", s.handleHome)
			r.Get("/meta", s.handleMeta)
			r.Get("/search", s.handleSearch)

			r.Get("/today", s.handleToday)
			r.Get("/days/{date}", s.handleDay)
			r.Put("/days/{date}", s.handlePutEntry)
			r.Delete("/days/{date}", s.handleDeleteEntry)
			r.Post("/days/{date}/refresh", s.handleRefreshDay)
			r.Get("/days/{date}/facts/{source}", s.handleDayFact)
			r.Post("/days/{date}/pins", s.handleCreatePin)
			r.Delete("/pins/{id}", s.handleDeletePin)

			r.Get("/entries", s.handleListEntries)
			r.Post("/entries", s.handleQuickEntry)
			r.Get("/tags", s.handleTags)
			r.Get("/calendar", s.handleCalendar)
			r.Get("/memories", s.handleMemories)
			r.Get("/years", s.handleYears)
			r.Get("/years/{year}", s.handleYear)

			r.Get("/moments", s.handleListMoments)
			r.Post("/moments", s.handleCreateMoment)
			r.Put("/moments/{id}", s.handleUpdateMoment)
			r.Delete("/moments/{id}", s.handleDeleteMoment)

			r.Get("/photos/{id}/thumb", s.handleThumb)

			r.Post("/ai/days/{date}/draft", s.handleAIDraft)
			r.Get("/ai/days/{date}/question", s.handleAIQuestion)
			r.Post("/ai/days/{date}/suggest", s.handleAISuggest)
			r.Post("/ai/transcribe", s.handleAITranscribe)
			r.Get("/ai/stories", s.handleListStories)
			r.Get("/ai/stories/{kind}/{period}", s.handleGetStory)
			r.Post("/ai/stories/{kind}/{period}", s.handleMakeStory)
			r.Delete("/ai/stories/{kind}/{period}", s.handleDeleteStory)
			r.Get("/ai/asks", s.handleListAsks)
			r.Post("/ai/asks", s.handleAsk)
			r.Delete("/ai/asks/{id}", s.handleDeleteAsk)
			r.Get("/ai/usage", s.handleAIUsage)
			r.Get("/patterns", s.handlePatterns)

			r.Post("/facts/backfill", s.handleBackfill)
			r.Get("/facts/status", s.handleFactsStatus)
			r.Post("/integrations/test", s.handleIntegrationTest)
			r.Get("/ai/models", s.handleInferenceModels)
			r.Get("/geo/search", s.handleGeocode)
			r.Post("/notify/test", s.handleNotifyTest)
			r.Post("/notify/run", s.handleNotifyRun)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
			r.Get("/export.json", s.handleExportJSON)
			r.Get("/export.md", s.handleExportMarkdown)
			r.Post("/import", s.handleImportJSON)
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// dateParam reads {date}, accepting "today" and "yesterday".
func dateParam(r *http.Request) (string, bool) {
	d := chi.URLParam(r, "date")
	switch d {
	case "today":
		return store.Today(), true
	case "yesterday":
		return store.AddDays(store.Today(), -1), true
	}
	return d, validDate(d)
}

// ---------- health / metrics ----------

// ---- Pocket Core ----

// coreLoop registers with Core on start (retrying until it answers) and
// re-registers every 30 minutes so version and last-seen stay fresh; each
// time it also reports how everybody connects (modes.go).
func (s *Server) coreLoop(ctx context.Context) {
	wait := 5 * time.Second
	for {
		err := s.registerWithCore(ctx)
		s.coreMu.Lock()
		s.coreOK, s.coreErr = err == nil, ""
		if err != nil {
			s.coreErr = err.Error()
		}
		s.coreMu.Unlock()
		if err != nil {
			s.log.Warn("pocket core: registration failed, retrying", "err", err, "in", wait.String())
		} else {
			wait = 30 * time.Minute
			s.reportModes(ctx) // on start and with every re-registration (30 min)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err != nil && wait < 5*time.Minute {
			wait *= 2
		}
	}
}

func (s *Server) registerWithCore(ctx context.Context) error {
	m := core.Manifest{
		Slug: "day", Name: "Pocket Day",
		Description: "A journal that writes itself: one page per day with your own words on top and what the other apps on your server know about it underneath.",
		Icon:        "/favicon.svg", Version: Version,
		URL: s.cfg.CoreAppURL, PublicURL: s.cfg.BaseURL, HealthPath: "/healthz", EnvPrefix: "DAY",
		Publishes: []string{"day.entry_written", "day.moment_created", "day.story_created"},
		// Read the person's own data in the other Pocket apps (see siblings.go).
		Uses: append(pocketUses(), core.InferenceUse()),
		// Reach the person's third-party services through Core (see services.go).
		Services: serviceUses(),
	}
	if s.cfg.BaseURL != "" {
		// ask Core for a relying-party client so "Sign in with Pocket Core" needs no configuration
		m.OIDC = &core.OIDCRequest{RedirectURIs: []string{s.oidcCallbackURL()}}
	}
	reg, err := s.core.Register(ctx, s.cfg.CoreRegistrationKey, m)
	if err != nil {
		return err
	}
	if reg.Token != "" {
		s.log.Info("pocket core: registered, app token stored", "core", s.cfg.CoreURL)
	} else {
		s.log.Debug("pocket core: re-registered", "core", s.cfg.CoreURL)
	}
	return nil
}

// publish persists an event locally; the SDK retries network delivery.

// resolveSibling asks Core where a sibling app lives (cached a minute) so the
// *_URL settings may stay empty on a box with Core.
func (s *Server) resolveSibling(ctx context.Context, slug string) string {
	url, _ := s.resolveSiblingFull(ctx, slug)
	return url
}

// resolveSiblingFull is resolveSibling with the app's public URL as well.
func (s *Server) resolveSiblingFull(ctx context.Context, slug string) (string, string) {
	if !s.core.Configured() {
		return "", ""
	}
	s.coreMu.Lock()
	if r, ok := s.resolved[slug]; ok && time.Since(r.at) < time.Minute {
		s.coreMu.Unlock()
		return r.url, r.public
	}
	s.coreMu.Unlock()
	url, public := "", ""
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if res, err := s.core.Resolve(cctx, slug); err == nil {
		url, public = res.URL, res.PublicURL
	}
	s.coreMu.Lock()
	s.resolved[slug] = resolvedURL{url: url, public: public, at: time.Now()}
	s.coreMu.Unlock()
	return url, public
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_day_info Build info.\n# TYPE pocket_day_info gauge\npocket_day_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_day_uptime_seconds Process uptime.\n# TYPE pocket_day_uptime_seconds gauge\npocket_day_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_day_http_requests_total HTTP requests served.\n# TYPE pocket_day_http_requests_total counter\npocket_day_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_day_http_errors_total HTTP 5xx responses.\n# TYPE pocket_day_http_errors_total counter\npocket_day_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_day_rows Rows per table.\n# TYPE pocket_day_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_day_rows{table=%q} %d\n", k, v)
	}
	fmt.Fprintf(w, "# HELP pocket_day_backfill_running Whether a backfill is running.\n# TYPE pocket_day_backfill_running gauge\npocket_day_backfill_running %d\n", boolInt(s.anyBackfillRunning()))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
