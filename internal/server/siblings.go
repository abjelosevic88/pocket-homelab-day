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
// The sources that are Pocket apps (Habits, Bookshelf, Travel, Kitchen,
// Garage, Invoicing, Split, Inventory and Money) are reached per person in
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
var pocketSourceOrder = []string{"habits", "bookshelf", "travel", "kitchen", "garage", "invoicing", "split", "inventory", "firefly"}

var pocketSources = map[string]pocketSource{
	"habits":    {slug: "habits", app: "Pocket Habits", scope: "habits:read", reason: "Your habits in the journal"},
	"bookshelf": {slug: "bookshelf", app: "Pocket Bookshelf", scope: "books:read", reason: "Books you read"},
	"travel":    {slug: "travel", app: "Pocket Travel", scope: "trips:read", reason: "Trips"},
	"kitchen":   {slug: "kitchen", app: "Pocket Kitchen", scope: "cooklog:read", reason: "What you cooked"},
	"garage":    {slug: "garage", app: "Pocket Garage", scope: "vehicles:read", reason: "Car costs and fill-ups"},
	"invoicing": {slug: "invoicing", app: "Pocket Invoicing", scope: "invoices:read", reason: "Invoices and payments"},
	"split":     {slug: "split", app: "Pocket Split", scope: "groups:read", reason: "Shared expenses"},
	"inventory": {slug: "inventory", app: "Pocket Inventory", scope: "activity:read", reason: "Inventory activity"},
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
	return out
}

// Connection modes.
const (
	modeCore   = "core"
	modeDirect = "direct"
	modeOff    = "off"
)

func validMode(m string) bool { return m == modeCore || m == modeDirect || m == modeOff }

// connMode is the mode in effect for a Pocket app source: the chosen one;
// else off when the source was switched off before modes existed, direct
// when a token is saved (pasted tokens keep working until the person
// switches), Pocket Core when the box has it, and direct otherwise.
func connMode(src store.Source, coreOK bool) string {
	if validMode(src.Mode) {
		return src.Mode
	}
	switch {
	case !src.Enabled:
		return modeOff
	case src.Key != "":
		return modeDirect
	case coreOK:
		return modeCore
	}
	return modeDirect
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
	}
	return ""
}

// sourceState is how one source stands for a person.
type sourceState struct {
	Active bool   // pulled
	Mode   string // Pocket app sources: the mode in effect
	Via    string // Pocket app sources: "core" | "direct" | "" (not connected)
	Detail string // Pocket app sources: why not connected
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

// sourceStates evaluates every source for a person, with at most one call
// to Pocket Core (only when a Pocket app source is in Pocket Core mode).
func (s *Server) sourceStates(ctx context.Context, u *store.User, st store.Settings) map[string]sourceState {
	out := make(map[string]sourceState, len(store.SourceNames))
	coreOK := s.core.Configured()
	var links map[string]core.LinkState
	var linkErr error
	asked := false
	for _, n := range store.SourceNames {
		src := st.Sources[n]
		ps, isPocket := pocketSources[n]
		if !isPocket {
			active := src.Configured(n)
			if n == "weather" {
				active = src.Enabled && (st.HomeLat != 0 || st.HomeLon != 0)
			}
			out[n] = sourceState{Active: active}
			continue
		}
		ss := sourceState{Mode: connMode(src, coreOK)}
		switch ss.Mode {
		case modeOff:
			ss.Detail = "switched off"
		case modeDirect:
			switch {
			case src.Key == "":
				ss.Detail = "no token yet"
			case src.URL == "" && s.resolveSibling(ctx, ps.slug) == "":
				ss.Detail = "no URL yet"
			default:
				ss.Via = modeDirect
			}
		default:
			if u == nil || !coreOK {
				ss.Detail = "there is no Pocket Core on this box; use a direct connection"
				break
			}
			if !asked {
				links, linkErr = s.linkStates(ctx, u.Email)
				asked = true
			}
			if linkErr != nil {
				ss.Detail = "Pocket Core did not answer: " + linkErr.Error()
				break
			}
			l, ok := links[ps.slug+" "+ps.scope]
			switch {
			case ok && l.On:
				ss.Via = modeCore
			case !ok && len(links) == 0:
				ss.Detail = "Pocket Core has no Day links for you: your email is not a person there, or this Day version has not registered yet (it does within a minute of starting)"
			case !ok:
				ss.Detail = linkReason(ps.app, "not_declared")
			default:
				ss.Detail = firstNonEmpty(linkReason(ps.app, l.State), l.Detail, l.State)
			}
		}
		ss.Active = ss.Via != ""
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
	case "habits":
		return sources.NewHabits(conn)
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
	case "firefly":
		return sources.NewFirefly(conn)
	}
	return nil
}

// sourceLink is what a browser opens for "open in …" on a day: the public
// URL set for the source, else where Pocket Core says the app lives, else
// the source URL when it is not an internal one.
func (s *Server) sourceLink(ctx context.Context, name string, src store.Source, ss sourceState) string {
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
	u := src.URL
	if isPocket && ps.prefix != "" {
		u = strings.TrimSuffix(u, ps.prefix)
	}
	if u != "" && !internal(u) {
		return u
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
