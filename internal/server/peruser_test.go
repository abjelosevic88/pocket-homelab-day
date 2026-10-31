package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// perUserEnv is a running app with an owner (admin, with sources, AI and a
// journal) and a member who just signed in. Each can authenticate with a
// local API token or a token Pocket Core issued to a paired phone.
type perUserEnv struct {
	t             *testing.T
	ts            *httptest.Server
	st            *store.Store
	owner, member *store.User
	ownerTok      string
	memberTok     string
	ownerTokID    int64
	momentID      int64
	pinID         int64
	askID         int64
}

const secret = "owner secret line"

func newPerUserEnv(t *testing.T) *perUserEnv {
	t.Helper()
	ctx := context.Background()
	// a fake Pocket Core that introspects paired-phone tokens
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		email := map[string]string{"pcd_owner": "owner@x", "pcd_member": "member@x", "pcd_new": "new@x"}[in.Token]
		if email == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "device", "user": map[string]any{"id": 9, "sub": email, "email": email, "name": email}})
	}))
	t.Cleanup(core.Close)

	dir := t.TempDir()
	cfg := testConfig(dir)
	cfg.CoreURL = core.URL
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(cfg.DBPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(cfg, st, log, nil) // no Bootstrap: no janitor, no registration loop
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)

	e := &perUserEnv{t: t, ts: ts, st: st}
	e.owner = &store.User{Email: "owner@x", Name: "Owner", Role: "admin"}
	e.member = &store.User{Email: "member@x", Name: "Member", Role: "member"}
	if err := st.CreateUser(ctx, e.owner); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUser(ctx, e.member); err != nil {
		t.Fatal(err)
	}
	var tok *store.APIToken
	e.ownerTok, tok, _ = st.CreateAPIToken(ctx, e.owner.ID, "owner")
	e.ownerTokID = tok.ID
	e.memberTok, _, _ = st.CreateAPIToken(ctx, e.member.ID, "member")

	// the owner's journal, facts, AI history and settings
	_ = st.UpsertEntry(ctx, &store.Entry{UserID: e.owner.ID, Date: "2026-10-01", Highlight: secret, Text: "owner text", Mood: 5, Tags: "ownertag", Pinned: true})
	_ = st.UpsertEntry(ctx, &store.Entry{UserID: e.owner.ID, Date: "2025-10-01", Highlight: "owner memory"})
	m := &store.Moment{UserID: e.owner.ID, Date: "2026-10-01", Time: "09:00", Text: "owner moment"}
	_ = st.CreateMoment(ctx, m)
	e.momentID = m.ID
	p := &store.Pin{UserID: e.owner.ID, Date: "2026-10-01", Source: "immich", Ref: "a1", Title: "owner pin"}
	_ = st.CreatePin(ctx, p)
	e.pinID = p.ID
	for _, src := range []string{"immich", "dawarich", "habits", "firefly"} {
		_ = st.PutFact(ctx, e.owner.ID, &store.Fact{Date: "2026-10-01", Source: src, Payload: json.RawMessage(`{"count":7,"note":"owner fact"}`), Headline: 7, Summary: "owner fact " + src})
	}
	_ = st.PutStory(ctx, &store.Story{UserID: e.owner.ID, Kind: "week", Period: "2026-W40", Title: "owner story", Text: secret})
	a := &store.Ask{UserID: e.owner.ID, Question: "owner question", Answer: secret}
	_ = st.CreateAsk(ctx, a)
	e.askID = a.ID
	st.LogUsage(ctx, e.owner.ID, "draft", "m", 10, 5)
	os, _ := st.GetSettings(ctx, e.owner.ID)
	os.AIKey, os.NotifyURL, os.HomeLat, os.HomeLon = "sk-owner-key-123456", "http://notify.invalid/owner", 44.7, 17.2
	im := os.Sources["immich"]
	im.URL, im.Key = "http://immich.invalid", "owner-immich-key"
	os.Sources["immich"] = im
	if err := st.SaveSettings(ctx, e.owner.ID, os); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *perUserEnv) do(tok, method, path, body string) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.ts.URL+"/api/v1"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// mustNotLeak fails when a response carries anything of the owner's.
func (e *perUserEnv) mustNotLeak(who, path, body string) {
	e.t.Helper()
	for _, s := range []string{secret, "owner text", "owner moment", "owner pin", "owner fact", "owner story", "owner question", "owner memory", "ownertag", "owner-immich-key", "sk-owner", "immich.invalid", "notify.invalid"} {
		if strings.Contains(body, s) {
			e.t.Errorf("%s GET %s leaked %q: %s", who, path, s, clipStr(body))
		}
	}
}

func clipStr(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// TestMemberSeesNothingOfTheOwner covers list, read, search, calendar,
// stories, asks, facts, settings and export, through a local API token and a
// Core paired-phone token.
func TestMemberSeesNothingOfTheOwner(t *testing.T) {
	e := newPerUserEnv(t)
	reads := []string{
		"/days/2026-10-01", "/today", "/entries", "/entries?q=owner", "/search?q=owner", "/tags", "/calendar?month=2026-10",
		"/memories?date=2026-10-01", "/moments?from=2026-01-01&to=2026-12-31", "/years", "/years/2026", "/dashboard", "/home",
		"/ai/stories", "/ai/asks", "/ai/usage", "/patterns?from=2026-01-01&to=2026-12-31", "/settings", "/facts/status",
		"/export.json", "/export.md", "/settings/system",
	}
	for _, tok := range []string{e.memberTok, "pcd_member"} {
		for _, path := range reads {
			code, body := e.do(tok, http.MethodGet, path, "")
			if code != http.StatusOK {
				t.Errorf("member GET %s = %d %s", path, code, clipStr(body))
				continue
			}
			e.mustNotLeak("member", path, body)
		}
		// the day page: no entry, no facts, no configured sources of the owner's
		_, body := e.do(tok, http.MethodGet, "/days/2026-10-01", "")
		var day dayPage
		_ = json.Unmarshal([]byte(body), &day)
		if day.Entry != nil || len(day.Moments) != 0 || len(day.Pins) != 0 || len(day.Facts) != 0 || len(day.Sources) != 0 || len(day.Links) != 0 || len(day.Memories) != 0 {
			t.Errorf("member day page shows owner data: %s", clipStr(body))
		}
		// the settings: none of the owner's sources, AI or notifications
		_, body = e.do(tok, http.MethodGet, "/settings", "")
		var st struct {
			AIConfigured     bool `json:"ai_configured"`
			NotifyConfigured bool `json:"notify_configured"`
			Sources          map[string]struct {
				Configured bool `json:"configured"`
			} `json:"sources"`
		}
		_ = json.Unmarshal([]byte(body), &st)
		if st.AIConfigured || st.NotifyConfigured || st.Sources["immich"].Configured || st.Sources["weather"].Configured {
			t.Errorf("member inherited owner settings: %s", clipStr(body))
		}
		// stories are keyed by period, not id: someone else's reads exactly like none
		if code, body := e.do(tok, http.MethodGet, "/ai/stories/week/2026-W40", ""); code != 200 || strings.TrimSpace(body) != `{"story":null}` {
			t.Errorf("member story read = %d %s", code, body)
		}
		// another person's records answer 404, never 403
		for _, c := range []struct{ method, path, body string }{
			{http.MethodDelete, "/ai/stories/week/2026-W40", ""},
			{http.MethodDelete, fmt.Sprintf("/ai/asks/%d", e.askID), ""},
			{http.MethodPut, fmt.Sprintf("/moments/%d", e.momentID), `{"text":"hijacked"}`},
			{http.MethodDelete, fmt.Sprintf("/moments/%d", e.momentID), ""},
			{http.MethodDelete, fmt.Sprintf("/pins/%d", e.pinID), ""},
			{http.MethodDelete, fmt.Sprintf("/tokens/%d", e.ownerTokID), ""},
			{http.MethodGet, "/days/2026-10-01/facts/immich", ""},
			{http.MethodGet, "/photos/a1/thumb", ""},
			{http.MethodGet, "/backup", ""},
		} {
			if code, body := e.do(tok, c.method, c.path, c.body); code != http.StatusNotFound {
				t.Errorf("member %s %s = %d, want 404: %s", c.method, c.path, code, clipStr(body))
			}
		}
	}

	// the owner's things are all still there and unchanged
	ctx := context.Background()
	if m, err := e.st.GetMoment(ctx, e.owner.ID, e.momentID); err != nil || m.Text != "owner moment" {
		t.Errorf("owner moment changed: %+v %v", m, err)
	}
	if _, err := e.st.GetStory(ctx, e.owner.ID, "week", "2026-W40"); err != nil {
		t.Errorf("owner story gone: %v", err)
	}
	if asks, _ := e.st.ListAsks(ctx, e.owner.ID, 0); len(asks) != 1 {
		t.Errorf("owner asks = %d", len(asks))
	}
	if pins, _ := e.st.ListPins(ctx, e.owner.ID, "2026-10-01", "2026-10-01"); len(pins) != 1 {
		t.Errorf("owner pins = %d", len(pins))
	}
	if toks, _ := e.st.ListAPITokens(ctx, e.owner.ID); len(toks) != 1 {
		t.Errorf("owner tokens = %d", len(toks))
	}

	// and the owner, through both token kinds, still sees their own journal
	for _, tok := range []string{e.ownerTok, "pcd_owner"} {
		_, body := e.do(tok, http.MethodGet, "/days/2026-10-01", "")
		var day dayPage
		_ = json.Unmarshal([]byte(body), &day)
		if day.Entry == nil || day.Entry.Highlight != secret || len(day.Facts) != 4 || len(day.Moments) != 1 || len(day.Pins) != 1 {
			t.Errorf("owner day page: %s", clipStr(body))
		}
		if _, body := e.do(tok, http.MethodGet, "/search?q=owner", ""); !strings.Contains(body, secret) {
			t.Errorf("owner search lost their entry: %s", clipStr(body))
		}
		if _, body := e.do(tok, http.MethodGet, "/export.json", ""); !strings.Contains(body, "owner fact") {
			t.Errorf("owner export lost their facts")
		}
	}
}

// TestMemberWritesStayTheirs: writes land in the member's own journal, ids in
// the body are ignored, settings and imports never touch the owner's.
func TestMemberWritesStayTheirs(t *testing.T) {
	e := newPerUserEnv(t)
	ctx := context.Background()

	// writing the same day, even naming the owner's id, makes the member's own entry
	if code, body := e.do(e.memberTok, http.MethodPut, "/days/2026-10-01", fmt.Sprintf(`{"user_id":%d,"highlight":"member line"}`, e.owner.ID)); code != 200 {
		t.Fatalf("member put entry = %d %s", code, body)
	}
	if code, body := e.do("pcd_member", http.MethodPost, "/moments", fmt.Sprintf(`{"user_id":%d,"date":"2026-10-01","text":"member moment"}`, e.owner.ID)); code != 201 {
		t.Fatalf("member moment = %d %s", code, body)
	}
	if got, _ := e.st.GetEntry(ctx, e.owner.ID, "2026-10-01"); got.Highlight != secret {
		t.Errorf("owner entry overwritten: %+v", got)
	}
	if got, err := e.st.GetEntry(ctx, e.member.ID, "2026-10-01"); err != nil || got.Highlight != "member line" {
		t.Errorf("member entry: %+v %v", got, err)
	}
	if ms, _ := e.st.ListMoments(ctx, e.owner.ID, "2026-10-01", "2026-10-01", 0); len(ms) != 1 {
		t.Errorf("owner moments = %d, want 1", len(ms))
	}

	// a member configures their own source; the owner's stays as it was
	if code, body := e.do(e.memberTok, http.MethodPut, "/settings", `{"sources":{"immich":{"url":"http://member-immich","key":"member-key"}},"ai_key":"sk-member"}`); code != 200 {
		t.Fatalf("member settings = %d %s", code, body)
	}
	os, _ := e.st.GetSettings(ctx, e.owner.ID)
	ms, _ := e.st.GetSettings(ctx, e.member.ID)
	if os.Sources["immich"].Key != "owner-immich-key" || os.AIKey != "sk-owner-key-123456" {
		t.Errorf("owner settings changed: %+v", os.Sources["immich"])
	}
	if ms.Sources["immich"].Key != "member-key" || ms.AIKey != "sk-member" || ms.NotifyURL != "" {
		t.Errorf("member settings: %+v ai=%q notify=%q", ms.Sources["immich"], ms.AIKey, ms.NotifyURL)
	}
	// the app title is the box's: a member cannot change it, the admin can
	if code, _ := e.do(e.memberTok, http.MethodPut, "/settings", `{"title":"Mine"}`); code != http.StatusForbidden {
		t.Errorf("member renamed the app: %d", code)
	}
	if code, _ := e.do(e.ownerTok, http.MethodPut, "/settings", `{"title":"Our Days"}`); code != 200 {
		t.Errorf("admin rename: %d", code)
	}
	if st, _ := e.st.GetSettings(ctx, e.member.ID); st.Title != "Our Days" {
		t.Errorf("title is box-wide, member sees %q", st.Title)
	}

	// importing the owner's export puts everything into the member's own journal
	_, export := e.do(e.ownerTok, http.MethodGet, "/export.json", "")
	if code, body := e.do(e.memberTok, http.MethodPost, "/import", export); code != 200 {
		t.Fatalf("import = %d %s", code, body)
	}
	if got, _ := e.st.GetEntry(ctx, e.owner.ID, "2026-10-01"); got.Highlight != secret || got.Text != "owner text" {
		t.Errorf("import touched the owner's entry: %+v", got)
	}
	if fs, _ := e.st.FactsForDay(ctx, e.owner.ID, "2026-10-01"); len(fs) != 4 {
		t.Errorf("owner facts = %d", len(fs))
	}

	// deleting the member's entry leaves the owner's alone
	if code, _ := e.do(e.memberTok, http.MethodDelete, "/days/2026-10-01", ""); code != http.StatusNoContent {
		t.Errorf("member delete entry = %d", code)
	}
	if _, err := e.st.GetEntry(ctx, e.owner.ID, "2026-10-01"); err != nil {
		t.Errorf("owner entry gone: %v", err)
	}
	// deleting all of the member's questions leaves the owner's
	if code, _ := e.do(e.memberTok, http.MethodDelete, "/ai/asks/0", ""); code != http.StatusNoContent {
		t.Errorf("member clear asks = %d", code)
	}
	if asks, _ := e.st.ListAsks(ctx, e.owner.ID, 0); len(asks) != 1 {
		t.Errorf("owner asks = %d", len(asks))
	}
}

// TestPullsUseOnlyTheirOwnSources: a member without sources pulls nothing,
// and facts written for one person never show for another.
func TestPullsUseOnlyTheirOwnSources(t *testing.T) {
	e := newPerUserEnv(t)
	ctx := context.Background()
	if code, body := e.do(e.memberTok, http.MethodPost, "/days/2026-10-02/refresh", ""); code != 200 || strings.Contains(body, "owner") {
		t.Fatalf("member refresh = %d %s", code, clipStr(body))
	}
	if fs, _ := e.st.FactsForDay(ctx, e.member.ID, "2026-10-02"); len(fs) != 0 {
		t.Errorf("member got facts without sources: %v", fs)
	}
	if fs, _ := e.st.FactsForDay(ctx, e.member.ID, "2026-10-01"); len(fs) != 0 {
		t.Errorf("member sees owner facts: %v", fs)
	}
	if d, _ := e.st.FactDates(ctx, e.member.ID); len(d) != 0 {
		t.Errorf("member fact dates: %v", d)
	}
}

// TestAutoCreatedAccountsAreMembers: an account created on first contact
// (here a Core paired-phone token for an unknown email; OIDC and proxy sign-in
// go through the same path) is a member with nothing of anyone else's. Only
// the very first account on an empty box becomes the admin (the owner).
func TestAutoCreatedAccountsAreMembers(t *testing.T) {
	e := newPerUserEnv(t)
	code, body := e.do("pcd_new", http.MethodGet, "/auth/me", "")
	if code != 200 {
		t.Fatalf("me = %d %s", code, body)
	}
	u, err := e.st.GetUserByEmail(context.Background(), "new@x")
	if err != nil || u.Role != "member" {
		t.Fatalf("auto-created account: %+v %v", u, err)
	}
	if n, _ := e.st.CountAdmins(context.Background()); n != 1 {
		t.Errorf("admins = %d, want only the owner", n)
	}
	_, body = e.do("pcd_new", http.MethodGet, "/days/2026-10-01", "")
	e.mustNotLeak("new account", "/days/2026-10-01", body)
	if code, _ := e.do("pcd_new", http.MethodPost, "/users", `{"email":"x@x","password":"12345678"}`); code != http.StatusForbidden {
		t.Errorf("auto-created member could create users: %d", code)
	}
}

// TestPullAgainDropsSwitchedOffSources: what a source stored for a day before it
// was switched off goes when that day is pulled again; other days keep theirs.
func TestPullAgainDropsSwitchedOffSources(t *testing.T) {
	e := newPerUserEnv(t)
	ctx := context.Background()
	for _, d := range []string{"2026-10-02", "2026-10-03"} {
		if err := e.st.PutFact(ctx, e.member.ID, &store.Fact{Date: d, Source: "nextcloud", Payload: json.RawMessage(`{"items":[]}`), Headline: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if code, body := e.do(e.memberTok, http.MethodPost, "/days/2026-10-02/refresh", ""); code != 200 {
		t.Fatalf("refresh = %d %s", code, clipStr(body))
	}
	if fs, _ := e.st.FactsForDay(ctx, e.member.ID, "2026-10-02"); len(fs) != 0 {
		t.Errorf("switched-off source still on the pulled day: %v", fs)
	}
	if fs, _ := e.st.FactsForDay(ctx, e.member.ID, "2026-10-03"); len(fs) != 1 {
		t.Errorf("another day lost its facts: %v", fs)
	}
}
