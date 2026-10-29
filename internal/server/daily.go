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
	for _, n := range []string{"travel", "dawarich", "immich", "maloja", "jellystat", "bookshelf", "kitchen", "firefly", "habits", "health", "still", "garage", "horoscope", "weather"} {
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

// notifiedDay returns (and remembers) the last day a push of one kind went to a user.
func (s *Server) notifiedDay(ctx context.Context, userID int64, kind string) string {
	key := fmt.Sprintf("%d:%s", userID, kind)
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	if s.notified[key] == "" {
		if v, _, ok := s.store.CacheGet(ctx, "notify:"+key); ok {
			s.notified[key] = v
		}
	}
	return s.notified[key]
}

func (s *Server) markNotified(ctx context.Context, userID int64, kind, day string) {
	key := fmt.Sprintf("%d:%s", userID, kind)
	s.notifyMu.Lock()
	s.notified[key] = day
	s.notifyMu.Unlock()
	_ = s.store.CachePut(ctx, "notify:"+key, day)
}

// maybeNotify sends one person's evening prompt, morning memory and Sunday
// story once a day each, to that person's own notification route.
func (s *Server) maybeNotify(ctx context.Context, u *store.User) {
	st, _ := s.store.GetSettings(ctx, u.ID)
	if st.NotifyURL == "" {
		return
	}
	now := time.Now()
	day := now.Format("2006-01-02")
	if st.AIWeeklyStory && s.aiClient(ctx, u, st) != nil && now.Weekday() == time.Sunday && now.Hour() >= st.EveningHour {
		if s.notifiedDay(ctx, u.ID, "weekly") != day {
			s.markNotified(ctx, u.ID, "weekly", day)
			if _, err := s.sendWeeklyStory(ctx, u); err != nil {
				s.log.Warn("weekly story", "user", u.ID, "err", err)
			}
		}
	}
	for _, kind := range []string{"evening", "morning"} {
		enabled, hour := st.NotifyEvening, st.EveningHour
		if kind == "morning" {
			enabled, hour = st.NotifyMorning, st.MorningHour
		}
		if !enabled || now.Hour() < hour {
			continue
		}
		if s.notifiedDay(ctx, u.ID, kind) == day {
			continue
		}
		s.markNotified(ctx, u.ID, kind, day)
		var err error
		if kind == "evening" {
			_, err = s.sendEvening(ctx, u, false)
		} else {
			_, err = s.sendMorning(ctx, u, false)
		}
		if err != nil {
			s.log.Warn("daily notification", "user", u.ID, "kind", kind, "err", err)
		}
	}
}

// sendEvening asks the person "how was today?" with their day's numbers.
// Skipped (unless forced) when they already wrote something today.
func (s *Server) sendEvening(ctx context.Context, u *store.User, force bool) (int, error) {
	st, _ := s.store.GetSettings(ctx, u.ID)
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if !n.Configured() {
		return 0, fmt.Errorf("notifications are not configured")
	}
	today := store.Today()
	if !force {
		if _, err := s.store.GetEntry(ctx, u.ID, today); err == nil {
			return 0, nil
		}
	}
	facts, _ := s.store.FactsForDay(ctx, u.ID, today)
	body := summaryLine(facts, 4)
	if body == "" {
		body = "Nothing pulled in yet."
	}
	question := st.Prompt
	if st.AIQuestion && s.aiClient(ctx, u, st) != nil {
		if q, err := s.aiQuestion(ctx, u, today, true); err == nil && q != "" {
			question = q
		} else if err != nil {
			s.log.Warn("ai question for the evening push", "err", err)
		}
	}
	body += "\n\n" + question
	link := ""
	if s.cfg.BaseURL != "" {
		link = s.cfg.BaseURL + "/day/" + today
	}
	if err := n.Send(ctx, notify.Message{Title: "How was today?", Body: body, Priority: "info", Tags: []string{"notebook"}, Link: link}); err != nil {
		return 0, err
	}
	return 1, nil
}

// sendMorning is the person's yesterday in one line plus their own "on this
// day" from earlier years.
func (s *Server) sendMorning(ctx context.Context, u *store.User, force bool) (int, error) {
	st, _ := s.store.GetSettings(ctx, u.ID)
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if !n.Configured() {
		return 0, fmt.Errorf("notifications are not configured")
	}
	today := store.Today()
	yesterday := store.AddDays(today, -1)
	facts, _ := s.store.FactsForDay(ctx, u.ID, yesterday)
	var lines []string
	if sum := summaryLine(facts, 5); sum != "" {
		lines = append(lines, "Yesterday: "+sum)
	}
	mem, _ := s.store.Memories(ctx, u.ID, today)
	for _, m := range mem {
		years := yearsBetween(m.Date, today)
		line := fmt.Sprintf("%s ago", sources.Plural(years, "year", "years"))
		if m.Highlight != "" {
			line += ": " + m.Highlight
		} else if m.Text != "" {
			line += ": " + firstLine(m.Text, 120)
		}
		lines = append(lines, line)
		if len(lines) >= 4 {
			break
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
	st, _ := s.store.GetSettings(r.Context(), userFrom(r.Context()).ID)
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
	u := userFrom(r.Context())
	var sent int
	var err error
	switch in.Which {
	case "morning":
		sent, err = s.sendMorning(r.Context(), u, true)
	case "weekly":
		sent, err = s.sendWeeklyStory(r.Context(), u)
	default:
		sent, err = s.sendEvening(r.Context(), u, true)
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
	facts, _ := s.store.FactsForDay(ctx, u.ID, today)
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
	writeJSON(w, http.StatusOK, map[string]any{"today": today, "stats": stats, "streak": cur, "longest_streak": longest, "recent": recent, "memories": mem, "backfill": s.backfillStatus(u.ID)})
}

var _ = json.Marshal

// sendWeeklyStory writes this week's story for one person and pushes it to
// their own notification route.
func (s *Server) sendWeeklyStory(ctx context.Context, u *store.User) (int, error) {
	st, _ := s.store.GetSettings(ctx, u.ID)
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if !n.Configured() {
		return 0, fmt.Errorf("notifications are not configured")
	}
	week := currentWeek(store.Today())
	story, err := s.makeStory(ctx, u, "week", week)
	if err != nil {
		return 0, err
	}
	link := ""
	if s.cfg.BaseURL != "" {
		link = s.cfg.BaseURL + "/stories?kind=week&period=" + week
	}
	if err := n.Send(ctx, notify.Message{Title: story.Title, Body: clip(story.Text, 1500), Priority: "info", Tags: []string{"books"}, Link: link}); err != nil {
		return 0, err
	}
	return 1, nil
}
