package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/config"
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

func TestMoneyHookSignature(t *testing.T) {
	hookPause = time.Hour // only check the queue, do not pull
	s := &Server{cfg: &config.Config{MoneyWebhookSecret: "s3cret"}}
	body := `{"event":"transaction.updated","data":{"transaction":{"date":"2026-10-01"}}}`
	sign := func(secret string) string {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write([]byte(body))
		return "sha256=" + hex.EncodeToString(m.Sum(nil))
	}
	for _, c := range []struct {
		sig  string
		want int
	}{{sign("s3cret"), 200}, {sign("wrong"), 401}, {"", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/money", strings.NewReader(body))
		req.Header.Set("X-Pocket-Signature", c.sig)
		rec := httptest.NewRecorder()
		s.handleMoneyHook(rec, req)
		if rec.Code != c.want {
			t.Errorf("signature %q: got %d, want %d", c.sig, rec.Code, c.want)
		}
	}
	if !s.hookDays["2026-10-01"] || len(s.hookDays) != 1 {
		t.Errorf("queued %v", s.hookDays)
	}
}
