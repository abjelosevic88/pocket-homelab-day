package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// hookPause collects a burst of changes (a bulk edit, the mirror) into one pull per day.
var hookPause = 3 * time.Second

// handleMoneyHook receives Pocket Money's webhooks. A transaction that was
// added, changed or deleted re-pulls the money facts for its day right away
// instead of waiting for the 30-minute refresh.
func (s *Server) handleMoneyHook(w http.ResponseWriter, r *http.Request) {
	secret := s.cfg.MoneyWebhookSecret
	if secret == "" {
		writeErr(w, http.StatusNotFound, "not enabled (set MONEY_WEBHOOK_SECRET)")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "unreadable body")
		return
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	want := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !hmac.Equal([]byte(r.Header.Get("X-Pocket-Signature")), []byte(want)) {
		writeErr(w, http.StatusUnauthorized, "bad signature")
		return
	}
	dates := hookDates(body)
	queued := 0
	for _, d := range dates {
		if s.queueHookDay(d) {
			queued++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "days": queued})
}

// hookDates reads the transaction dates from either payload format: Pocket
// Money's own ({"data":{"transaction":{"date"}}}) or Firefly III's
// ({"content":{"transactions":[{"date"}]}}). A ping has none.
func hookDates(body []byte) []string {
	var v struct {
		Data struct {
			Transaction struct {
				Date string `json:"date"`
			} `json:"transaction"`
		} `json:"data"`
		Content struct {
			Attributes struct {
				Transactions []struct {
					Date string `json:"date"`
				} `json:"transactions"`
			} `json:"attributes"`
			Transactions []struct {
				Date string `json:"date"`
			} `json:"transactions"`
		} `json:"content"`
	}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	raw := []string{v.Data.Transaction.Date}
	for _, t := range v.Content.Transactions {
		raw = append(raw, t.Date)
	}
	for _, t := range v.Content.Attributes.Transactions {
		raw = append(raw, t.Date)
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range raw {
		if len(d) >= 10 {
			d = d[:10]
		}
		if _, err := time.Parse("2006-01-02", d); err != nil || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// queueHookDay schedules one day; the first date of a burst starts the timer.
func (s *Server) queueHookDay(date string) bool {
	if date > store.Today() {
		return false // a planned payment: nothing to show yet
	}
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	if s.hookDays == nil {
		s.hookDays = map[string]bool{}
	}
	first := len(s.hookDays) == 0
	s.hookDays[date] = true
	if first {
		time.AfterFunc(hookPause, s.flushHookDays)
	}
	return true
}

func (s *Server) flushHookDays() {
	s.hookMu.Lock()
	days := make([]string, 0, len(s.hookDays))
	for d := range s.hookDays {
		days = append(days, d)
	}
	s.hookDays = map[string]bool{}
	s.hookMu.Unlock()
	ctx := context.Background()
	for _, d := range days {
		// a pull already running for the day may have read the old state: try again shortly
		for try := 0; try < 5; try++ {
			if _, ran := s.pullDayOnly(ctx, d, true, []string{"firefly"}); ran {
				break
			}
			time.Sleep(5 * time.Second)
		}
		s.log.Info("money webhook refreshed", "date", d)
	}
}
