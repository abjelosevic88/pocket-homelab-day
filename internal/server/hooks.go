package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// hookPause collects a burst of changes (a bulk edit, the mirror) into one pull per day.
var hookPause = 3 * time.Second

// hookDay is one person's day to re-pull.
type hookDay struct {
	user int64
	date string
}

// handleMoneyHook receives Pocket Money's webhooks. A transaction that was
// added, changed or deleted re-pulls the money facts for its day right away
// instead of waiting for the 30-minute refresh.
//
// The facts refreshed are one person's: the account named by the signed body
// ("user_email", or "data.user.email") or by ?user=<email> on the webhook URL.
// A webhook that names nobody (today's Pocket Money and Firefly payloads do
// not) is the owner's, because the webhook was set up with the owner's
// Pocket Money account. Only that person's own Firefly/Pocket Money source is
// called, so nobody else's money facts are touched.
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
	if len(dates) > 0 {
		uid := s.hookUser(r.Context(), body, r.URL.Query().Get("user"))
		if uid == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "days": 0})
			return
		}
		for _, d := range dates {
			if s.queueHookDay(uid, d) {
				queued++
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "days": queued})
}

// hookUser attributes a webhook to a local account (see handleMoneyHook).
func (s *Server) hookUser(ctx context.Context, body []byte, queryUser string) int64 {
	var v struct {
		UserEmail string `json:"user_email"`
		Data      struct {
			User struct {
				Email string `json:"email"`
			} `json:"user"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &v)
	for _, email := range []string{v.UserEmail, v.Data.User.Email, queryUser} {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		if u, err := s.store.GetUserByEmail(ctx, email); err == nil {
			return u.ID
		}
		s.log.Warn("money webhook: named account not found, ignored", "email", email)
		return 0
	}
	owner := s.store.OwnerID(ctx)
	s.log.Debug("money webhook names no account: attributed to the owner", "user", owner)
	return owner
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

// queueHookDay schedules one user's day; the first date of a burst starts the timer.
func (s *Server) queueHookDay(userID int64, date string) bool {
	if date > store.Today() {
		return false // a planned payment: nothing to show yet
	}
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	if s.hookDays == nil {
		s.hookDays = map[hookDay]bool{}
	}
	first := len(s.hookDays) == 0
	s.hookDays[hookDay{userID, date}] = true
	if first {
		time.AfterFunc(hookPause, s.flushHookDays)
	}
	return true
}

func (s *Server) flushHookDays() {
	s.hookMu.Lock()
	days := make([]hookDay, 0, len(s.hookDays))
	for d := range s.hookDays {
		days = append(days, d)
	}
	s.hookDays = map[hookDay]bool{}
	s.hookMu.Unlock()
	ctx := context.Background()
	for _, d := range days {
		// a pull already running for the day may have read the old state: try again shortly
		for try := 0; try < 5; try++ {
			if _, ran := s.pullDayOnly(ctx, d.user, d.date, true, []string{"firefly"}); ran {
				break
			}
			time.Sleep(5 * time.Second)
		}
		s.log.Info("money webhook refreshed", "user", d.user, "date", d.date)
	}
}
