package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"

	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// Pocket Movement is the default location provider only with Pocket Core; a
// Dawarich URL and key (from the environment or pasted) do not change that.
func TestLocationProvider(t *testing.T) {
	envDawarich := store.Source{URL: "http://dawarich", Key: "k", Enabled: true}
	cases := []struct {
		src    store.Source
		coreOK bool
		want   string
	}{
		{envDawarich, true, locMovement},
		{envDawarich, false, locDawarich},
		{store.Source{Provider: locDawarich}, true, locDawarich},
		{store.Source{Provider: locMovement}, false, locDawarich},
		{store.Source{Provider: locMovement}, true, locMovement},
	}
	for i, c := range cases {
		if got := locationProvider(c.src, c.coreOK); got != c.want {
			t.Errorf("case %d: %s, want %s", i, got, c.want)
		}
	}
}

// With Pocket Core, the location source reads Pocket Movement's day view as
// the person; a Movement link switched off in Core falls back to Dawarich,
// and a person who picks Dawarich gets Dawarich.
func TestLocationFromPocketMovement(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	var mcalls []string
	mv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		mcalls = append(mcalls, r.Header.Get("Authorization")+" "+r.URL.Path)
		e.mu.Unlock()
		if r.URL.Path != "/api/v1/day/"+day0 && !strings.HasPrefix(r.URL.Path, "/api/v1/day/") {
			http.NotFound(rw, r)
			return
		}
		_, _ = rw.Write([]byte(`{"summary":{"distance":12345,"moving_s":1800,"modes":{"driving":12345}},"entries":[{"type":"journey","start_ts":1788249600,"end_ts":1788251400,"distance":12345,"mode":"driving","start":[44.7,17.2],"end":[44.9,16.7]},{"type":"visit","start_ts":1788251400,"end_ts":1788255000,"name":"Prijedor","status":"confirmed","lat":44.9,"lng":16.7}]}`))
	}))
	t.Cleanup(mv.Close)
	e.mu.Lock()
	e.targets[movementApp.slug] = mv
	e.mu.Unlock()

	loc := e.settings(e.ownerTok).Sources["dawarich"].Location
	if loc == nil || loc.Provider != locMovement || loc.MovementVia != modeCore || loc.Choice != "" || loc.DawarichVia != modeCore {
		t.Fatalf("location %+v", loc)
	}
	where := func() sources.Where {
		t.Helper()
		var w sources.Where
		f := e.facts(e.owner, day0)["dawarich"]
		if err := json.Unmarshal(f.Payload, &w); err != nil || f.Error != "" {
			t.Fatalf("fact %+v", f)
		}
		return w
	}
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if w := where(); w.Provider != "movement" || w.DistanceKm != 12.3 || w.Places != 1 || w.Entries[1].Name != "Prijedor" {
		t.Fatalf("movement where %+v", w)
	}
	if len(mcalls) != 1 || mcalls[0] != "Bearer pcx_movement_movement:read /api/v1/day/"+day0 {
		t.Fatalf("movement calls %v", mcalls)
	}
	_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "movement"})
	if !strings.Contains(body, "Pocket Movement reachable through Pocket Core") {
		t.Errorf("movement test %s", body)
	}

	// The link switched off in Core: Dawarich answers, the day keeps a location.
	e.mu.Lock()
	e.off[movementApp.slug+" "+movementApp.scope] = "off_admin"
	e.mu.Unlock()
	loc = e.settings(e.ownerTok).Sources["dawarich"].Location
	if loc.Provider != locMovement || loc.MovementVia != "" || !strings.Contains(loc.MovementDetail, "Day → Pocket Movement link is switched off in Pocket Core") {
		t.Fatalf("location off %+v", loc)
	}
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if w := where(); w.Provider != "dawarich" {
		t.Fatalf("fallback where %+v", w)
	}

	// The person picks Dawarich: no Movement calls even with the link on.
	e.mu.Lock()
	delete(e.off, movementApp.slug+" "+movementApp.scope)
	e.mu.Unlock()
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"dawarich": map[string]any{"provider": "dawarich"}}})
	loc = e.settings(e.ownerTok).Sources["dawarich"].Location
	if loc.Choice != locDawarich || loc.Provider != locDawarich {
		t.Fatalf("location dawarich %+v", loc)
	}
	n := len(mcalls)
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if w := where(); w.Provider != "dawarich" || len(mcalls) != n {
		t.Fatalf("dawarich where %+v, %d movement calls", w, len(mcalls)-n)
	}
}

// Without Pocket Core the location source is Dawarich, whatever was stored.
func TestLocationWithoutCore(t *testing.T) {
	e := newLinkEnv(t, false)
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"dawarich": map[string]any{"provider": "movement"}}})
	loc := e.settings(e.ownerTok).Sources["dawarich"].Location
	if loc == nil || loc.Choice != locMovement || loc.Provider != locDawarich || loc.MovementVia != "" {
		t.Fatalf("location %+v", loc)
	}
}
