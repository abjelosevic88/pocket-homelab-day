package store

import (
	"context"
	"database/sql"
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
	u := &User{Email: "a@b", Role: "admin"}
	_ = st.CreateUser(ctx, u)
	if err := st.PutFact(ctx, u.ID, &Fact{Date: "2026-10-01", Source: "maloja", Payload: []byte(`{"listens":3}`), Headline: 3, Summary: "3 listens"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutFactError(ctx, u.ID, "2026-10-01", "maloja", "boom"); err != nil {
		t.Fatal(err)
	}
	f, err := st.FactsForDay(ctx, u.ID, "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	m := f["maloja"]
	if string(m.Payload) != `{"listens":3}` || m.Error != "boom" || m.Headline != 3 {
		t.Fatalf("fact = %+v", m)
	}
	if st.FactSources(ctx, u.ID, "2026-10-01")["maloja"] {
		t.Fatal("a fact with an error must not count as present")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := &User{Email: "a@b", Role: "admin"}
	_ = st.CreateUser(ctx, u)
	s, _ := st.GetSettings(ctx, u.ID)
	src := s.Sources["immich"]
	src.URL, src.Key = "http://x", "k"
	s.Sources["immich"] = src
	s.HomeLat, s.HomeLon = 44.77, 17.19
	if err := st.SaveSettings(ctx, u.ID, s); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetSettings(ctx, u.ID)
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

// TestPerUserMigrationBackfillsOwner builds a database as it was before facts
// and settings were per person and checks that 0004 hands everything to the
// owner (the first admin), leaves members with nothing and keeps the counts.
func TestPerUserMigrationBackfillsOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_init.sql", "0002_ai.sql", "0003_entry_origin.sql"} {
		body, _ := migrationFS.ReadFile("migrations/" + name)
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(name, err)
		}
		_, _ = db.Exec(`INSERT INTO schema_migrations VALUES (?, 'x')`, name)
	}
	for _, q := range []string{
		`INSERT INTO users (id, email, role) VALUES (1, 'owner@x', 'admin'), (2, 'member@x', 'member')`,
		`INSERT INTO entries (user_id, date, highlight) VALUES (1, '2026-10-01', 'mine')`,
		`INSERT INTO facts (date, source, payload, fetched_at) VALUES ('2026-10-01', 'immich', '{}', 'x'), ('2026-10-01', 'maloja', '{}', 'x'), ('2026-10-02', 'immich', '{}', 'x')`,
		`INSERT INTO settings (key, value) VALUES ('title', 'Our Days'), ('setup_complete', '1'), ('ai_key', 'sk-owner'), ('notify_url', 'http://n'), ('src_immich_url', 'http://immich'), ('src_immich_key', 'k')`,
		`INSERT INTO ai_usage (user_id, feature) VALUES (NULL, 'draft'), (1, 'ask')`,
		`INSERT INTO cache (key, value, fetched_at) VALUES ('notify:evening', '2026-10-18', 'x'), ('aiq:1:2026-10-01', 'q', 'x')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	db.Close()

	st, err := Open(path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(1) FROM facts WHERE user_id = 1`).Scan(&n)
	if n != 3 {
		t.Errorf("owner facts = %d, want 3", n)
	}
	if f, _ := st.FactsForDay(ctx, 2, "2026-10-01"); len(f) != 0 {
		t.Errorf("member has facts: %v", f)
	}
	own, _ := st.GetSettings(ctx, 1)
	mem, _ := st.GetSettings(ctx, 2)
	if own.AIKey != "sk-owner" || own.NotifyURL != "http://n" || own.Sources["immich"].Key != "k" || own.Title != "Our Days" || !own.SetupComplete {
		t.Errorf("owner settings: %+v", own)
	}
	if mem.AIKey != "" || mem.NotifyURL != "" || mem.Sources["immich"].Configured("immich") || mem.Title != "Our Days" {
		t.Errorf("member settings: ai=%q notify=%q immich=%+v title=%q", mem.AIKey, mem.NotifyURL, mem.Sources["immich"], mem.Title)
	}
	_ = st.DB.QueryRow(`SELECT COUNT(1) FROM settings WHERE key NOT IN ('title', 'setup_complete')`).Scan(&n)
	if n != 0 {
		t.Errorf("personal keys left in the box-wide settings: %d", n)
	}
	_ = st.DB.QueryRow(`SELECT COUNT(1) FROM ai_usage WHERE user_id = 1`).Scan(&n)
	if n != 2 {
		t.Errorf("owner ai_usage = %d, want 2", n)
	}
	if v, _, ok := st.CacheGet(ctx, "notify:1:evening"); !ok || v != "2026-10-18" {
		t.Errorf("notify marker not moved to the owner: %q %v", v, ok)
	}
	for _, tbl := range ContentTables {
		_ = st.DB.QueryRow(`SELECT COUNT(1) FROM ` + tbl + ` WHERE user_id IS NULL`).Scan(&n)
		if n != 0 {
			t.Errorf("%s has %d rows without an owner", tbl, n)
		}
	}
	// re-opening applies nothing twice
	st.Close()
	st2, err := Open(path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	_ = st2.DB.QueryRow(`SELECT COUNT(1) FROM facts`).Scan(&n)
	if n != 3 {
		t.Errorf("facts after reopen = %d", n)
	}
}

// TestFactsAndSettingsArePerUser: the same day and source can hold different
// facts for two people, and personal settings never mix.
func TestFactsAndSettingsArePerUser(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	a, b := &User{Email: "a@x", Role: "admin"}, &User{Email: "b@x", Role: "member"}
	_ = st.CreateUser(ctx, a)
	_ = st.CreateUser(ctx, b)
	_ = st.PutFact(ctx, a.ID, &Fact{Date: "2026-10-01", Source: "immich", Payload: []byte(`{"n":1}`), Headline: 1})
	_ = st.PutFact(ctx, b.ID, &Fact{Date: "2026-10-01", Source: "immich", Payload: []byte(`{"n":2}`), Headline: 2})
	fa, _ := st.FactsForDay(ctx, a.ID, "2026-10-01")
	fb, _ := st.FactsForDay(ctx, b.ID, "2026-10-01")
	if fa["immich"].Headline != 1 || fb["immich"].Headline != 2 {
		t.Fatalf("facts mixed: %v %v", fa, fb)
	}
	_ = st.DeleteFacts(ctx, b.ID, "")
	if fa, _ := st.FactsForDay(ctx, a.ID, "2026-10-01"); len(fa) != 1 {
		t.Fatal("deleting b's facts removed a's")
	}
	sa, _ := st.GetSettings(ctx, a.ID)
	sa.AIKey = "sk-a"
	_ = st.SaveSettings(ctx, a.ID, sa)
	if sb, _ := st.GetSettings(ctx, b.ID); sb.AIKey != "" {
		t.Fatalf("b sees a's key")
	}
	if st.OwnerID(ctx) != a.ID {
		t.Fatalf("owner = %d", st.OwnerID(ctx))
	}
}
