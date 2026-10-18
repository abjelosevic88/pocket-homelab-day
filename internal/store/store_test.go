package store

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestNormalizeTags(t *testing.T) {
	cases := map[string]string{"": "", "Work, #family ,work": "work,family", " , ,a": "a", "#A,b,a": "a,b"}
	for in, want := range cases {
		if got := NormalizeTags(in); got != want {
			t.Errorf("NormalizeTags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEntryUpsertAndEmptyDeletes(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := &User{Email: "a@b", Role: "admin"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	e := &Entry{UserID: u.ID, Date: "2026-10-01", Highlight: "Hello", Tags: "X, y"}
	if err := st.UpsertEntry(ctx, e); err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 || e.Tags != "x,y" || e.Words != 1 {
		t.Fatalf("unexpected entry %+v", e)
	}
	e.Text = "two words"
	_ = st.UpsertEntry(ctx, e)
	got, err := st.GetEntry(ctx, u.ID, "2026-10-01")
	if err != nil || got.Text != "two words" || got.Words != 3 {
		t.Fatalf("got %+v, %v", got, err)
	}
	// emptying every field removes the row
	if err := st.UpsertEntry(ctx, &Entry{UserID: u.ID, Date: "2026-10-01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetEntry(ctx, u.ID, "2026-10-01"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStreakAndMemories(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := &User{Email: "a@b", Role: "admin"}
	_ = st.CreateUser(ctx, u)
	for _, d := range []string{"2025-10-18", "2026-10-10", "2026-10-11", "2026-10-12", "2026-10-15", "2026-10-16", "2026-10-17"} {
		if err := st.UpsertEntry(ctx, &Entry{UserID: u.ID, Date: d, Highlight: d}); err != nil {
			t.Fatal(err)
		}
	}
	cur, longest := st.Streak(ctx, u.ID, "2026-10-18") // today not written yet: count from yesterday
	if cur != 3 || longest != 3 {
		t.Fatalf("streak = %d/%d, want 3/3", cur, longest)
	}
	cur, _ = st.Streak(ctx, u.ID, "2026-10-20")
	if cur != 0 {
		t.Fatalf("streak after a gap = %d, want 0", cur)
	}
	mem, err := st.Memories(ctx, u.ID, "2026-10-18")
	if err != nil || len(mem) != 1 || mem[0].Date != "2025-10-18" {
		t.Fatalf("memories = %+v, %v", mem, err)
	}
	prev, next := st.Neighbours(ctx, u.ID, "2026-10-13")
	if prev != "2026-10-12" || next != "2026-10-15" {
		t.Fatalf("neighbours = %s %s", prev, next)
	}
}

func TestFactsKeepPayloadOnError(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.PutFact(ctx, &Fact{Date: "2026-10-01", Source: "maloja", Payload: []byte(`{"listens":3}`), Headline: 3, Summary: "3 listens"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutFactError(ctx, "2026-10-01", "maloja", "boom"); err != nil {
		t.Fatal(err)
	}
	f, err := st.FactsForDay(ctx, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	m := f["maloja"]
	if string(m.Payload) != `{"listens":3}` || m.Error != "boom" || m.Headline != 3 {
		t.Fatalf("fact = %+v", m)
	}
	if st.FactSources(ctx, "2026-10-01")["maloja"] {
		t.Fatal("a fact with an error must not count as present")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	s, _ := st.GetSettings(ctx)
	src := s.Sources["immich"]
	src.URL, src.Key = "http://x", "k"
	s.Sources["immich"] = src
	s.HomeLat, s.HomeLon = 44.77, 17.19
	if err := st.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetSettings(ctx)
	if got.Sources["immich"].URL != "http://x" || !got.Sources["immich"].Configured("immich") || got.HomeLat != 44.77 {
		t.Fatalf("settings = %+v", got)
	}
	if got.Sources["dawarich"].Configured("dawarich") {
		t.Fatal("unset source must not be configured")
	}
	if !got.Sources["weather"].Configured("weather") {
		t.Fatal("weather needs no URL")
	}
}
