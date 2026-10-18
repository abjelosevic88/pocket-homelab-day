package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func timeNow() time.Time { return time.Now() }

// summaryLine joins the one-line summaries of a day's facts, most telling first.
func summaryLine(facts map[string]store.Fact, max int) string {
	var parts []string
	for _, n := range []string{"travel", "dawarich", "immich", "maloja", "jellystat", "bookshelf", "kitchen", "firefly", "habits", "garage", "weather"} {
		f, ok := facts[n]
		if !ok || f.Error != "" || f.Summary == "" {
			continue
		}
		parts = append(parts, f.Summary)
		if max > 0 && len(parts) >= max {
			break
		}
	}
	return strings.Join(parts, " · ")
}

// maybeNotify sends the evening prompt and the morning memory once a day each.
func (s *Server) maybeNotify(ctx context.Context) {
	st, _ := s.store.GetSettings(ctx)
	if st.NotifyURL == "" {
		return
	}
	now := time.Now()
	day := now.Format("2006-01-02")
	for _, kind := range []string{"evening", "morning"} {
		enabled, hour := st.NotifyEvening, st.EveningHour
		if kind == "morning" {
			enabled, hour = st.NotifyMorning, st.MorningHour
		}
		if !enabled || now.Hour() < hour {
			continue
		}
		if s.notified[kind] == "" {
			if v, _, ok := s.store.CacheGet(ctx, "notify:"+kind); ok {
				s.notified[kind] = v
			}
		}
		if s.notified[kind] == day {
			continue
		}
		s.notified[kind] = day
		_ = s.store.CachePut(ctx, "notify:"+kind, day)
		var err error
		if kind == "evening" {
			_, err = s.sendEvening(ctx, false)
		} else {
			_, err = s.sendMorning(ctx, false)
		}
		if err != nil {
			s.log.Warn("daily notification", "kind", kind, "err", err)
		}
	}
}

// sendEvening asks "how was today?" with the day's numbers. Skipped (unless
// forced) when every user already wrote something today.
func (s *Server) sendEvening(ctx context.Context, force bool) (int, error) {
	st, _ := s.store.GetSettings(ctx)
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if !n.Configured() {
		return 0, fmt.Errorf("notifications are not configured")
	}
	today := store.Today()
	if !force {
		users, _ := s.store.ListUsers(ctx)
		written := 0
		for _, u := range users {
			if _, err := s.store.GetEntry(ctx, u.ID, today); err == nil {
				written++
			}
		}
		if len(users) > 0 && written == len(users) {
			return 0, nil
		}
	}
	facts, _ := s.store.FactsForDay(ctx, today)
	body := summaryLine(facts, 4)
	if body == "" {
		body = "Nothing pulled in yet."
	}
	body += "\n\n" + st.Prompt
	link := ""
	if s.cfg.BaseURL != "" {
		link = s.cfg.BaseURL + "/day/" + today
	}
	if err := n.Send(ctx, notify.Message{Title: "How was today?", Body: body, Priority: "info", Tags: []string{"notebook"}, Link: link}); err != nil {
		return 0, err
	}
	return 1, nil
}

// sendMorning is yesterday in one line plus "on this day" from earlier years.
func (s *Server) sendMorning(ctx context.Context, force bool) (int, error) {
	st, _ := s.store.GetSettings(ctx)
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if !n.Configured() {
		return 0, fmt.Errorf("notifications are not configured")
	}
	today := store.Today()
	yesterday := store.AddDays(today, -1)
	facts, _ := s.store.FactsForDay(ctx, yesterday)
	var lines []string
	if sum := summaryLine(facts, 5); sum != "" {
		lines = append(lines, "Yesterday: "+sum)
	}
	users, _ := s.store.ListUsers(ctx)
	for _, u := range users {
		mem, _ := s.store.Memories(ctx, u.ID, today)
		for _, m := range mem {
			years := yearsBetween(m.Date, today)
			line := fmt.Sprintf("%s ago", sources.Plural(years, "year", "years"))
			if m.Highlight != "" {
				line += ": " + m.Highlight
			} else if m.Text != "" {
				line += ": " + firstLine(m.Text, 120)
			}
			if len(users) > 1 && u.Name != "" {
				line = u.Name + ", " + line
			}
			lines = append(lines, line)
			if len(lines) >= 4 {
				break
			}
		}
	}
	if len(lines) == 0 {
		if !force {
			return 0, nil
		}
		lines = append(lines, "No memories for today yet, and nothing pulled in for yesterday.")
	}
	link := ""
	if s.cfg.BaseURL != "" {
		link = s.cfg.BaseURL + "/day/" + yesterday
	}
	title := "Good morning"
	if len(lines) > 1 || strings.Contains(lines[0], " ago") {
		title = "On this day"
	}
	if err := n.Send(ctx, notify.Message{Title: title, Body: strings.Join(lines, "\n"), Priority: "info", Tags: []string{"sunrise"}, Link: link}); err != nil {
		return 0, err
	}
	return 1, nil
}

func yearsBetween(a, b string) int {
	ta, _ := store.ParseDate(a)
	tb, _ := store.ParseDate(b)
	y := tb.Year() - ta.Year()
	if y < 1 {
		y = 1
	}
	return y
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > n {
		return strings.TrimSpace(string(r[:n])) + "…"
	}
	return s
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	st, _ := s.store.GetSettings(r.Context())
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(r.Context(), notify.Message{Title: "Pocket Day", Body: "Notifications work. The evening prompt and the morning memories will arrive here.", Priority: "info", Tags: []string{"notebook"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleNotifyRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Which string `json:"which"`
	}
	_ = decode(r, &in)
	var sent int
	var err error
	if in.Which == "morning" {
		sent, err = s.sendMorning(r.Context(), true)
	} else {
		sent, err = s.sendEvening(r.Context(), true)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "sent": sent})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": sent})
}

// ---- dashboard (Homepage widget) & home ----

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	today := store.Today()
	stats := s.store.Stats(ctx, u.ID, today)
	cur, longest := s.store.Streak(ctx, u.ID, today)
	_, written := s.store.GetEntry(ctx, u.ID, today)
	facts, _ := s.store.FactsForDay(ctx, today)
	head := func(src string) float64 {
		if f, ok := facts[src]; ok && f.Error == "" {
			return f.Headline
		}
		return 0
	}
	mem, _ := s.store.Memories(ctx, u.ID, today)
	writeJSON(w, http.StatusOK, map[string]any{
		"today_written": written == nil, "today_text": map[bool]string{true: "written", false: "not yet"}[written == nil],
		"streak": cur, "longest_streak": longest, "entries_month": stats.ThisMonth, "entries_year": stats.ThisYear, "entries_total": stats.Total,
		"mood_month": round1(stats.MoodMonth), "memories": len(mem),
		"photos_today": int(head("immich")), "km_today": head("dawarich"), "listens_today": int(head("maloja")), "watch_min_today": int(head("jellystat")), "spend_today": head("firefly"),
		"habits_today_pct": head("habits"), "today_summary": summaryLine(facts, 3),
	})
}

// handleHome feeds the landing page: today, the streak, the latest entries, memories.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	today := store.Today()
	stats := s.store.Stats(ctx, u.ID, today)
	cur, longest := s.store.Streak(ctx, u.ID, today)
	recent, _, _ := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, Limit: 7})
	mem, _ := s.store.Memories(ctx, u.ID, today)
	writeJSON(w, http.StatusOK, map[string]any{"today": today, "stats": stats, "streak": cur, "longest_streak": longest, "recent": recent, "memories": mem, "backfill": s.backfillStatus()})
}

var _ = json.Marshal
