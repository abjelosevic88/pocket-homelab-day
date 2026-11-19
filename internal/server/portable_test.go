package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// moveEnv is a Day server behind a fake Pocket Core that introspects Pocket
// Move's link tokens: pcx_<exp|imp>_<name> acts for <name>@x with
// move:export or move:import.
func moveEnv(t *testing.T) *linkEnv {
	e := newLinkEnv(t, false)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/introspect" {
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.Split(in.Token, "_")
		if len(parts) != 3 {
			_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
			return
		}
		scope := map[string]string{"exp": core.ScopeMoveExport, "imp": core.ScopeMoveImport}[parts[1]]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active": true, "kind": "delegated", "scope": scope, "audience": "day", "iss": "https://core.test",
			"exp":  time.Now().Add(time.Hour).Unix(),
			"user": map[string]any{"sub": "sub-" + parts[2], "email": parts[2] + "@x", "name": parts[2]},
		})
	}))
	t.Cleanup(fake.Close)
	e.s.core = core.New(fake.URL, core.Options{DB: e.st.DB})
	return e
}

// Pocket Move's link tokens reach exactly the move routes and nothing else.
func TestPortableLinkScopes(t *testing.T) {
	e := moveEnv(t)
	exp, imp := "pcx_exp_owner", "pcx_imp_owner"
	code, body := e.do(exp, "GET", "/pocket/portable", nil)
	var desc core.Portable
	if code != 200 || json.Unmarshal([]byte(body), &desc) != nil || len(desc.Exports) != 2 || desc.Exports[0].Kind != core.KindPocket || len(desc.Imports) != 1 {
		t.Fatalf("portable with move:export: %d %s", code, body)
	}
	if code, _ := e.do(imp, "GET", "/pocket/portable", nil); code != 200 {
		t.Errorf("portable with move:import: %d", code)
	}
	for _, p := range []string{"/export.json", "/export.md"} {
		if code, _ := e.do(exp, "GET", p, nil); code != 200 {
			t.Errorf("GET %s with move:export: %d", p, code)
		}
	}
	for _, c := range []struct{ tok, method, path string }{
		{exp, "POST", "/import"},
		{exp, "GET", "/settings"},
		{exp, "GET", "/backup"},
		{exp, "GET", "/days/" + day0},
		{imp, "GET", "/export.json"},
		{imp, "GET", "/settings"},
		{imp, "PUT", "/days/" + day0},
	} {
		if code, _ := e.do(c.tok, c.method, c.path, map[string]any{}); code != http.StatusForbidden {
			t.Errorf("%s %s with %s: %d, want 403", c.method, c.path, c.tok, code)
		}
	}
	if code, body := e.do(imp, "POST", "/import", map[string]any{"entries": []any{}}); code != 200 {
		t.Errorf("import with move:import: %d %s", code, body)
	}
}

// The export goes into another person's journal through Move, only the
// exporting person's journal is in it, and the same file twice adds nothing.
func TestPortableImportOnceAndPerPerson(t *testing.T) {
	e := moveEnv(t)
	ctx := context.Background()
	_ = e.st.UpsertEntry(ctx, &store.Entry{UserID: e.owner.ID, Date: "2026-10-01", Highlight: "owner secret", Text: "owner text"})
	_ = e.st.CreateMoment(ctx, &store.Moment{UserID: e.owner.ID, Date: "2026-10-01", Time: "09:00", Text: "coffee", Kind: "note"})
	_ = e.st.CreatePin(ctx, &store.Pin{UserID: e.owner.ID, Date: "2026-10-01", Source: "photos", Ref: "a1", Title: "Lake"})
	_ = e.st.UpsertEntry(ctx, &store.Entry{UserID: e.member.ID, Date: "2026-10-02", Highlight: "member secret"})

	_, ownerExport := e.do("pcx_exp_owner", "GET", "/export.json", nil)
	if strings.Contains(ownerExport, "member secret") {
		t.Error("member's journal in the owner's export")
	}
	if _, md := e.do("pcx_exp_member", "GET", "/export.md", nil); strings.Contains(md, "owner secret") {
		t.Error("owner's journal in the member's Markdown export")
	}

	var first map[string]int
	code, body := e.do("pcx_imp_member", "POST", "/import", json.RawMessage(ownerExport))
	if code != 200 || json.Unmarshal([]byte(body), &first) != nil || first["entries"] != 1 || first["moments"] != 1 || first["pins"] != 1 || first["imported"] != 3 {
		t.Fatalf("first import: %d %s", code, body)
	}
	var second map[string]int
	_, body = e.do("pcx_imp_member", "POST", "/import", json.RawMessage(ownerExport))
	if json.Unmarshal([]byte(body), &second) != nil || second["imported"] != 0 || second["skipped"] != 3 {
		t.Errorf("second import: %s", body)
	}
	pins, _ := e.st.ListPins(ctx, e.member.ID, "0000-01-01", "9999-12-31")
	moments, _ := e.st.ListMoments(ctx, e.member.ID, "0000-01-01", "9999-12-31", 100)
	if len(pins) != 1 || len(moments) != 1 {
		t.Errorf("member has %d pins, %d moments", len(pins), len(moments))
	}
	if got, _ := e.st.GetEntry(ctx, e.member.ID, "2026-10-02"); got.Highlight != "member secret" {
		t.Errorf("member's own day changed: %+v", got)
	}
	if ms, _ := e.st.ListMoments(ctx, e.owner.ID, "0000-01-01", "9999-12-31", 100); len(ms) != 1 {
		t.Errorf("owner moments = %d", len(ms))
	}
}
