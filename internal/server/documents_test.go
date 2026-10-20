package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
)

// Pocket Documents replaced Paperless-ngx: a Pocket app source ("documents")
// read through a Core app link with documents:read (or a pd_ token typed in
// Settings), per person; Paperless is gone from the sources and the services.
func TestDocumentsSourceReplacesPaperless(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	if ps, ok := pocketSources["documents"]; !ok || ps.slug != "documents" || ps.scope != "documents:read" || ps.reason == "" {
		t.Fatalf("documents source: %+v", ps)
	}
	if _, ok := serviceSources["paperless"]; ok {
		t.Fatal("paperless still a service source")
	}
	v := e.settings(e.ownerTok)
	if _, ok := v.Sources["paperless"]; ok {
		t.Error("paperless still in Settings → Sources")
	}
	d := v.Sources["documents"]
	if !d.Pocket || d.Service || d.App != "Pocket Documents" || d.Mode != "core" || d.Via != "core" {
		t.Fatalf("documents in settings: %+v", d)
	}

	// Pocket Core: a fresh link token, the list of the day, nothing outside the scope.
	if errs, _ := e.s.pullDayOnly(ctx, e.owner.ID, day0, false, []string{"documents"}); len(errs) != 0 {
		t.Fatalf("pull: %v", errs)
	}
	if last := e.lastCall("documents"); last != "Bearer pcx_documents_documents:read /api/v1/documents" {
		t.Errorf("documents read %q", last)
	}
	f := e.facts(e.owner, day0)["documents"]
	var a sources.Activity
	if err := json.Unmarshal(f.Payload, &a); err != nil || a.Count != 1 || a.Items[0].Title != "Water bill" || a.Items[0].Sub != "ViK · Invoice" || a.Items[0].URL != "/documents/12" {
		t.Errorf("documents fact %+v (%s)", f, f.Payload)
	}
	if f.Summary != "1 document · Water bill" || f.Headline != 1 {
		t.Errorf("summary %q headline %v", f.Summary, f.Headline)
	}
	e.mu.Lock()
	bad := append([]string(nil), e.bad...)
	e.mu.Unlock()
	if len(bad) > 0 {
		t.Errorf("calls outside documents:read: %v", bad)
	}
	_, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "documents"})
	if !strings.Contains(body, "Pocket Documents 9.9 through Pocket Core") {
		t.Errorf("documents test %s", body)
	}
	// the day page links to Documents' public URL (the item links are relative to it)
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"documents": map[string]any{"public_url": "https://docs.example/"}}})
	_, body = e.do(e.ownerTok, "GET", "/days/"+day0, nil)
	if !strings.Contains(body, `"documents":"https://docs.example"`) || !strings.Contains(body, `"url":"/documents/12"`) {
		t.Errorf("day links %s", clipStr(body))
	}

	// Direct: the person's own pd_ token typed into Settings, nothing asked of Core.
	e.do(e.ownerTok, "PUT", "/settings", map[string]any{"sources": map[string]any{"documents": map[string]any{"mode": "direct", "url": e.targets["documents"].URL, "key": "direct-documents"}}})
	if d := e.settings(e.ownerTok).Sources["documents"]; d.Mode != "direct" || d.Via != "direct" {
		t.Fatalf("documents direct: %+v", d)
	}
	e.mu.Lock()
	asked := len(e.tokens)
	e.mu.Unlock()
	e.s.pullDayOnly(ctx, e.owner.ID, day0, true, []string{"documents"})
	if last := e.lastCall("documents"); last != "Bearer direct-documents /api/v1/documents" {
		t.Errorf("documents direct read %q", last)
	}
	e.mu.Lock()
	if len(e.tokens) != asked {
		t.Errorf("asked Core for a token in direct mode: %v", e.tokens[asked:])
	}
	e.mu.Unlock()

	// The member has no Documents link: nothing pulled, nothing failed, nobody else's documents.
	if d := e.settings(e.memberTok).Sources["documents"]; d.Via != "" || !strings.Contains(d.Detail, "do not have access to Pocket Documents") {
		t.Errorf("member documents: %+v", d)
	}
	if errs, _ := e.s.pullDay(ctx, e.member.ID, day0, false); len(errs) != 0 {
		t.Errorf("member pull errs %v", errs)
	}
	if _, ok := e.facts(e.member, day0)["documents"]; ok {
		t.Error("member got documents facts")
	}
	_, body = e.do(e.memberTok, "GET", "/days/"+day0, nil)
	if strings.Contains(body, "Water bill") {
		t.Error("member sees the owner's documents")
	}
}
