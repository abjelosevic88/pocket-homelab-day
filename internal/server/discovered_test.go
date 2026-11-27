package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// wardrobeDay is what the fake Pocket Wardrobe says happened on day0: paths
// for the item and its picture, money in minor units, one thing without a time.
const wardrobeDay = `{"date":"` + day0 + `","summary":"2 outfits worn","items":[
	{"id":"9","title":"Rain jacket, boots","sub":"Walk","kind":"worn","note":"Soaked anyway"},
	{"id":"4","time":"07:40","end":"08:00","title":"Blue linen shirt, chinos","sub":"Work","kind":"worn","done":true,"amount":4500,"currency":"eur","url":"/outfits/4","image":"/img/4.jpg"},
	{"id":"5","title":"   "}],
	"metrics":[{"label":"Worn this month","value":"31"}]}`

func wardrobeApp() *dayApp {
	return &dayApp{name: "Pocket Wardrobe", feed: map[string]any{"one": "outfit", "many": "outfits", "what": "What you wore that day"}, body: wardrobeDay}
}

// dayView is the part of the day page the discovered sources are in.
type dayView struct {
	Sources    []string                    `json:"sources"`
	Facts      map[string]store.Fact       `json:"facts"`
	Links      map[string]string           `json:"links"`
	SourceMeta map[string]store.SourceMeta `json:"source_meta"`
}

func (e *linkEnv) day(tok, date string) dayView {
	e.t.Helper()
	code, body := e.do(tok, "GET", "/days/"+date, nil)
	var v dayView
	if err := json.Unmarshal([]byte(body), &v); err != nil || code != http.StatusOK {
		e.t.Fatalf("day: %d %v %s", code, err, body)
	}
	return v
}

type settingsNames struct {
	Names      []string                    `json:"source_names"`
	SourceMeta map[string]store.SourceMeta `json:"source_meta"`
}

func (e *linkEnv) names(tok string) settingsNames {
	e.t.Helper()
	_, body := e.do(tok, "GET", "/settings", nil)
	var v settingsNames
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		e.t.Fatalf("settings: %v %s", err, body)
	}
	return v
}

func count(list []string, want string) int {
	n := 0
	for _, x := range list {
		if x == want {
			n++
		}
	}
	return n
}

func (w *linkWorld) asked(prefix string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, t := range w.tokens {
		if strings.HasPrefix(t, prefix) {
			n++
		}
	}
	return n
}

// An app that offers its day becomes a source with nothing written for it in
// Day: found through Core, read through the one route, shown with what the
// app said about itself.
func TestDiscoveredSource(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.addDayApp("wardrobe", wardrobeApp())

	// Settings: listed after Day's own sources, through Core, with nothing to paste
	v := e.settings(e.ownerTok)
	if ps := v.Sources["wardrobe"]; !ps.Discovered || ps.Pocket || ps.Service || ps.App != "Pocket Wardrobe" || !ps.Enabled || !ps.Configured || ps.Mode != "core" || ps.Via != "core" || ps.URL != "" || ps.Key != "" {
		t.Fatalf("settings source: %+v", ps)
	}
	n := e.names(e.ownerTok)
	if len(n.Names) != len(store.SourceNames)+1 || n.Names[len(n.Names)-1] != "wardrobe" {
		t.Errorf("source_names: %v", n.Names)
	}
	want := store.SourceMeta{Label: "Wardrobe", App: "Pocket Wardrobe", Icon: "https://wardrobe.example/favicon.svg", One: "outfit", Many: "outfits", Unit: "count", What: "What you wore that day", URL: "https://wardrobe.example"}
	if n.SourceMeta["wardrobe"] != want {
		t.Errorf("settings meta:\n got %+v\nwant %+v", n.SourceMeta["wardrobe"], want)
	}

	errs, ran := e.s.pullDay(ctx, e.owner.ID, day0, false)
	if !ran || len(errs) != 0 {
		t.Fatalf("pull: ran=%v errs=%v", ran, errs)
	}
	if got := e.asked("owner@x wardrobe pocket:day"); got != 1 {
		t.Errorf("link tokens asked for wardrobe: %d (%v)", got, e.tokens)
	}
	if c := e.lastCall("wardrobe"); c != "Bearer pcx_wardrobe_pocket:day /api/v1/pocket/day/"+day0 {
		t.Errorf("wardrobe was called with %q", c)
	}
	if len(e.bad) != 0 {
		t.Errorf("calls outside a link's scope: %v", e.bad)
	}

	f, ok := e.facts(e.owner, day0)["wardrobe"]
	if !ok || f.Error != "" || f.Summary != "2 outfits worn" || f.Headline != 2 {
		t.Fatalf("fact: %+v", f)
	}
	var a sources.Activity
	if err := json.Unmarshal(f.Payload, &a); err != nil {
		t.Fatal(err)
	}
	if a.Count != 2 || a.Done != 1 || a.Amount != 45 || a.Cur != "EUR" || len(a.Items) != 2 || len(a.Top) != 1 || a.Top[0] != (sources.Count{Name: "worn", Count: 2}) {
		t.Fatalf("activity: %+v", a)
	}
	// timed first; the item's address and picture are absolute, at the app's public address
	if it := a.Items[0]; it.Time != "07:40" || it.End != "08:00" || it.Title != "Blue linen shirt, chinos" || it.Sub != "Work" || it.Group != "worn" || !it.Done || it.Ref != "4" ||
		it.Amount != 45 || it.Currency != "EUR" || it.URL != "https://wardrobe.example/outfits/4" || it.Image != "https://wardrobe.example/img/4.jpg" {
		t.Errorf("item: %+v", it)
	}
	if it := a.Items[1]; it.Time != "" || it.Title != "Rain jacket, boots" || it.Note != "Soaked anyway" || it.Ref != "9" || it.URL != "" {
		t.Errorf("untimed item: %+v", it)
	}
	if len(a.Metrics) != 1 || a.Metrics[0] != (sources.ActivityMetric{Label: "Worn this month", Value: "31"}) {
		t.Errorf("metrics: %+v", a.Metrics)
	}

	// the day page: the source after Day's own, its meta, and the app's address to open
	d := e.day(e.ownerTok, day0)
	if len(d.Sources) == 0 || d.Sources[len(d.Sources)-1] != "wardrobe" || count(d.Sources, "wardrobe") != 1 {
		t.Errorf("day sources: %v", d.Sources)
	}
	if d.SourceMeta["wardrobe"] != want || d.Links["wardrobe"] != "https://wardrobe.example" || d.Facts["wardrobe"].Summary != "2 outfits worn" {
		t.Errorf("day page: meta %+v link %q fact %+v", d.SourceMeta["wardrobe"], d.Links["wardrobe"], d.Facts["wardrobe"])
	}

	// the brief the AI gets has the source's line, with the app's own words for its things
	brief := e.s.dayBrief(ctx, e.owner, day0, false, true, 0)
	if !strings.Contains(brief, "Wardrobe: 2 outfits, 45.00 EUR: 07:40 Blue linen shirt, chinos (Work) ✓; Rain jacket, boots (Walk) — Soaked anyway; Worn this month 31") {
		t.Errorf("brief:\n%s", brief)
	}
	// …and the day's one line names it before the weather
	if sum := summaryLine(e.facts(e.owner, day0), 0, e.s.keptActivities(ctx, e.owner.ID).Names()); !strings.Contains(sum, "2 outfits worn") {
		t.Errorf("summary line: %s", sum)
	}

	// the journal list, the month and the year carry the meta too
	if err := e.st.UpsertEntry(ctx, &store.Entry{UserID: e.owner.ID, Date: day0, Text: "A day"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/entries", "/calendar?month=" + day0[:7], "/years/" + day0[:4]} {
		_, body := e.do(e.ownerTok, "GET", path, nil)
		var r struct {
			SourceMeta map[string]store.SourceMeta              `json:"source_meta"`
			Summaries  map[string]map[string]string             `json:"summaries"`
			Activity   map[string]map[string]any                `json:"activity"`
			Days       []struct{ Headlines map[string]float64 } `json:"days"`
		}
		if err := json.Unmarshal([]byte(body), &r); err != nil || r.SourceMeta["wardrobe"] != want {
			t.Errorf("%s: meta %+v (%v)", path, r.SourceMeta["wardrobe"], err)
		}
		switch {
		case path == "/entries" && r.Summaries[day0]["wardrobe"] != "2 outfits worn":
			t.Errorf("journal summaries: %v", r.Summaries)
		case strings.HasPrefix(path, "/years/") && r.Activity["wardrobe"]["count"] != float64(2):
			t.Errorf("year activity: %v", r.Activity["wardrobe"])
		case strings.HasPrefix(path, "/calendar") && (len(r.Days) == 0 || r.Days[0].Headlines["wardrobe"] != 2):
			t.Errorf("calendar headlines: %+v", r.Days)
		}
	}

	// "Test connection" says how the link stands; nothing is asked of the app
	before := e.callCount("wardrobe")
	if _, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "app:wardrobe"}); !strings.Contains(body, `"ok":true`) || !strings.Contains(body, "Pocket Wardrobe is connected through Pocket Core") {
		t.Errorf("test: %s", body)
	}
	if e.callCount("wardrobe") != before {
		t.Error("the test called the app")
	}
}

// An app that already has a hand-written source is read by that source only,
// whatever it offers; the apps that feed a source are left alone too.
func TestDiscoveredNotTwice(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	// (music stands for the six apps that feed a source; the fake Core would
	// take a fake weather or movement for the real thing)
	for _, slug := range []string{"habits", "money", "contacts", "music", "day"} {
		e.addDayApp(slug, &dayApp{name: "Pocket " + slug, body: wardrobeDay})
	}
	e.addDayApp("bad_name", &dayApp{name: "Pocket Bad", body: wardrobeDay}) // settings keys could not tell it apart
	e.addDayApp("wardrobe", wardrobeApp())
	n := e.names(e.ownerTok)
	if len(n.Names) != len(store.SourceNames)+1 || count(n.Names, "habits") != 1 || count(n.Names, "wardrobe") != 1 {
		t.Fatalf("source_names: %v", n.Names)
	}
	if len(n.SourceMeta) != 1 {
		t.Errorf("meta: %v", n.SourceMeta)
	}
	if ps := e.settings(e.ownerTok).Sources["habits"]; !ps.Pocket || ps.Discovered {
		t.Errorf("habits is no longer the hand-written source: %+v", ps)
	}
	if errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	facts := e.facts(e.owner, day0)
	for _, slug := range []string{"money", "contacts", "music", "day", "bad_name"} {
		if _, ok := facts[slug]; ok {
			t.Errorf("a second source for %s", slug)
		}
	}
	// the habits fact is the hand-written one (its own payload), and nobody asked habits for the generic day
	if f := facts["habits"]; !strings.Contains(string(f.Payload), `"journal":"good day"`) {
		t.Errorf("habits fact: %s", f.Payload)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for slug, calls := range e.calls {
		for _, c := range calls {
			if slug != "wardrobe" && strings.Contains(c, core.DayPath) {
				t.Errorf("%s was asked for its generic day: %s", slug, c)
			}
		}
	}
}

// A link that is off: the source is listed with the reason, skipped by the
// pull and leaves no error behind. The same when it goes off after the look.
func TestDiscoveredLinkOff(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.addDayApp("wardrobe", wardrobeApp())
	e.off["wardrobe pocket:day"] = "off_person"
	ps := e.settings(e.ownerTok).Sources["wardrobe"]
	if !ps.Discovered || ps.Configured || ps.Via != "" || ps.Mode != "core" || !ps.Enabled || !strings.Contains(ps.Detail, "you switched the Day → Pocket Wardrobe link off in Pocket Core") {
		t.Fatalf("settings source: %+v", ps)
	}
	if errs, ran := e.s.pullDay(ctx, e.owner.ID, day0, false); !ran || len(errs) != 0 {
		t.Fatalf("pull: ran=%v errs=%v", ran, errs)
	}
	if _, ok := e.facts(e.owner, day0)["wardrobe"]; ok || e.asked("owner@x wardrobe") != 0 || e.callCount("wardrobe") != 0 {
		t.Errorf("a source with its link off was pulled: tokens %v", e.tokens)
	}
	if d := e.day(e.ownerTok, day0); count(d.Sources, "wardrobe") != 0 {
		t.Errorf("day sources: %v", d.Sources)
	}
	if _, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "app:wardrobe"}); !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "you switched the Day → Pocket Wardrobe link off") {
		t.Errorf("test: %s", body)
	}

	// somebody else: Core says they have no access, so nothing is pulled for them either
	if ps := e.settings(e.memberTok).Sources["wardrobe"]; ps.Configured || !strings.Contains(ps.Detail, "you do not have access to Pocket Wardrobe") {
		t.Errorf("member: %+v", ps)
	}

	// on in the list Day has, off by the time of the fetch: skipped, not an error
	delete(e.off, "wardrobe pocket:day")
	e.s.discovery.forget("")
	e.tokOff["wardrobe pocket:day"] = true
	errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, true)
	if _, failed := errs["wardrobe"]; failed {
		t.Errorf("link gone since the look: %v", errs)
	}
	if f, ok := e.facts(e.owner, day0)["wardrobe"]; ok {
		t.Errorf("a fact was written: %+v", f)
	}
	if e.asked("owner@x wardrobe pocket:day") != 1 || e.callCount("wardrobe") != 0 {
		t.Errorf("tokens %v calls %d", e.tokens, e.callCount("wardrobe"))
	}
}

// An app that fails leaves an error on the day like any other source, with
// its label kept so the card can say whose it is.
func TestDiscoveredAppError(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	a := wardrobeApp()
	a.status = http.StatusInternalServerError
	e.addDayApp("wardrobe", a)
	errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false)
	if errs["wardrobe"] == nil {
		t.Fatalf("no error: %v", errs)
	}
	f := e.facts(e.owner, day0)["wardrobe"]
	if f.Error != "the app could not answer (HTTP 500)" || len(f.Payload) > 4 {
		t.Errorf("fact: %+v", f)
	}
	if m, _ := e.st.SourceMetas(ctx, e.owner.ID); m["wardrobe"].Label != "Wardrobe" {
		t.Errorf("meta not kept with the error: %+v", m)
	}
	// it answers again: the error goes
	a.status = 0
	if errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, true); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	if f := e.facts(e.owner, day0)["wardrobe"]; f.Error != "" || f.Summary != "2 outfits worn" {
		t.Errorf("fact: %+v", f)
	}
}

// A day with nothing is kept as nothing (like every other source with
// nothing that day): no line, no number, and it is not asked for again.
func TestDiscoveredEmptyDay(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.addDayApp("wardrobe", &dayApp{name: "Pocket Wardrobe", body: `{"date":"` + day0 + `","items":[]}`})
	if errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	f, ok := e.facts(e.owner, day0)["wardrobe"]
	if !ok || f.Summary != "" || f.Headline != 0 || !strings.Contains(string(f.Payload), `"count":0`) || !strings.Contains(string(f.Payload), `"items":[]`) {
		t.Errorf("fact: %+v %s", f, f.Payload)
	}
	if brief := e.s.dayBrief(ctx, e.owner, day0, false, true, 0); strings.Contains(brief, "Wardrobe") {
		t.Errorf("brief names a source with nothing: %s", brief)
	}
	// no label from the app: its name without the family word, and plain words for its things
	if m := e.day(e.ownerTok, day0).SourceMeta["wardrobe"]; m.Label != "Wardrobe" || m.One != "item" || m.Many != "items" || m.Unit != "count" {
		t.Errorf("default meta: %+v", m)
	}
	n := e.callCount("wardrobe")
	e.s.pullDay(ctx, e.owner.ID, day0, false) // day0 is long over: frozen
	if e.callCount("wardrobe") != n {
		t.Error("a frozen day with nothing was asked for again")
	}
}

// Each person can switch a discovered source off (and on again) for
// themselves; off means not fetched.
func TestDiscoveredSwitch(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.addDayApp("wardrobe", wardrobeApp())
	if errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	// a URL or key sent along is not kept: there is no Direct mode
	code, _ := e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"wardrobe": map[string]any{"enabled": false, "url": "http://x", "key": "secret", "mode": "direct"}, "nosuchapp": map[string]any{"enabled": false}}})
	if code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	ps := e.settings(e.ownerTok).Sources["wardrobe"]
	if ps.Enabled || ps.Configured || ps.Mode != "off" || ps.Via != "" || ps.Detail != "switched off" || ps.URL != "" || ps.Key != "" {
		t.Fatalf("switched off: %+v", ps)
	}
	st, _ := e.st.GetSettings(ctx, e.owner.ID)
	if on, ok := st.Discovered["wardrobe"]; !ok || on || len(st.Discovered) != 1 {
		t.Errorf("stored switches: %v", st.Discovered)
	}
	if _, own := st.Sources["wardrobe"]; own {
		t.Error("a discovered source ended up among Day's own")
	}
	// Core's map hears about the switch (off here, like any other source)
	modes := e.s.personModes(e.owner.Email, st)
	if last := modes[len(modes)-1]; last != (core.ConnMode{User: "owner@x", Target: "wardrobe", Mode: "off"}) {
		t.Errorf("reported mode: %+v", last)
	}
	// "Pull again" drops what it stored for the day and does not fetch it
	asked := e.asked("owner@x wardrobe")
	if code, body := e.do(e.ownerTok, "POST", "/days/"+day0+"/refresh", nil); code != http.StatusOK {
		t.Fatalf("refresh: %d %s", code, body)
	}
	if _, ok := e.facts(e.owner, day0)["wardrobe"]; ok || e.asked("owner@x wardrobe") != asked {
		t.Errorf("switched off but pulled (tokens %v)", e.tokens)
	}
	if d := e.day(e.ownerTok, day0); count(d.Sources, "wardrobe") != 0 {
		t.Errorf("day sources: %v", d.Sources)
	}
	// the other person's switch is their own
	if ps := e.settings(e.memberTok).Sources["wardrobe"]; !ps.Enabled {
		t.Errorf("member's switch followed the owner's: %+v", ps)
	}
	// on again
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"wardrobe": map[string]any{"enabled": true}}})
	if ps := e.settings(e.ownerTok).Sources["wardrobe"]; !ps.Enabled || !ps.Configured || ps.Via != "core" {
		t.Fatalf("switched on: %+v", ps)
	}
	if errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, true); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	if f := e.facts(e.owner, day0)["wardrobe"]; f.Summary != "2 outfits worn" {
		t.Errorf("fact after switching on: %+v", f)
	}
}

// Without Pocket Core, with a Core from before capabilities (rows without a
// target) and with a Core that lists nobody, Day is exactly as it was.
func TestDiscoveredNothingToDiscover(t *testing.T) {
	check := func(t *testing.T, e *linkEnv) {
		t.Helper()
		n := e.names(e.ownerTok)
		if strings.Join(n.Names, " ") != strings.Join(store.SourceNames, " ") || len(n.SourceMeta) != 0 {
			t.Errorf("source_names %v meta %v", n.Names, n.SourceMeta)
		}
		if got := len(e.settings(e.ownerTok).Sources); got != len(store.SourceNames) {
			t.Errorf("%d sources in settings, want %d", got, len(store.SourceNames))
		}
		if list := e.s.discovered(context.Background(), e.owner); len(list) != 0 {
			t.Errorf("discovered: %+v", list)
		}
		if d := e.day(e.ownerTok, day0); len(d.SourceMeta) != 0 || count(d.Sources, "wardrobe") != 0 {
			t.Errorf("day: %v %v", d.Sources, d.SourceMeta)
		}
	}
	t.Run("no core", func(t *testing.T) {
		e := newLinkEnv(t, false)
		e.addDayApp("wardrobe", wardrobeApp())
		check(t, e)
		if e.dayAsked != 0 {
			t.Error("asked a Core that is not there")
		}
	})
	t.Run("old core", func(t *testing.T) {
		e := newLinkEnv(t, true)
		a := wardrobeApp()
		a.old = true
		e.addDayApp("wardrobe", a)
		check(t, e)
		if errs, _ := e.s.pullDay(context.Background(), e.owner.ID, day0, false); len(errs) != 0 {
			t.Errorf("pull: %v", errs)
		}
		if _, ok := e.facts(e.owner, day0)["wardrobe"]; ok || e.callCount("wardrobe") != 0 {
			t.Error("pulled an app an old Core only has a row for")
		}
	})
	t.Run("nobody offers it", func(t *testing.T) {
		check(t, newLinkEnv(t, true))
	})
	t.Run("core is down", func(t *testing.T) {
		e := newLinkEnv(t, true)
		e.addDayApp("wardrobe", wardrobeApp())
		e.s.core = core.New("http://127.0.0.1:1", core.Options{DB: e.st.DB, Token: "pca_day"})
		if list := e.s.discovered(context.Background(), e.owner); len(list) != 0 {
			t.Errorf("discovered: %+v", list)
		}
	})
}

// What an app said about its source is kept with the facts: when Core lists
// it no more, the day still has its card, label and line.
func TestDiscoveredMetaSurvives(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.addDayApp("wardrobe", wardrobeApp())
	if errs, _ := e.s.pullDay(ctx, e.owner.ID, day0, false); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	e.mu.Lock()
	delete(e.dayApps, "wardrobe") // the app is removed
	e.mu.Unlock()
	e.s.discovery.forget("")

	if n := e.names(e.ownerTok); count(n.Names, "wardrobe") != 0 || n.SourceMeta["wardrobe"].Label != "Wardrobe" {
		t.Errorf("settings: names %v meta %+v", n.Names, n.SourceMeta)
	}
	d := e.day(e.ownerTok, day0)
	if m := d.SourceMeta["wardrobe"]; m.Label != "Wardrobe" || m.App != "Pocket Wardrobe" || m.One != "outfit" || m.Many != "outfits" || m.Icon == "" {
		t.Errorf("kept meta: %+v", m)
	}
	if d.Facts["wardrobe"].Summary != "2 outfits worn" || count(d.Sources, "wardrobe") != 0 || d.Links["wardrobe"] != "https://wardrobe.example" {
		t.Errorf("day: fact %+v sources %v link %q", d.Facts["wardrobe"], d.Sources, d.Links["wardrobe"])
	}
	if brief := e.s.dayBrief(ctx, e.owner, day0, false, true, 0); !strings.Contains(brief, "Wardrobe: 2 outfits") {
		t.Errorf("brief: %s", brief)
	}
	// "Pull again" keeps the day of a source Core no longer lists
	if code, _ := e.do(e.ownerTok, "POST", "/days/"+day0+"/refresh", nil); code != http.StatusOK {
		t.Fatal(code)
	}
	if _, ok := e.facts(e.owner, day0)["wardrobe"]; !ok {
		t.Error("the day of a removed app was dropped")
	}
	// the export carries the meta, and another account that brings it in reads the same
	_, exported := e.do(e.ownerTok, "GET", "/export.json", nil)
	if !strings.Contains(exported, `"source_meta":{"wardrobe"`) {
		t.Fatalf("export has no meta: %.300s", exported)
	}
	var doc any
	_ = json.Unmarshal([]byte(exported), &doc)
	if code, body := e.do(e.memberTok, "POST", "/import", doc); code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if m := e.day(e.memberTok, day0); m.SourceMeta["wardrobe"].Label != "Wardrobe" || m.Facts["wardrobe"].Summary != "2 outfits worn" {
		t.Errorf("imported day: %+v", m.SourceMeta)
	}
}

// Core is asked who offers a day once a minute per person, however often the
// list is needed; the cache does not grow without end.
func TestDiscoveryCache(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	e.addDayApp("wardrobe", wardrobeApp())
	for i := 0; i < 5; i++ {
		if list := e.s.discovered(ctx, e.owner); len(list) != 1 || list[0].name != "wardrobe" || !list[0].on {
			t.Fatalf("discovered: %+v", list)
		}
	}
	e.settings(e.ownerTok)
	e.day(e.ownerTok, day0)
	if e.dayAsked != 1 {
		t.Errorf("Core was asked %d times, want 1", e.dayAsked)
	}
	e.s.discovered(ctx, e.member) // each person has their own list
	if e.dayAsked != 2 {
		t.Errorf("Core was asked %d times, want 2", e.dayAsked)
	}
	e.s.discovery.forget(e.owner.Email)
	e.s.discovered(ctx, e.owner)
	if e.dayAsked != 3 {
		t.Errorf("after forgetting: asked %d times, want 3", e.dayAsked)
	}

	var c discoveryCache
	for i := 0; i < discoveryMax*3; i++ {
		c.put(fmt.Sprintf("p%d@x", i), nil, time.Minute)
	}
	if len(c.m) > discoveryMax {
		t.Errorf("cache holds %d people, more than %d", len(c.m), discoveryMax)
	}
	c.put("old@x", []discovered{{name: "wardrobe"}}, -time.Second)
	if _, ok := c.get("old@x"); ok {
		t.Error("an expired list was used")
	}
}

func TestDiscoveredMetaDefaults(t *testing.T) {
	m := discoveredMeta(core.CapableApp{App: "notes", Name: "Pocket Notes", Icon: "/favicon.svg", PublicURL: "https://notes.example/",
		Capabilities: core.Capabilities{Day: &core.DayFeed{Label: "Notes written", One: "note", Many: "notes", Unit: "min"}}})
	if m != (store.SourceMeta{Label: "Notes written", App: "Pocket Notes", One: "note", Many: "notes", Unit: "min", URL: "https://notes.example"}) {
		t.Errorf("meta: %+v", m)
	}
	// nothing said: the name without the family word, plain words, a count; an unknown unit counts too
	m = discoveredMeta(core.CapableApp{App: "print-farm", Name: "Pocket Print Farm", Capabilities: core.Capabilities{Day: &core.DayFeed{Unit: "furlongs"}}})
	if m.Label != "Print Farm" || m.One != "item" || m.Many != "items" || m.Unit != "count" {
		t.Errorf("defaults: %+v", m)
	}
	if m := discoveredMeta(core.CapableApp{App: "solo"}); m.App != "Solo" || m.Label != "Solo" {
		t.Errorf("no name: %+v", m)
	}
	for slug, want := range map[string]bool{"wardrobe": true, "print-farm": true, "habits": false, "money": false, "weather": false, "immich": false, "paperless": false, "bad_name": false, "": false, "Notes": false, "day": false, "a/b": false} {
		if discoverable(slug) != want {
			t.Errorf("discoverable(%q) = %v", slug, !want)
		}
	}
	if a := activityMeta(store.SourceMeta{Label: "Notes", App: "Pocket Notes", One: "note", Many: "notes", Unit: "count"}); a.Verb != "with notes" || a.Label != "Notes" {
		t.Errorf("activity meta: %+v", a)
	}
}
