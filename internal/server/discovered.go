package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// ---- sources apps bring themselves ----
//
// Every source above is written out in Day: its Core link (siblings.go), its
// fetcher, its card. An app that offers the "day" capability (Pocket Core,
// docs/capabilities.md) needs none of that. It says in its own manifest that
// it can tell what happened on a day and answers on one fixed route in one
// shape; Day asks Pocket Core who offers it for the person
// (UseEvery(pocket:day) in the manifest) and reads each one through the
// generic fetcher (sources.Capability). The source's name is the app's slug.
//
// The hand-written sources stay as they are: an app that already has one is
// left out here, so its stored days and its own card keep working.
//
// A discovered source differs from the others in three ways:
//   - it exists per person and only while Core lists it, so nothing about it
//     is in store.SourceNames or the tables next to it;
//   - it comes through Pocket Core only: there is no Direct mode, the person
//     can only switch it off in Settings → Sources (src_<name>_enabled);
//   - its label, the app's name and icon and how its things are counted come
//     from the app. They are kept with the person's facts (source_meta), so a
//     day from long ago still reads well when the app is gone.

// everyDayReason is why Day asks for the day of every app, shown in Pocket Core.
const everyDayReason = "Write what happened in each of your apps into your journal, day by day"

// discovered is one app that offers its day, as it stands for a person.
type discovered struct {
	name   string // the source's id: the app's slug in Pocket Core
	meta   store.SourceMeta
	on     bool   // the link works for the person now
	state  string // why not (Core's word)
	detail string
}

const (
	discoveryTTL     = 60 * time.Second // how long a person's list is reused
	discoveryFailTTL = 15 * time.Second // …and how long "Core did not answer" is
	discoveryMax     = 500              // people kept; past it the oldest lists go
)

// discoveryCache keeps each person's list for a minute: the day page, the
// pull and Settings all need it, and none of them should wait on Core for it.
type discoveryCache struct {
	mu sync.Mutex
	m  map[string]discoveryEntry // by email
}

type discoveryEntry struct {
	until time.Time
	list  []discovered
}

func (c *discoveryCache) get(email string) ([]discovered, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[email]
	if !ok || time.Now().After(e.until) {
		return nil, false
	}
	return e.list, true
}

func (c *discoveryCache) put(email string, list []discovered, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]discoveryEntry{}
	}
	if len(c.m) >= discoveryMax {
		now := time.Now()
		for k, e := range c.m {
			if now.After(e.until) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= discoveryMax { // everybody is fresh: start over rather than grow
			c.m = map[string]discoveryEntry{}
		}
	}
	c.m[email] = discoveryEntry{until: time.Now().Add(ttl), list: list}
}

// forget drops one person's list (or everybody's with ""), so the next look asks Core.
func (c *discoveryCache) forget(email string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if email == "" {
		c.m = nil
		return
	}
	delete(c.m, strings.ToLower(email))
}

// handWritten are the Core slugs of the apps Day already reads with a source
// of its own: the Pocket app sources (contacts and money among them) and the
// six apps that feed a source (movement, music, audiobooks, requests, watch,
// weather). They are not discovered a second time. Pocket Photos is one of
// them too: it shows the Immich library the "immich" source already reads, so
// its day would put the same photos on the page twice.
var handWritten = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{"day": true, "photos": true} // never itself
	for _, ps := range pocketSources {
		out[ps.slug] = true
	}
	for _, ps := range []pocketSource{movementApp, musicApp, audiobooksApp, requestsApp, watchApp, weatherApp} {
		out[ps.slug] = true
	}
	return out
})

// builtinSource are the names of Day's own sources, with the one that was
// renamed ("paperless" became "documents"): people still have its settings
// saved, and they are not the switch of an app that came along later.
var builtinSource = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{"paperless": true}
	for _, n := range store.SourceNames {
		out[n] = true
	}
	return out
})

// discoveredNameRe is what a discovered source may be called: a slug without
// "_" (settings are stored as src_<name>_<what>) that is safe in an address
// and in the page.
var discoveredNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// discoverable reports whether an app's slug can be a discovered source.
func discoverable(slug string) bool {
	return discoveredNameRe.MatchString(slug) && !handWritten()[slug] && !builtinSource()[slug]
}

// discoveredMeta is what Day shows for an app that offers its day: the label
// it gave (else its name without the family word), its name and icon, the
// words for its things and what its number measures.
func discoveredMeta(a core.CapableApp) store.SourceMeta {
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = strings.ToUpper(a.App[:1]) + a.App[1:]
	}
	m := store.SourceMeta{App: name, Icon: strings.TrimSpace(a.Icon), URL: strings.TrimRight(strings.TrimSpace(a.PublicURL), "/"), Unit: "count"}
	if d := a.Day; d != nil {
		m.Label, m.One, m.Many, m.What = strings.TrimSpace(d.Label), strings.TrimSpace(d.One), strings.TrimSpace(d.Many), strings.TrimSpace(d.What)
		switch d.Unit {
		case "min", "amount", "qty":
			m.Unit = d.Unit
		}
	}
	if m.Label == "" {
		m.Label = strings.TrimSpace(strings.TrimPrefix(name, "Pocket "))
	}
	if m.Label == "" {
		m.Label = name
	}
	if m.One == "" || m.Many == "" {
		m.One, m.Many = "item", "items"
	}
	if !strings.HasPrefix(m.Icon, "http://") && !strings.HasPrefix(m.Icon, "https://") {
		m.Icon = "" // only an address a browser can load
	}
	return m
}

// activityMeta is a discovered source for the code that reads Activity
// payloads (briefs, recaps, mood patterns).
func activityMeta(m store.SourceMeta) sources.ActivityMeta {
	verb := "with something in " + m.App
	if m.Many != "" && m.Many != "items" {
		verb = "with " + m.Many
	}
	return sources.ActivityMeta{Label: m.Label, One: m.One, Many: m.Many, Unit: m.Unit, Verb: verb, Group: "kinds"}
}

// discovered lists the apps that offer their day to this person and are not
// behind a hand-written source, by name. It asks Pocket Core at most once a
// minute per person. Without Pocket Core, when Core does not answer, or with
// a Core from before capabilities, the list is empty and Day is as it was.
func (s *Server) discovered(ctx context.Context, u *store.User) []discovered {
	if u == nil || u.Email == "" || !s.core.Configured() {
		return nil
	}
	key := strings.ToLower(u.Email)
	if list, ok := s.discovery.get(key); ok {
		return list
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	apps, err := s.core.Capable(cctx, u.Email, core.ScopeDay)
	if err != nil {
		s.log.Debug("pocket core: who offers a day", "user", u.ID, "err", err)
		s.discovery.put(key, nil, discoveryFailTTL)
		return nil
	}
	list := []discovered{}
	seen := map[string]bool{}
	for _, a := range apps {
		if !discoverable(a.App) || seen[a.App] {
			continue
		}
		seen[a.App] = true
		list = append(list, discovered{name: a.App, meta: discoveredMeta(a), on: a.On, state: a.State, detail: a.Detail})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
	s.discovery.put(key, list, discoveryTTL)
	return list
}

// discoveredState is how a discovered source stands for a person: pulled
// while its link is on and the person has not switched it off here.
func discoveredState(d discovered, st store.Settings) sourceState {
	ss := sourceState{Mode: modeCore}
	switch {
	case !st.DiscoveredOn(d.name):
		ss.Mode, ss.Detail = modeOff, "switched off"
	case d.on:
		ss.Via = modeCore
	default:
		ss.Detail = firstNonEmpty(linkReason(d.meta.App, d.state), d.detail, d.state, "the link is off in Pocket Core")
	}
	ss.Active = ss.Via != ""
	return ss
}

// namedZone gives the local time zone its name when the box says it (TZ), so
// the app asked can measure the same day; time.Local alone has none.
func namedZone(loc *time.Location) *time.Location {
	if loc == nil || loc.String() != "Local" {
		return loc
	}
	if tz := os.Getenv("TZ"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	return loc
}

// broughtFetcher is the fetcher of a discovered source with what the app said
// about it, which the pull keeps next to the facts it writes.
type broughtFetcher struct {
	*sources.Capability
	meta store.SourceMeta
}

// capabilityFetcher builds the fetcher of a discovered source: every fetch
// takes a fresh link token from Core (DayIn), so a link switched off stops
// the next one.
func (s *Server) capabilityFetcher(u *store.User, d discovered) *broughtFetcher {
	email, app := u.Email, d.meta.App
	notConnected := func(le *core.LinkError) error {
		return fmt.Errorf("%w: %s", pocket.ErrNotConnected, firstNonEmpty(linkReason(app, le.Reason), le.Message, le.Reason))
	}
	return &broughtFetcher{meta: d.meta, Capability: &sources.Capability{
		Source: d.name, App: app, Meta: activityMeta(d.meta),
		Day: func(ctx context.Context, date string, loc *time.Location) (*core.Day, error) {
			day, err := s.core.DayIn(ctx, email, d.name, date, namedZone(loc))
			var le *core.LinkError
			var ce *core.CapabilityError
			switch {
			case errors.As(err, &le):
				return nil, notConnected(le)
			case errors.Is(err, core.ErrNotConfigured):
				return nil, fmt.Errorf("%w: there is no Pocket Core on this box", pocket.ErrNotConnected)
			case errors.As(err, &ce):
				return nil, fmt.Errorf("%s (HTTP %d)", ce.Message, ce.Status)
			}
			return day, err
		},
		Check: func(ctx context.Context) (string, error) {
			// the link as Core has it now, not as it was a minute ago
			s.discovery.forget(email)
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			apps, err := s.core.Capable(cctx, email, core.ScopeDay)
			if err != nil {
				return "", fmt.Errorf("Pocket Core did not answer: %w", err)
			}
			for _, a := range apps {
				if a.App != d.name {
					continue
				}
				if !a.On {
					return "", errors.New(firstNonEmpty(linkReason(app, a.State), a.Detail, a.State, "the link is off in Pocket Core"))
				}
				return app + " is connected through Pocket Core", nil
			}
			return "", fmt.Errorf("Pocket Core does not list %s for you any more", app)
		},
	}}
}

// sourceMetas is what Day knows about the sources apps brought, for one
// person: the ones Core lists now, and the ones kept with their facts (an app
// that is gone, or a Core that is away, still has its days here).
func (s *Server) sourceMetas(ctx context.Context, u *store.User) map[string]store.SourceMeta {
	out := map[string]store.SourceMeta{}
	if u == nil {
		return out
	}
	if kept, err := s.store.SourceMetas(ctx, u.ID); err == nil {
		for n, m := range kept {
			if discoveredNameRe.MatchString(n) && !builtinSource()[n] {
				out[n] = m
			}
		}
	}
	for _, d := range s.discovered(ctx, u) {
		out[d.name] = d.meta
	}
	return out
}

// keptActivities are the person's discovered sources as activity sources, from
// what was kept with their facts: for the code that reads stored days (the
// brief, the year, the mood patterns). It never asks Core.
func (s *Server) keptActivities(ctx context.Context, userID int64) sources.More {
	kept, err := s.store.SourceMetas(ctx, userID)
	if err != nil || len(kept) == 0 {
		return nil
	}
	out := sources.More{}
	for n, m := range kept {
		if discoveredNameRe.MatchString(n) && !builtinSource()[n] {
			out[n] = activityMeta(m)
		}
	}
	return out
}

// discoveredNames is the sorted names of a meta map: the order discovered
// sources are listed in everywhere, after Day's own.
func discoveredNames(metas map[string]store.SourceMeta) []string {
	out := make([]string, 0, len(metas))
	for n := range metas {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
