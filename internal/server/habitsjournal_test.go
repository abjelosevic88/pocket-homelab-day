package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-day/internal/config"
	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func TestHabitsJournalFillsEmptyDays(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(&config.Config{}, st, log, nil)
	u := &store.User{Email: "a@b", Role: "admin"}
	_ = st.CreateUser(ctx, u)
	putHabits := func(date string, c sources.Checkin) {
		b, _ := json.Marshal(c)
		if err := st.PutFact(ctx, u.ID, &store.Fact{Date: date, Source: "habits", Payload: b}); err != nil {
			t.Fatal(err)
		}
	}

	// 1. nothing written → the Habits lines become the entry
	putHabits("2026-10-14", sources.Checkin{Journal: "Filip i Marija", Important: "Proba, ostao duze", Bad: "malo sna"})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-14")
	e, err := st.GetEntry(ctx, u.ID, "2026-10-14")
	if err != nil || e.Origin != "habits" || e.Highlight != "Filip i Marija" || e.Text != "Proba, ostao duze\n\n👎 malo sna" {
		t.Fatalf("entry = %+v, %v", e, err)
	}

	// 2. Habits changes → the habits entry follows
	putHabits("2026-10-14", sources.Checkin{Journal: "Filip, Marija i proba"})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-14")
	e, _ = st.GetEntry(ctx, u.ID, "2026-10-14")
	if e.Highlight != "Filip, Marija i proba" || e.Text != "" {
		t.Fatalf("not updated: %+v", e)
	}

	// 3. the user edits it → it is theirs, Habits no longer overwrites
	e.Text, e.Origin = "my own words", "user"
	_ = st.UpsertEntry(ctx, e)
	putHabits("2026-10-14", sources.Checkin{Journal: "something else"})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-14")
	e, _ = st.GetEntry(ctx, u.ID, "2026-10-14")
	if e.Origin != "user" || e.Highlight != "Filip, Marija i proba" || e.Text != "my own words" {
		t.Fatalf("user entry overwritten: %+v", e)
	}

	// 4. a day the user wrote first is never touched
	_ = st.UpsertEntry(ctx, &store.Entry{UserID: u.ID, Date: "2026-10-15", Highlight: "mine"})
	putHabits("2026-10-15", sources.Checkin{Journal: "from habits"})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-15")
	if e, _ := st.GetEntry(ctx, u.ID, "2026-10-15"); e.Highlight != "mine" {
		t.Fatalf("got %+v", e)
	}

	// 5. a removed habits entry stays removed
	putHabits("2026-10-16", sources.Checkin{Journal: "Filip moj"})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-16")
	_ = st.UpsertEntry(ctx, &store.Entry{UserID: u.ID, Date: "2026-10-16"})
	_ = st.CachePut(ctx, habitsSkipKey(u.ID, "2026-10-16"), "1")
	s.syncHabitsEntry(ctx, u.ID, "2026-10-16")
	if _, err := st.GetEntry(ctx, u.ID, "2026-10-16"); err != store.ErrNotFound {
		t.Fatalf("came back: %v", err)
	}

	// 6. a mood without words still gets the Habits words, and keeps the mood
	_ = st.UpsertEntry(ctx, &store.Entry{UserID: u.ID, Date: "2026-10-18", Mood: 4})
	putHabits("2026-10-18", sources.Checkin{Journal: "Kafa sa Valdom"})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-18")
	if e, _ := st.GetEntry(ctx, u.ID, "2026-10-18"); e.Highlight != "Kafa sa Valdom" || e.Mood != 4 || e.Origin != "habits" {
		t.Fatalf("mood-only day: %+v", e)
	}

	// 7. an empty check-in creates nothing
	putHabits("2026-10-17", sources.Checkin{Done: 2, Total: 5})
	s.syncHabitsEntry(ctx, u.ID, "2026-10-17")
	if _, err := st.GetEntry(ctx, u.ID, "2026-10-17"); err != store.ErrNotFound {
		t.Fatalf("empty check-in made an entry: %v", err)
	}
}

// A member's Habits facts fill only the member's journal; the owner's never do.
func TestHabitsJournalIsPerPerson(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"), log)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := New(&config.Config{}, st, log, nil)
	owner := &store.User{Email: "owner@x", Role: "admin"}
	member := &store.User{Email: "member@x", Role: "member"}
	_ = st.CreateUser(ctx, owner)
	_ = st.CreateUser(ctx, member)
	b, _ := json.Marshal(sources.Checkin{Journal: "owner's private line"})
	_ = st.PutFact(ctx, owner.ID, &store.Fact{Date: "2026-10-14", Source: "habits", Payload: b})
	s.syncAllHabitsEntries(ctx)
	if e, err := st.GetEntry(ctx, owner.ID, "2026-10-14"); err != nil || e.Highlight != "owner's private line" {
		t.Fatalf("owner entry = %+v, %v", e, err)
	}
	if _, err := st.GetEntry(ctx, member.ID, "2026-10-14"); err != store.ErrNotFound {
		t.Fatalf("member got the owner's habits journal: %v", err)
	}
}
