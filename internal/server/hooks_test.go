package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/config"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func TestHookDates(t *testing.T) {
	native := `{"event":"transaction.created","data":{"transaction":{"date":"2026-10-17","description":"Konzum"}}}`
	firefly := `{"trigger":"STORE_TRANSACTION","content":{"id":"5","attributes":{"transactions":[{"date":"2026-10-16T00:00:00+02:00"},{"date":"2026-10-16T00:00:00+02:00"}]}}}`
	if d := hookDates([]byte(native)); len(d) != 1 || d[0] != "2026-10-17" {
		t.Errorf("native: %v", d)
	}
	if d := hookDates([]byte(firefly)); len(d) != 1 || d[0] != "2026-10-16" {
		t.Errorf("firefly: %v", d)
	}
	if d := hookDates([]byte(`{"event":"ping","data":{"message":"hi"}}`)); len(d) != 0 {
		t.Errorf("ping: %v", d)
	}
}

func hookTestServer(t *testing.T) (*Server, *store.User, *store.User) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	owner := &store.User{Email: "owner@x", Role: "admin"}
	member := &store.User{Email: "member@x", Role: "member"}
	_ = st.CreateUser(context.Background(), owner)
	_ = st.CreateUser(context.Background(), member)
	return New(&config.Config{MoneyWebhookSecret: "s3cret"}, st, log, nil), owner, member
}

func signHook(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestMoneyHookSignature(t *testing.T) {
	hookPause = time.Hour // only check the queue, do not pull
	s, owner, _ := hookTestServer(t)
	body := `{"event":"transaction.updated","data":{"transaction":{"date":"2026-10-01"}}}`
	for _, c := range []struct {
		sig  string
		want int
	}{{signHook("s3cret", body), 200}, {signHook("wrong", body), 401}, {"", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/money", strings.NewReader(body))
		req.Header.Set("X-Pocket-Signature", c.sig)
		rec := httptest.NewRecorder()
		s.handleMoneyHook(rec, req)
		if rec.Code != c.want {
			t.Errorf("signature %q: got %d, want %d", c.sig, rec.Code, c.want)
		}
	}
	// a payload naming nobody is the owner's
	if !s.hookDays[hookDay{owner.ID, "2026-10-01"}] || len(s.hookDays) != 1 {
		t.Errorf("queued %v", s.hookDays)
	}
}

// A webhook that names an account refreshes that account's facts only; an
// unknown account refreshes nobody's.
func TestMoneyHookAttribution(t *testing.T) {
	hookPause = time.Hour
	s, owner, member := hookTestServer(t)
	send := func(target, body string) {
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		req.Header.Set("X-Pocket-Signature", signHook("s3cret", body))
		rec := httptest.NewRecorder()
		s.handleMoneyHook(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", target, rec.Code)
		}
	}
	send("/api/v1/hooks/money", `{"user_email":"Member@x","data":{"transaction":{"date":"2026-10-02"}}}`)
	send("/api/v1/hooks/money?user=member@x", `{"data":{"transaction":{"date":"2026-10-03"}}}`)
	send("/api/v1/hooks/money", `{"user_email":"stranger@x","data":{"transaction":{"date":"2026-10-04"}}}`)
	want := map[hookDay]bool{{member.ID, "2026-10-02"}: true, {member.ID, "2026-10-03"}: true}
	if len(s.hookDays) != len(want) {
		t.Fatalf("queued %v, want %v", s.hookDays, want)
	}
	for k := range want {
		if !s.hookDays[k] {
			t.Errorf("missing %v in %v", k, s.hookDays)
		}
	}
	if s.hookDays[hookDay{owner.ID, "2026-10-02"}] || s.hookDays[hookDay{owner.ID, "2026-10-04"}] {
		t.Error("owner got a day that was not theirs")
	}
}
