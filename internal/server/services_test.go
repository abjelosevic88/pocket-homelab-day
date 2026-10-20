package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-day/internal/config"
)

// How each service is called, by the key Core hands out (box-<type> for the
// owner, an admin): the header or parameter each one's API wants.
var wantServiceAuth = map[string]string{
	"immich":         "auth=box-immich",      // x-api-key
	"maloja":         "auth=",                // keyless read API
	"dawarich":       "api_key=box-dawarich", // query parameter
	"jellystat":      "auth=box-jellystat",   // x-api-token
	"pinepods":       "auth=box-pinepods",    // Api-Key
	"audiobookshelf": "auth=Bearer box-audiobookshelf",
	"jellyseerr":     "auth=box-jellyseerr", // X-Api-Key
	"forgejo":        "auth=token box-forgejo",
	"nextcloud":      "auth=basic owner-nc:box-nextcloud", // USERNAME + APP_PASSWORD
	"paperless":      "auth=Token box-paperless",
}

func (e *linkEnv) askedFor(typ string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, a := range e.svcAsked {
		if strings.HasSuffix(a, " "+typ) {
			n++
		}
	}
	return n
}

func TestManifestDeclaresServices(t *testing.T) {
	e := newLinkEnv(t, true)
	if err := e.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Services []struct{ Type, Reason string } `json:"services"`
	}
	if err := json.Unmarshal(e.reg, &m); err != nil || len(m.Services) == 0 {
		var wrap struct {
			Manifest struct {
				Services []struct{ Type, Reason string } `json:"services"`
			} `json:"manifest"`
		}
		_ = json.Unmarshal(e.reg, &wrap)
		m.Services = wrap.Manifest.Services
	}
	got := []string{}
	for _, sv := range m.Services {
		if sv.Reason == "" {
			t.Errorf("service %s has no reason", sv.Type)
		}
		got = append(got, sv.Type)
	}
	sort.Strings(got)
	want := "audiobookshelf dawarich forgejo immich jellyseerr jellystat maloja nextcloud paperless pinepods"
	if strings.Join(got, " ") != want {
		t.Fatalf("services\n got %s\nwant %s", strings.Join(got, " "), want)
	}
}

// With Pocket Core and nothing saved, every service comes through Core: its
// address and key asked on every pull and test, used the way each API wants.
func TestServicesThroughCore(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	v := e.settings(e.ownerTok)
	for _, n := range serviceSourceOrder {
		ps := v.Sources[n]
		if !ps.Service || ps.Pocket || ps.Mode != "core" || ps.Via != "core" || !ps.Configured || ps.KeySource != "box" {
			t.Errorf("%s: %+v", n, ps)
		}
	}
	if w := v.Sources["weather"]; w.Service || w.Mode != "" {
		t.Errorf("weather is not a service: %+v", w)
	}

	for _, n := range serviceSourceOrder {
		_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": n})
		if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "through Pocket Core (the box's key)") {
			t.Errorf("%s test: %s", n, body)
		}
		last := e.svcLastCall(n)
		if !strings.Contains(last, wantServiceAuth[n]) {
			t.Errorf("%s called as %q, want %q", n, last, wantServiceAuth[n])
		}
		if n == "maloja" && !strings.HasSuffix(last, "auth=") {
			t.Errorf("maloja got a key: %q", last)
		}
	}
	if last := e.svcLastCall("audiobookshelf"); !strings.HasPrefix(last, "GET /api/sessions?") {
		t.Errorf("box key reads every session: %q", last)
	}
	if last := e.svcLastCall("nextcloud"); !strings.Contains(last, "/remote.php/dav/calendars/owner-nc/") {
		t.Errorf("nextcloud user from Core: %q", last)
	}

	before := e.askedFor("immich")
	errs, ran := e.s.pullDay(ctx, e.owner.ID, day0, false)
	if !ran || len(errs) != 0 {
		t.Fatalf("pull: %v %v", ran, errs)
	}
	facts := e.facts(e.owner, day0)
	for _, n := range serviceSourceOrder {
		if f, ok := facts[n]; !ok || f.Error != "" {
			t.Errorf("fact %s: %+v", n, f)
		}
	}
	if f := facts["immich"]; f.Headline != 1 {
		t.Errorf("immich fact %+v", f)
	}
	if e.askedFor("immich") != before+1 {
		t.Errorf("immich credential asked %d times in a pull", e.askedFor("immich")-before)
	}
	// not kept: the next pull asks again
	e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"immich"})
	if e.askedFor("immich") != before+2 {
		t.Errorf("immich credential cached between pulls")
	}
	// thumbnails go through the same connection
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/v1/photos/a1/thumb", nil)
	req.Header.Set("Authorization", "Bearer "+e.ownerTok)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("thumb: %v %v", err, res)
	}
	res.Body.Close()
	if last := e.svcLastCall("immich"); !strings.HasPrefix(last, "GET /api/assets/a1/thumbnail") || !strings.Contains(last, "auth=box-immich") {
		t.Errorf("thumb read %q", last)
	}
	// the day links to the public address Core gave
	_, body := e.do(e.ownerTok, "GET", "/days/"+day0, nil)
	if !strings.Contains(body, `"immich":"https://immich.example"`) {
		t.Errorf("day links %s", clipStr(body))
	}
}

// Saved keys mean direct (the owner's setup keeps working), keyless Maloja
// counts a saved URL, Off makes no calls, a link off in Core is skipped.
func TestServiceModes(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	var malojaDirect atomic.Int32
	mal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		malojaDirect.Add(1)
		_, _ = w.Write([]byte(`{"status":"ok","amount":1,"list":[],"pagination":{"next_page":null}}`))
	}))
	defer mal.Close()
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{
		"immich": map[string]any{"url": e.svcs["immich"].URL, "key": "direct-immich"},
		"maloja": map[string]any{"url": mal.URL},
	}})
	v := e.settings(e.ownerTok)
	if im := v.Sources["immich"]; im.Mode != "direct" || im.Via != "direct" || im.KeySource != "" {
		t.Fatalf("immich with a saved key: %+v", im)
	}
	if ml := v.Sources["maloja"]; ml.Mode != "direct" || ml.Via != "direct" {
		t.Fatalf("maloja with a saved URL: %+v", ml)
	}
	// off: nothing called, nothing asked
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"dawarich": map[string]any{"mode": "off"}}})
	if d := e.settings(e.ownerTok).Sources["dawarich"]; d.Mode != "off" || d.Via != "" || d.Configured || d.Enabled {
		t.Fatalf("dawarich off: %+v", d)
	}
	// switched off by the person in Core; refused between status and credential
	e.mu.Lock()
	e.svcOff["owner@x jellystat"] = "off_person"
	e.svcCredOff["owner@x pinepods"] = true
	e.mu.Unlock()
	if j := e.settings(e.ownerTok).Sources["jellystat"]; j.Mode != "core" || j.Via != "" || !strings.Contains(j.Detail, "switched the Day → Jellystat link off") {
		t.Fatalf("jellystat off in Core: %+v", j)
	}
	asked0 := map[string]int{}
	for _, n := range serviceSourceOrder {
		asked0[n] = e.askedFor(n)
	}
	calls0 := map[string]int{}
	for _, n := range serviceSourceOrder {
		calls0[n] = e.svcCallCount(n)
	}

	errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false)
	if len(errs) != 0 {
		t.Fatalf("pull failed: %v", errs)
	}
	facts := e.facts(e.owner, day0)
	for _, n := range []string{"dawarich", "jellystat", "pinepods"} {
		if f, ok := facts[n]; ok {
			t.Errorf("%s pulled or recorded as failed: %+v", n, f)
		}
		if e.svcCallCount(n) != calls0[n] {
			t.Errorf("%s called while off", n)
		}
	}
	for _, n := range []string{"immich", "maloja", "forgejo"} {
		if f, ok := facts[n]; !ok || f.Error != "" {
			t.Errorf("%s: %+v", n, f)
		}
	}
	if last := e.svcLastCall("immich"); !strings.Contains(last, "auth=direct-immich") {
		t.Errorf("immich direct read %q", last)
	}
	if malojaDirect.Load() == 0 || e.svcCallCount("maloja") != calls0["maloja"] {
		t.Errorf("maloja direct: %d on the saved URL, %d on Core's", malojaDirect.Load(), e.svcCallCount("maloja")-calls0["maloja"])
	}
	for _, n := range []string{"immich", "maloja", "dawarich", "jellystat"} {
		if e.askedFor(n) != asked0[n] {
			t.Errorf("asked Core for %s", n)
		}
	}

	// Choosing Pocket Core ignores the saved key (kept for switching back);
	// a typed key is tested directly.
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"immich": map[string]any{"mode": "core"}}})
	e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"immich"})
	if last := e.svcLastCall("immich"); !strings.Contains(last, "auth=box-immich") {
		t.Errorf("immich in core mode read %q", last)
	}
	if st, _ := e.st.GetSettings(ctx, e.owner.ID); st.Sources["immich"].Key != "direct-immich" {
		t.Error("switching to Pocket Core dropped the saved key")
	}
	_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "immich", "url": e.svcs["immich"].URL, "key": "typed-key"})
	if !strings.Contains(body, "Immich 1.2.3, direct connection") || !strings.Contains(e.svcLastCall("immich"), "auth=typed-key") {
		t.Errorf("typed key test %s / %s", body, e.svcLastCall("immich"))
	}
	// Disconnect: nothing saved and no mode → Pocket Core
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"forgejo": map[string]any{"url": "http://f", "key": "fk"}}})
	if f := e.settings(e.ownerTok).Sources["forgejo"]; f.Mode != "direct" {
		t.Fatalf("forgejo direct: %+v", f)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"forgejo": map[string]any{"url": "", "key": "", "public_url": ""}}})
	if f := e.settings(e.ownerTok).Sources["forgejo"]; f.Mode != "core" || f.Via != "core" {
		t.Errorf("forgejo after disconnect: %+v", f)
	}
	// direct says what is missing
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"paperless": map[string]any{"mode": "direct"}}})
	if p := e.settings(e.ownerTok).Sources["paperless"]; p.Via != "" || p.Detail != "no URL yet" {
		t.Errorf("paperless direct with nothing: %+v", p)
	}
}

// A member without an account of their own in a service is not connected and
// the source is skipped; with their own key in Core they get exactly that.
func TestServiceMemberOwnKeyOrNothing(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	v := e.settings(e.memberTok)
	im := v.Sources["immich"]
	if im.Mode != "core" || im.Via != "" || im.Configured || im.Detail != "you have no account of your own in Immich yet; ask the admin or let a connector create one" {
		t.Fatalf("member immich: %+v", im)
	}
	calls := 0
	for _, n := range serviceSourceOrder {
		calls += e.svcCallCount(n)
	}
	errs, _ := e.s.pullDay(ctx, e.member.ID, day0, false)
	if len(errs) != 0 || len(e.facts(e.member, day0)) != 0 {
		t.Fatalf("member pull: %v %v", errs, e.facts(e.member, day0))
	}
	after := 0
	for _, n := range serviceSourceOrder {
		after += e.svcCallCount(n)
	}
	if after != calls {
		t.Errorf("services called for a member with no account")
	}
	_, body := e.do(e.memberTok, "POST", "/integrations/test", map[string]any{"which": "immich"})
	if !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "no account of your own in Immich") {
		t.Errorf("member immich test %s", body)
	}

	// their own key in Core
	e.mu.Lock()
	e.svcPersonal["member@x immich"] = true
	e.svcPersonal["member@x audiobookshelf"] = true
	e.svcPersonal["member@x maloja"] = true
	e.mu.Unlock()
	v = e.settings(e.memberTok)
	if im := v.Sources["immich"]; im.Via != "core" || im.KeySource != "personal" {
		t.Fatalf("member immich with own key: %+v", im)
	}
	_, body = e.do(e.memberTok, "POST", "/integrations/test", map[string]any{"which": "immich"})
	if !strings.Contains(body, "Immich 1.2.3 through Pocket Core (your own key)") || !strings.Contains(e.svcLastCall("immich"), "auth=own-immich-member") {
		t.Errorf("member immich test %s / %s", body, e.svcLastCall("immich"))
	}
	errs, _ = e.s.pullDay(ctx, e.member.ID, day0, false)
	if len(errs) != 0 {
		t.Fatalf("member pull: %v", errs)
	}
	f := e.facts(e.member, day0)
	if len(f) != 3 || f["immich"].Headline != 1 || f["audiobookshelf"].Headline != 10 {
		t.Errorf("member facts: %+v", f)
	}
	if last := e.svcLastCall("audiobookshelf"); !strings.HasPrefix(last, "GET /api/me/listening-sessions?") || !strings.Contains(last, "own-audiobookshelf-member") {
		t.Errorf("abs with the member's own key read %q", last)
	}
	if last := e.svcLastCall("maloja"); !strings.HasSuffix(last, "auth=") {
		t.Errorf("maloja got a key: %q", last)
	}
}

// Without Pocket Core a service is direct, as before; Core mode says why not.
func TestServicesWithoutCore(t *testing.T) {
	e := newLinkEnv(t, false)
	v := e.settings(e.ownerTok)
	if im := v.Sources["immich"]; im.Mode != "direct" || im.Via != "" || im.Detail != "no URL yet" {
		t.Errorf("immich without core: %+v", im)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"immich": map[string]any{"mode": "core"}}})
	if im := e.settings(e.ownerTok).Sources["immich"]; im.Via != "" || !strings.Contains(im.Detail, "no Pocket Core on this box") {
		t.Errorf("core mode without core: %+v", im)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"nextcloud": map[string]any{"url": "http://nc", "key": "pw"}}})
	if nc := e.settings(e.ownerTok).Sources["nextcloud"]; nc.Mode != "direct" || nc.Detail != "no user name yet" {
		t.Errorf("nextcloud without a user: %+v", nc)
	}
	if errs, _ := e.s.pullDay(context.Background(), e.owner.ID, day0, false); len(errs) != 0 {
		t.Errorf("errs %v", errs)
	}
}

// Env values for a service are copied once, like the Pocket ones.
func TestServiceEnvSeededOnce(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.cfg.Sources["immich"] = config.SourceEnv{URL: e.svcs["immich"].URL, Key: "env-immich"}
	if err := e.s.seedOwner(ctx); err != nil {
		t.Fatal(err)
	}
	if im := e.settings(e.ownerTok).Sources["immich"]; im.Mode != "direct" || im.Via != "direct" {
		t.Fatalf("seeded key should mean direct: %+v", im)
	}
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"immich": map[string]any{"url": "", "key": ""}}})
	if err := e.s.seedOwner(ctx); err != nil {
		t.Fatal(err)
	}
	if im := e.settings(e.ownerTok).Sources["immich"]; im.Mode != "core" || im.URL != "" || im.Key != "" {
		t.Errorf("disconnect undone by a restart: %+v", im)
	}
}
