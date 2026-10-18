package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// A day with no words written in Pocket Day (a mood or tags alone do not
// count) takes its words from the Pocket
// Habits journal: the "good thing" line becomes the highlight, the important
// note and the "bad thing" line become the text. Such an entry has origin
// "habits" and keeps following Habits until it is edited here (then it is the
// user's). Removing one here is remembered, so it does not come back.

// habitsOwner is the account the Habits journal belongs to: the first admin.
func (s *Server) habitsOwner(ctx context.Context) *store.User {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil
	}
	for i := range users {
		if users[i].Role == "admin" {
			return &users[i]
		}
	}
	if len(users) > 0 {
		return &users[0]
	}
	return nil
}

func habitsSkipKey(userID int64, date string) string {
	return fmt.Sprintf("habits-skip:%d:%s", userID, date)
}

// habitsEntryText turns a Habits check-in into highlight + text.
func habitsEntryText(c sources.Checkin) (string, string) {
	highlight := strings.TrimSpace(c.Journal)
	var paras []string
	if imp := strings.TrimSpace(c.Important); imp != "" {
		if highlight == "" {
			highlight, imp = imp, ""
		}
		if imp != "" {
			paras = append(paras, imp)
		}
	}
	if bad := strings.TrimSpace(c.Bad); bad != "" {
		paras = append(paras, "👎 "+bad)
	}
	return highlight, strings.Join(paras, "\n\n")
}

// syncHabitsEntry applies the rule for one date. It never touches an entry
// the user wrote.
func (s *Server) syncHabitsEntry(ctx context.Context, date string) {
	u := s.habitsOwner(ctx)
	if u == nil {
		return
	}
	if v, _, ok := s.store.CacheGet(ctx, habitsSkipKey(u.ID, date)); ok && v == "1" {
		return
	}
	cur, err := s.store.GetEntry(ctx, u.ID, date)
	if err == nil && cur.Origin != "habits" && (strings.TrimSpace(cur.Highlight) != "" || strings.TrimSpace(cur.Text) != "") {
		return // the user's own words win; a mood or tags alone are not words
	}
	facts, _ := s.store.FactsForDay(ctx, date)
	f, ok := facts["habits"]
	if !ok || len(f.Payload) <= 4 {
		return
	}
	var c sources.Checkin
	if json.Unmarshal(f.Payload, &c) != nil {
		return
	}
	hl, text := habitsEntryText(c)
	if cur == nil {
		if hl == "" && text == "" {
			return
		}
		cur = &store.Entry{UserID: u.ID, Date: date}
	}
	if cur.Highlight == hl && cur.Text == text {
		return
	}
	if cur.Origin != "habits" && hl == "" && text == "" {
		return
	}
	cur.Highlight, cur.Text, cur.Origin = hl, text, "habits" // mood, tags and the star stay; an entry left with nothing is deleted
	if err := s.store.UpsertEntry(ctx, cur); err != nil {
		s.log.Warn("habits journal → entry", "date", date, "err", err)
	}
}

// syncAllHabitsEntries runs the rule over every day Habits has a fact for.
func (s *Server) syncAllHabitsEntries(ctx context.Context) {
	fs, err := s.store.FactsRange(ctx, "0000-01-01", "9999-12-31", "habits")
	if err != nil {
		return
	}
	for _, f := range fs {
		if ctx.Err() != nil {
			return
		}
		s.syncHabitsEntry(ctx, f.Date)
	}
}
