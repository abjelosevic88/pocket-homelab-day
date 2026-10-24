package server

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// modeTargets is every target Day reports, in Core's names: the Pocket apps
// (the money source as "money") and the services.
var modeTargets = []string{"podcasts", "habits", "health", "bookshelf", "travel", "kitchen", "garage", "invoicing", "split", "inventory", "documents", "tasks", "projects", "money",
	"service:immich", "service:maloja", "service:dawarich", "service:jellystat", "service:pinepods", "service:audiobookshelf",
	"service:jellyseerr", "service:forgejo", "service:nextcloud"}

// expModes is "user target mode" for every target: def, except the given ones.
func expModes(user, def string, except map[string]string) []string {
	out := make([]string, 0, len(modeTargets))
	for _, t := range modeTargets {
		m := def
		if v, ok := except[t]; ok {
			m = v
		}
		out = append(out, user+" "+t+" "+m)
	}
	return out
}

func (w *linkWorld) reported() ([]string, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.modes))
	for _, m := range w.modes {
		out = append(out, m["user"]+" "+m["target"]+" "+m["mode"])
	}
	sort.Strings(out)
	return out, w.nModes
}

// waitModes waits for a report (after the `after`-th) carrying exactly exp.
func (e *linkEnv) waitModes(t *testing.T, what string, after int, exp ...string) int {
	t.Helper()
	sort.Strings(exp)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, n := e.reported()
		if n > after && strings.Join(got, "\n") == strings.Join(exp, "\n") {
			return n
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: reports %d (after %d)\n got  %q\n want %q", what, n, after, got, exp)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Day tells Core how each person reaches every Pocket app and service: the
// mode in effect (defaults included) for everybody in one call after
// registering, and again right after someone's modes change.
func TestModesReportedToCore(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// before start: the owner pasted an Immich key (direct by default); the
	// member switched Kitchen off before modes existed (off by default)
	ost, _ := e.st.GetSettings(ctx, e.owner.ID)
	im := ost.Sources["immich"]
	im.URL, im.Key = e.svcs["immich"].URL, "direct-immich"
	ost.Sources["immich"] = im
	if err := e.st.SaveSettings(ctx, e.owner.ID, ost); err != nil {
		t.Fatal(err)
	}
	ms, _ := e.st.GetSettings(ctx, e.member.ID)
	k := ms.Sources["kitchen"]
	k.Enabled = false
	ms.Sources["kitchen"] = k
	if err := e.st.SaveSettings(ctx, e.member.ID, ms); err != nil {
		t.Fatal(err)
	}

	go e.s.coreLoop(ctx)
	owner := map[string]string{"service:immich": "direct"}
	member := map[string]string{"kitchen": "off"}
	n := e.waitModes(t, "start", 0, append(expModes("owner@x", "core", owner), expModes("member@x", "core", member)...)...)

	// a mode picked, and a keyless Maloja URL pasted (direct by default)
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"habits": map[string]any{"mode": "off"}, "firefly": map[string]any{"mode": "direct"}}})
	e.do(e.memberTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"maloja": map[string]any{"url": "http://maloja.local"}}})
	owner["habits"], owner["money"] = "off", "direct"
	member["service:maloja"] = "direct"
	n = e.waitModes(t, "after change", n, append(expModes("owner@x", "core", owner), expModes("member@x", "core", member)...)...)

	// Disconnect (URL and key cleared) falls back to the default, Pocket Core
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"immich": map[string]any{"url": "", "key": "", "public_url": ""}}})
	delete(owner, "service:immich")
	n = e.waitModes(t, "after disconnect", n, append(expModes("owner@x", "core", owner), expModes("member@x", "core", member)...)...)

	// saving without a mode change does not report
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"date_format": "iso", "sources": map[string]any{"jellystat": map[string]any{"extra": "someone"}}})
	time.Sleep(200 * time.Millisecond)
	if _, m := e.reported(); m != n {
		t.Fatalf("reported without a mode change: %d → %d", n, m)
	}

	// what is reported is exactly what Settings shows
	got, _ := e.s.connModes(ctx)
	for _, who := range []struct{ email, tok string }{{"owner@x", e.ownerTok}, {"member@x", e.memberTok}} {
		v := e.settings(who.tok)
		for _, name := range connectableOrder() {
			ct, _ := connectable(name)
			for _, m := range got {
				if m.User == who.email && m.Target == ct.target && v.Sources[name].Mode != m.Mode {
					t.Errorf("%s %s: settings %q, reported %q", who.email, name, v.Sources[name].Mode, m.Mode)
				}
			}
		}
	}
}

// Without Pocket Core nothing is reported and nothing breaks.
func TestModesWithoutCore(t *testing.T) {
	e := newLinkEnv(t, false)
	e.s.reportModes(context.Background())
	e.s.reportModesSoon()
	if modes, _ := e.s.connModes(context.Background()); len(modes) != 2*len(modeTargets) || modes[0].Mode != "direct" {
		t.Fatalf("modes without Core: %v", modes)
	}
	if _, n := e.reported(); n != 0 {
		t.Fatalf("reported without Core: %d", n)
	}
}

// With the SDK's credential cache a pull and the thumbnails ask Core once a
// minute, Test connection always asks afresh, the status line still knows
// whose key it is, and a Core restart does not stop the pulls.
func TestServiceCredentialCache(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.s.core = core.New(e.coreURL, core.Options{DB: e.s.store.DB, Token: "pca_day"}) // default: one minute

	before := e.askedFor("immich")
	e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"immich"})
	e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"immich"})
	if e.s.immich(ctx, e.owner) == nil {
		t.Fatal("thumbnail client")
	}
	if got := e.askedFor("immich") - before; got != 1 {
		t.Fatalf("immich credential asked %d times for two pulls and a thumbnail, want 1", got)
	}
	if f := e.facts(e.owner, day0)["immich"]; f.Error != "" || f.Headline != 1 {
		t.Fatalf("immich fact %+v", f)
	}

	// Test connection skips the cache
	_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "immich"})
	if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "through Pocket Core (the box's key)") {
		t.Fatalf("test: %s", body)
	}
	if got := e.askedFor("immich") - before; got != 2 {
		t.Fatalf("Test connection used the cache: asked %d", got)
	}
	// whose key it is, from the last answer, without asking again
	if v := e.settings(e.ownerTok).Sources["immich"]; v.KeySource != "box" || v.Via != "core" {
		t.Fatalf("status line %+v", v)
	}
	if got := e.askedFor("immich") - before; got != 2 {
		t.Fatalf("settings asked Core again: %d", got)
	}

	// Core restarting: a fresh test says so, pulls ride on the last answer
	e.s.core = core.New(e.coreURL, core.Options{DB: e.s.store.DB, Token: "pca_day", ServiceCache: time.Millisecond})
	e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"immich"})
	time.Sleep(5 * time.Millisecond)
	e.mu.Lock()
	e.svcDown = true
	e.mu.Unlock()
	_, body = e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "immich"})
	if !strings.Contains(body, `"ok":false`) {
		t.Fatalf("test while Core is down: %s", body)
	}
	calls := e.svcCallCount("immich")
	errs, _ := e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"immich"})
	if len(errs) != 0 || e.svcCallCount("immich") == calls {
		t.Fatalf("pull while Core is down: %v (immich calls %d → %d)", errs, calls, e.svcCallCount("immich"))
	}
	if f := e.facts(e.owner, day0)["immich"]; f.Error != "" {
		t.Fatalf("immich fact while Core is down %+v", f)
	}
}
