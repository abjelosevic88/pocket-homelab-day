package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// ---- Pocket app sources ----
//
// (The third-party services, Immich, Maloja, Dawarich…, use the same three
// modes through Pocket Core service connections; see services.go.)
//
// The sources that are Pocket apps (Habits, Bookshelf, Travel, Kitchen,
// Garage, Invoicing, Split, Inventory, Documents and Money) are reached per person in
// one of three ways, chosen in Settings → Sources:
//
//   - core: Day acts for the person through a Pocket Core app link. Every
//     fetch asks Core for a fresh 5-minute pcx_ token, so switching the link
//     off in Core stops the next fetch. Nothing to paste, and every person
//     on the box gets their own data.
//   - direct: the URL and API token typed into Settings (how it always worked,
//     and the only way without Pocket Core).
//   - off: not pulled.
//
// Not chosen yet: direct when a token is saved, else core when this box has
// Pocket Core, else direct.

// pocketSource is a source that is a Pocket app.
type pocketSource struct {
	slug   string // the app in Pocket Core
	app    string // display name
	scope  string // what Day asks for
	reason string // why, shown in Pocket Core
	prefix string // path under the app's URL the source talks to
}

// pocketSourceOrder is the order the uses are declared in.
var pocketSourceOrder = []string{"podcasts", "habits", "health", "still", "horoscope", "bookshelf", "travel", "kitchen", "garage", "invoicing", "split", "inventory", "documents", "tasks", "projects", "firefly"}

var pocketSources = map[string]pocketSource{
	"podcasts":  {slug: "podcasts", app: "Pocket Podcasts", scope: "history:read", reason: "Podcast listening time in your journal"},
	"tasks":     {slug: "tasks", app: "Pocket Tasks", scope: "tasks:read", reason: "Tasks you completed in your journal"},
	"projects":  {slug: "projects", app: "Pocket Projects", scope: "projects:read", reason: "Your work on issues, day by day, in your journal"},
	"habits":    {slug: "habits", app: "Pocket Habits", scope: "habits:read", reason: "Your habits in the journal"},
	"health":    {slug: "health", app: "Pocket Health", scope: "health:read", reason: "Steps, sleep, workouts, heart and weight in your journal"},
	"still":     {slug: "still", app: "Pocket Still", scope: "mindfulness:read", reason: "Breathing and meditation practice in your journal"},
	"horoscope": {slug: "horoscope", app: "Pocket Horoscope", scope: "horoscope:read", reason: "The Moon, sky events and your daily horoscope and mood check-in in your journal"},
	"bookshelf": {slug: "bookshelf", app: "Pocket Bookshelf", scope: "books:read", reason: "Books you read"},
	"travel":    {slug: "travel", app: "Pocket Travel", scope: "trips:read", reason: "Trips"},
	"kitchen":   {slug: "kitchen", app: "Pocket Kitchen", scope: "cooklog:read", reason: "What you cooked"},
	"garage":    {slug: "garage", app: "Pocket Garage", scope: "vehicles:read", reason: "Car costs and fill-ups"},
	"invoicing": {slug: "invoicing", app: "Pocket Invoicing", scope: "invoices:read", reason: "Invoices and payments"},
	"split":     {slug: "split", app: "Pocket Split", scope: "groups:read", reason: "Shared expenses"},
	"inventory": {slug: "inventory", app: "Pocket Inventory", scope: "activity:read", reason: "Inventory activity"},
	// Pocket Documents replaced Paperless-ngx (a service source until then).
	"documents": {slug: "documents", app: "Pocket Documents", scope: "documents:read", reason: "Documents you added on a day, in your journal"},
	// The money source keeps its old id so the days frozen while it read
	// Firefly III stay as they are; it now reads Pocket Money's
	// Firefly-compatible API.
	"firefly": {slug: "money", app: "Pocket Money", scope: "transactions:read", reason: "Money spent and received", prefix: "/firefly"},
}

// pocketUses is the manifest's "uses": one per Pocket app source.
func pocketUses() []core.Use {
	out := make([]core.Use, 0, len(pocketSourceOrder))
	for _, n := range pocketSourceOrder {
		ps := pocketSources[n]
		out = append(out, core.Use{App: ps.slug, Scopes: []string{ps.scope}, Reason: ps.reason})
	}
	// Pocket Movement is not a source of its own: it feeds the location source (movement.go).
	// Pocket Music feeds the music source the same way (music.go), Pocket Audiobooks the audiobooks one (audiobooks.go),
	// Pocket Requests the requests one (requests.go).
	return append(out, core.Use{App: movementApp.slug, Scopes: []string{movementApp.scope}, Reason: movementApp.reason},
		core.Use{App: musicApp.slug, Scopes: []string{musicApp.scope}, Reason: musicApp.reason},
		core.Use{App: audiobooksApp.slug, Scopes: []string{audiobooksApp.scope}, Reason: audiobooksApp.reason},
		core.Use{App: requestsApp.slug, Scopes: []string{requestsApp.scope}, Reason: requestsApp.reason})
}

// Connection modes.
const (
	modeCore   = "core"
	modeDirect = "direct"
	modeOff    = "off"
)

func validMode(m string) bool { return m == modeCore || m == modeDirect || m == modeOff }

// connMode is the mode in effect for a source reached through Pocket Core or
// directly (a Pocket app or a third-party service): the chosen one; else off
// when the source was switched off before modes existed, direct when a
// credential is saved (pasted tokens and keys keep working until the person
// switches; for keyless Maloja a saved URL counts), Pocket Core when the box
// has it, and direct otherwise.
func connMode(name string, src store.Source, coreOK bool) string {
	if validMode(src.Mode) {
		return src.Mode
	}
	switch {
	case !src.Enabled:
		return modeOff
	case hasSavedCred(name, src):
		return modeDirect
	case coreOK:
		return modeCore
	}
	return modeDirect
}

// hasSavedCred reports whether a direct connection was set up for the source.
func hasSavedCred(name string, src store.Source) bool {
	if _, ok := serviceSources[name]; ok && !store.NeedsKey(name) {
		return src.URL != ""
	}
	return src.Key != ""
}

// connTarget is what a connectable source is in Pocket Core: the link's
// target ("habits", or "service:immich" for a third-party service) and scope.
type connTarget struct {
	target  string
	scope   string
	app     string // display name
	service bool
}

// connectable returns how a source is reached through Pocket Core; false for
// the sources that never are (weather).
func connectable(name string) (connTarget, bool) {
	if ps, ok := pocketSources[name]; ok {
		return connTarget{target: ps.slug, scope: ps.scope, app: ps.app}, true
	}
	if sv, ok := serviceSources[name]; ok {
		return connTarget{target: servicePrefix + sv.typ, scope: serviceScope, app: sv.app, service: true}, true
	}
	return connTarget{}, false
}

func linkReason(app, reason string) string {
	switch reason {
	case "off_person":
		return "you switched the Day → " + app + " link off in Pocket Core (Settings → Apps acting for me)"
	case "off_admin":
		return "the Day → " + app + " link is switched off in Pocket Core (Links)"
	case "target_missing":
		return app + " is not registered with Pocket Core"
	case "not_offered":
		return "this " + app + " version does not accept app links from Day yet"
	case "not_declared":
		return "Pocket Core has not seen this Day version yet; it re-registers within a minute of starting"
	case "not_in_audience":
		return "you do not have access to " + app + " in Pocket Core"
	case "unknown_user":
		return "your email is not a person in Pocket Core"
	case "no_account":
		return "you have no account of your own in " + app + " yet; ask the admin or let a connector create one"
	}
	return ""
}

// sourceState is how one source stands for a person.
type sourceState struct {
	Active bool   // pulled
	Mode   string // connectable sources: the mode in effect
	Via    string // connectable sources: "core" | "direct" | "" (not connected)
	Detail string // connectable sources: why not connected
	// KeySource is whose key a service comes with through Pocket Core:
	// "personal" (the person's own) or "box" (the box-wide one, admins only);
	// "" until Day has asked Core for it once.
	KeySource string
	// The location source (movement.go): the provider in effect ("movement"
	// | "dawarich"), how Pocket Movement and Dawarich are each reached
	// ("core" / "core" | "direct" | "") and why Pocket Movement is not.
	Provider       string
	MovementVia    string
	MovementDetail string
	DawarichVia    string
	// The music source (music.go): Pocket Music's and Maloja's connections.
	MusicVia    string
	MusicDetail string
	MalojaVia   string
	// The audiobooks source (audiobooks.go): Pocket Audiobooks' and Audiobookshelf's connections.
	AudiobooksVia    string
	AudiobooksDetail string
	ABSVia           string
	// The requests source (requests.go): Pocket Requests' and Jellyseerr's connections.
	RequestsVia    string
	RequestsDetail string
	SeerrVia       string
}

// linkStates asks Pocket Core, once, for every link Day has for the person
// (keyed "app scope"). No token is issued.
func (s *Server) linkStates(ctx context.Context, email string) (map[string]core.LinkState, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	list, err := s.core.LinkStatus(cctx, email, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]core.LinkState, len(list))
	for _, l := range list {
		out[l.App+" "+l.Scope] = l
	}
	return out, nil
}

// directDetail says what a direct connection still lacks ("" = complete).
func (s *Server) directDetail(ctx context.Context, name string, src store.Source) string {
	if ps, ok := pocketSources[name]; ok {
		switch {
		case src.Key == "":
			return "no token yet"
		case src.URL == "" && s.resolveSibling(ctx, ps.slug) == "":
			return "no URL yet"
		}
		return ""
	}
	switch {
	case src.URL == "":
		return "no URL yet"
	case store.NeedsKey(name) && src.Key == "":
		if name == "nextcloud" {
			return "no app password yet"
		}
		return "no key yet"
	case name == "nextcloud" && src.Extra == "":
		return "no user name yet"
	}
	return ""
}

// sourceStates evaluates every source for a person, with at most one call
// to Pocket Core (only when a source is in Pocket Core mode).
func (s *Server) sourceStates(ctx context.Context, u *store.User, st store.Settings) map[string]sourceState {
	out := make(map[string]sourceState, len(store.SourceNames))
	coreOK := s.core.Configured()
	var links map[string]core.LinkState
	var linkErr error
	asked := false
	getLinks := func() (map[string]core.LinkState, error) {
		if !asked {
			links, linkErr = s.linkStates(ctx, u.Email)
			asked = true
		}
		return links, linkErr
	}
	for _, n := range store.SourceNames {
		src := st.Sources[n]
		ct, ok := connectable(n)
		if !ok {
			active := src.Configured(n)
			if n == "weather" {
				active = src.Enabled && (st.HomeLat != 0 || st.HomeLon != 0)
			}
			out[n] = sourceState{Active: active}
			continue
		}
		ss := sourceState{Mode: connMode(n, src, coreOK)}
		switch ss.Mode {
		case modeOff:
			ss.Detail = "switched off"
		case modeDirect:
			if ss.Detail = s.directDetail(ctx, n, src); ss.Detail == "" {
				ss.Via = modeDirect
			}
		default:
			if u == nil || !coreOK {
				ss.Detail = "there is no Pocket Core on this box; use a direct connection"
				break
			}
			if _, linkErr = getLinks(); linkErr != nil {
				ss.Detail = "Pocket Core did not answer: " + linkErr.Error()
				break
			}
			l, ok := links[ct.target+" "+ct.scope]
			switch {
			case ok && l.On:
				ss.Via = modeCore
				if ct.service {
					ss.KeySource = s.seenService(u.ID, n).source
				}
			case !ok && len(links) == 0:
				ss.Detail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
			case !ok:
				ss.Detail = linkReason(ct.app, "not_declared")
			default:
				ss.Detail = firstNonEmpty(linkReason(ct.app, l.State), l.Detail, l.State)
			}
		}
		ss.Active = ss.Via != ""
		if n == "dawarich" {
			applyLocation(&ss, src, coreOK && u != nil, u, getLinks)
		}
		if n == "maloja" {
			applyMusic(&ss, src, coreOK && u != nil, u, getLinks)
		}
		if n == "audiobookshelf" {
			applyAudiobooks(&ss, src, coreOK && u != nil, u, getLinks)
		}
		if n == "jellyseerr" {
			applyRequests(&ss, src, coreOK && u != nil, u, getLinks)
		}
		out[n] = ss
	}
	return out
}

// pocketConn is how a fetch reaches a person's Pocket app: a fixed URL and
// token, or a fresh Pocket Core link token on every call.
func (s *Server) pocketConn(u *store.User, name string, src store.Source, mode string) pocket.Conn {
	ps := pocketSources[name]
	timeout := 20 * time.Second
	if name == "firefly" {
		timeout = 30 * time.Second
	}
	switch mode {
	case modeOff:
		return func(context.Context) (*pocket.Client, error) {
			return nil, fmt.Errorf("%w: %s is switched off in Settings → Sources", pocket.ErrNotConnected, ps.app)
		}
	case modeDirect:
		return func(ctx context.Context) (*pocket.Client, error) {
			base := src.URL
			if base == "" {
				if b := s.resolveSibling(ctx, ps.slug); b != "" {
					base = strings.TrimRight(b, "/") + ps.prefix
				}
			}
			if base == "" || src.Key == "" {
				return nil, fmt.Errorf("%w: add the %s URL and token in Settings → Sources, or switch to Pocket Core", pocket.ErrNotConnected, ps.app)
			}
			return pocket.New(base, src.Key, timeout), nil
		}
	}
	email := ""
	if u != nil {
		email = u.Email
	}
	return func(ctx context.Context) (*pocket.Client, error) {
		if !s.core.Configured() || email == "" {
			return nil, fmt.Errorf("%w: there is no Pocket Core on this box; use a direct connection to %s", pocket.ErrNotConnected, ps.app)
		}
		tok, err := s.core.ActAs(ctx, email, ps.slug, ps.scope)
		if err != nil {
			var le *core.LinkError
			if errors.As(err, &le) {
				return nil, fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(ps.app, le.Reason), le.Message, le.Reason))
			}
			return nil, err
		}
		return pocket.New(strings.TrimRight(tok.URL, "/")+ps.prefix, tok.AccessToken, timeout), nil
	}
}

// pocketFetcher builds the fetcher of a Pocket app source over a connection.
func pocketFetcher(name string, conn pocket.Conn) sources.Fetcher {
	switch name {
	case "podcasts":
		return sources.NewPodcasts(conn)
	case "tasks":
		return sources.NewTasks(conn)
	case "projects":
		return sources.NewProjects(conn)
	case "habits":
		return sources.NewHabits(conn)
	case "health":
		return sources.NewHealth(conn)
	case "still":
		return sources.NewStill(conn)
	case "horoscope":
		return sources.NewHoroscope(conn)
	case "bookshelf":
		return sources.NewBookshelf(conn)
	case "travel":
		return sources.NewTravel(conn)
	case "kitchen":
		return sources.NewKitchen(conn)
	case "garage":
		return sources.NewGarage(conn)
	case "invoicing":
		return sources.NewInvoicing(conn)
	case "split":
		return sources.NewSplit(conn)
	case "inventory":
		return sources.NewInventory(conn)
	case "documents":
		return sources.NewDocuments(conn)
	case "firefly":
		return sources.NewFirefly(conn)
	}
	return nil
}

// sourceLink is what a browser opens for "open in …" on a day: the public
// URL set for the source, else where Pocket Core says the app (or service)
// lives, else the source URL when it is not an internal one.
func (s *Server) sourceLink(ctx context.Context, u *store.User, name string, src store.Source, ss sourceState) string {
	if name == "audiobookshelf" && ss.AudiobooksVia != "" {
		// the days come from Pocket Audiobooks: open it (the public URL set is Audiobookshelf's)
		_, public := s.resolveSiblingFull(ctx, audiobooksApp.slug)
		return public
	}
	if name == "jellyseerr" && ss.RequestsVia != "" {
		// the days come from Pocket Requests: open it (the public URL set is Jellyseerr's)
		_, public := s.resolveSiblingFull(ctx, requestsApp.slug)
		return public
	}
	if name == "maloja" && ss.MusicVia != "" {
		// the days come from Pocket Music: open it (the public URL set is Maloja's)
		_, public := s.resolveSiblingFull(ctx, musicApp.slug)
		return public
	}
	if name == "dawarich" && ss.MovementVia != "" {
		// the days come from Pocket Movement: open it (the public URL set is Dawarich's)
		_, public := s.resolveSiblingFull(ctx, movementApp.slug)
		return public
	}
	if src.PublicURL != "" {
		return src.PublicURL
	}
	internal := func(u string) bool {
		return strings.Contains(u, "host.docker.internal") || strings.Contains(u, "localhost")
	}
	ps, isPocket := pocketSources[name]
	if isPocket && (ss.Via == modeCore || (ss.Via == modeDirect && src.URL == "")) {
		url, public := s.resolveSiblingFull(ctx, ps.slug)
		if public != "" {
			return public
		}
		if url != "" && !internal(url) {
			return url
		}
		return ""
	}
	if _, isService := serviceSources[name]; isService && ss.Via == modeCore {
		if u == nil {
			return ""
		}
		seen := s.seenService(u.ID, name)
		if seen.public != "" {
			return seen.public
		}
		if seen.url != "" && !internal(seen.url) {
			return seen.url
		}
		return ""
	}
	url := src.URL
	if isPocket && ps.prefix != "" {
		url = strings.TrimSuffix(url, ps.prefix)
	}
	if url != "" && !internal(url) {
		return url
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
