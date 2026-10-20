package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// ---- third-party service sources ----
//
// The sources that are not Pocket apps (Immich, Maloja, Dawarich, Jellystat,
// PinePods, Audiobookshelf, Jellyseerr, Forgejo, Nextcloud) have
// the same three modes as the Pocket ones (see siblings.go):
//
//   - core: Day asks Pocket Core for the service's address and the person's
//     credential there (a "service connection": POST /services/credential).
//     The SDK reuses the answer for a minute (a pull, or a page of photo
//     thumbnails, asks once) and Test connection always asks afresh; Day
//     itself never keeps the key. The person's own key
//     comes back for anyone who has one in Core; the box's key only when the
//     person is an admin. A member without an account of their own there is
//     "not connected" (no_account) and the source is skipped.
//   - direct: the URL and key typed into Settings, as before.
//   - off: not pulled.
//
// Settings that are not credentials (the Jellystat and Audiobookshelf user
// filters, the Forgejo user name, public URLs) apply in every mode.

// Service links in Pocket Core: target "service:<type>", one scope.
const (
	servicePrefix = "service:"
	serviceScope  = "account"
)

// serviceSource is a source that is a third-party service.
type serviceSource struct {
	typ      string // the Pocket Core catalog type
	app      string // display name
	reason   string // why, shown in Pocket Core
	keyField string // the catalog field Day uses as the key ("" = keyless)
}

// serviceSourceOrder is the order the services are declared in.
var serviceSourceOrder = []string{"immich", "maloja", "dawarich", "jellystat", "pinepods", "audiobookshelf", "jellyseerr", "forgejo", "nextcloud"}

var serviceSources = map[string]serviceSource{
	"immich":         {typ: "immich", app: "Immich", reason: "Photos and videos of the day", keyField: "API_KEY"},
	"maloja":         {typ: "maloja", app: "Maloja", reason: "What played"},
	"dawarich":       {typ: "dawarich", app: "Dawarich", reason: "Where you went", keyField: "API_KEY"},
	"jellystat":      {typ: "jellystat", app: "Jellystat", reason: "What you watched", keyField: "API_KEY"},
	"pinepods":       {typ: "pinepods", app: "PinePods", reason: "Podcast episodes you played", keyField: "API_KEY"},
	"audiobookshelf": {typ: "audiobookshelf", app: "Audiobookshelf", reason: "Audiobooks you listened to", keyField: "TOKEN"},
	"jellyseerr":     {typ: "jellyseerr", app: "Jellyseerr", reason: "Films and series you requested", keyField: "API_KEY"},
	"forgejo":        {typ: "forgejo", app: "Forgejo", reason: "What you pushed", keyField: "TOKEN"},
	"nextcloud":      {typ: "nextcloud", app: "Nextcloud", reason: "Tasks done, events and birthdays", keyField: "APP_PASSWORD"},
}

// serviceUses is the manifest's "services": one per service source.
func serviceUses() []core.ServiceUse {
	out := make([]core.ServiceUse, 0, len(serviceSourceOrder))
	for _, n := range serviceSourceOrder {
		sv := serviceSources[n]
		out = append(out, core.ServiceUse{Type: sv.typ, Reason: sv.reason})
	}
	return out
}

// serviceCred is how one fetch reaches a service: the address, the key and
// (Nextcloud) the user name, from Settings or from Pocket Core.
type serviceCred struct {
	URL       string
	Key       string
	User      string
	PublicURL string
	Via       string // core | direct
	Source    string // through Pocket Core: personal | box
}

// seenSvc is what Day last heard from Core about a person's service, for
// the status line and the ↗ links; never the key.
type seenSvc struct {
	source, url, public string
}

type serviceMemo struct {
	mu   sync.Mutex
	seen map[string]seenSvc
}

func (s *Server) seenService(userID int64, name string) seenSvc {
	s.svc.mu.Lock()
	defer s.svc.mu.Unlock()
	return s.svc.seen[fmt.Sprintf("%d:%s", userID, name)]
}

func (s *Server) rememberService(userID int64, name string, v seenSvc) {
	s.svc.mu.Lock()
	defer s.svc.mu.Unlock()
	if s.svc.seen == nil {
		s.svc.seen = map[string]seenSvc{}
	}
	s.svc.seen[fmt.Sprintf("%d:%s", userID, name)] = v
}

// serviceConn resolves a person's connection to a service for one fetch.
// Errors wrapping pocket.ErrNotConnected mean "skip, nothing failed".
// Through Pocket Core the SDK reuses the answer for a minute (and rides out
// Core being briefly unreachable); serviceConnFresh skips that.
func (s *Server) serviceConn(ctx context.Context, u *store.User, name string, src store.Source, mode string) (*serviceCred, error) {
	return s.resolveService(ctx, u, name, src, mode, false)
}

// serviceConnFresh is serviceConn asking Core right now (Test connection).
func (s *Server) serviceConnFresh(ctx context.Context, u *store.User, name string, src store.Source, mode string) (*serviceCred, error) {
	return s.resolveService(ctx, u, name, src, mode, true)
}

func (s *Server) resolveService(ctx context.Context, u *store.User, name string, src store.Source, mode string, fresh bool) (*serviceCred, error) {
	sv := serviceSources[name]
	switch mode {
	case modeOff:
		return nil, fmt.Errorf("%w: %s is switched off in Settings → Sources", pocket.ErrNotConnected, sv.app)
	case modeDirect:
		if d := s.directDetail(ctx, name, src); d != "" {
			return nil, fmt.Errorf("%w: %s: %s; add it in Settings → Sources, or switch to Pocket Core", pocket.ErrNotConnected, sv.app, d)
		}
		return &serviceCred{URL: src.URL, Key: src.Key, User: src.Extra, PublicURL: src.PublicURL, Via: modeDirect}, nil
	}
	if !s.core.Configured() || u == nil || u.Email == "" {
		return nil, fmt.Errorf("%w: there is no Pocket Core on this box; use a direct connection to %s", pocket.ErrNotConnected, sv.app)
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ask := s.core.ServiceFor
	if fresh {
		ask = s.core.ServiceForFresh
	}
	c, err := ask(cctx, u.Email, sv.typ)
	if err != nil {
		var le *core.LinkError
		if errors.As(err, &le) {
			return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(sv.app, le.Reason), le.Message, le.Reason))
		}
		if errors.Is(err, core.ErrNotConfigured) {
			return nil, fmt.Errorf("%w: there is no Pocket Core on this box", pocket.ErrNotConnected)
		}
		return nil, fmt.Errorf("pocket core: %w", err)
	}
	out := &serviceCred{URL: strings.TrimRight(c.URL, "/"), PublicURL: firstNonEmpty(src.PublicURL, strings.TrimRight(c.PublicURL, "/")), Via: modeCore, Source: c.Source}
	if sv.keyField != "" {
		out.Key = firstNonEmpty(c.Fields[sv.keyField], c.Key)
	}
	if name == "nextcloud" {
		// the user name is part of the credential: Core's, else the one typed here
		out.User = firstNonEmpty(c.Fields["USERNAME"], src.Extra)
	}
	s.rememberService(u.ID, name, seenSvc{source: c.Source, url: out.URL, public: strings.TrimRight(c.PublicURL, "/")})
	switch {
	case out.URL == "":
		return nil, fmt.Errorf("%w: Pocket Core has no address for %s", pocket.ErrNotConnected, sv.app)
	case sv.keyField != "" && out.Key == "":
		return nil, fmt.Errorf("%w: Pocket Core has no key for you in %s", pocket.ErrNotConnected, sv.app)
	case name == "nextcloud" && out.User == "":
		return nil, fmt.Errorf("%w: Pocket Core has no Nextcloud user name for you", pocket.ErrNotConnected)
	}
	return out, nil
}

// serviceClient builds the concrete fetcher over one resolved connection.
// Non-credential settings (user filters, the Forgejo user) come from src.
func serviceClient(name string, c *serviceCred, src store.Source) sources.Fetcher {
	switch name {
	case "immich":
		return sources.NewImmich(c.URL, c.Key)
	case "maloja":
		return sources.NewMaloja(c.URL)
	case "dawarich":
		return sources.NewDawarich(c.URL, c.Key)
	case "jellystat":
		return sources.NewJellystat(c.URL, c.Key, src.Extra)
	case "pinepods":
		return sources.NewPinePods(c.URL, c.Key)
	case "audiobookshelf":
		a := sources.NewAudiobookshelf(c.URL, c.Key, src.Extra)
		// the person's own key in Core is their own ABS account: their sessions
		a.Own = c.Via == modeCore && c.Source == "personal"
		return a
	case "jellyseerr":
		return sources.NewJellyseerr(c.URL, c.Key)
	case "forgejo":
		return sources.NewForgejo(c.URL, c.Key, src.Extra)
	case "nextcloud":
		return sources.NewNextcloud(c.URL, c.Key, c.User)
	}
	return nil
}

// serviceFetcher resolves the connection on every Fetch and Test. A Fetch
// through Core uses the SDK's one-minute credential cache, so a switch flipped
// in Pocket Core (or a key changed there) applies within a minute; Test asks
// Core afresh.
type serviceFetcher struct {
	s    *Server
	u    *store.User
	name string
	src  store.Source
	mode string
}

func (s *Server) serviceFetcher(u *store.User, name string, src store.Source, mode string) *serviceFetcher {
	return &serviceFetcher{s: s, u: u, name: name, src: src, mode: mode}
}

func (f *serviceFetcher) Name() string { return f.name }

func (f *serviceFetcher) client(ctx context.Context, fresh bool) (sources.Fetcher, *serviceCred, error) {
	c, err := f.s.resolveService(ctx, f.u, f.name, f.src, f.mode, fresh)
	if err != nil {
		return nil, nil, err
	}
	return serviceClient(f.name, c, f.src), c, nil
}

func (f *serviceFetcher) Fetch(ctx context.Context, date string, loc *time.Location) (*sources.Result, error) {
	cl, _, err := f.client(ctx, false)
	if err != nil {
		return nil, err
	}
	return cl.Fetch(ctx, date, loc)
}

// Test is "Test connection": it asks Core afresh, never the cached answer.
func (f *serviceFetcher) Test(ctx context.Context) (string, error) {
	cl, c, err := f.client(ctx, true)
	if err != nil {
		return "", err
	}
	msg, err := cl.Test(ctx)
	if err != nil {
		return "", err
	}
	return msg + viaText(c), nil
}

// viaText is the end of a Test message: how the source was reached.
func viaText(c *serviceCred) string {
	if c.Via != modeCore {
		return ", direct connection"
	}
	return " through Pocket Core" + keySourceText(c.Source)
}

func keySourceText(src string) string {
	switch src {
	case "personal":
		return " (your own key)"
	case "box":
		return " (the box's key)"
	}
	return ""
}

// probeKeySources fills in, for the Settings page, whose key the services in
// Pocket Core mode come with when Day has not asked Core since it started.
// The answer's key is dropped on the spot.
func (s *Server) probeKeySources(ctx context.Context, u *store.User, st store.Settings, states map[string]sourceState) {
	if u == nil {
		return
	}
	todo := map[string]sourceState{}
	for _, n := range serviceSourceOrder {
		if ss := states[n]; ss.Via == modeCore && ss.KeySource == "" {
			todo[n] = ss
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for n, ss := range todo {
		wg.Add(1)
		go func(n string, ss sourceState) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			_, err := s.serviceConn(cctx, u, n, st.Sources[n], modeCore)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if errors.Is(err, pocket.ErrNotConnected) {
					// switched off (or the key went) since the status was read
					ss.Via, ss.Active = "", false
					ss.Detail = strings.TrimPrefix(err.Error(), pocket.ErrNotConnected.Error()+": ")
					states[n] = ss
				}
				return
			}
			ss.KeySource = s.seenService(u.ID, n).source
			states[n] = ss
		}(n, ss)
	}
	wg.Wait()
}
