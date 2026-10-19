package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func (s *Server) handleNotifyTestWith(w http.ResponseWriter, r *http.Request, url, format string) {
	n := notify.New(url, format)
	if err := n.Send(r.Context(), notify.Message{Title: "Pocket Day", Body: "Notifications work. The evening prompt and the morning memories will arrive here.", Priority: "info", Tags: []string{"notebook"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Test message sent via " + n.Format})
}

// handleBackup downloads the whole database file. It holds every account's
// journal, so it is a box operation for the admin only (the same bytes the
// nightly file backup copies); a member gets 404 and uses the JSON export.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if u := userFrom(r.Context()); u == nil || u.Role != "admin" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-day-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, dst)
}

// exportDoc is the JSON backup of one user's journal plus their facts.
type exportDoc struct {
	App      string         `json:"app"`
	Version  string         `json:"version"`
	Exported string         `json:"exported_at"`
	User     string         `json:"user"`
	Entries  []store.Entry  `json:"entries"`
	Moments  []store.Moment `json:"moments"`
	Pins     []store.Pin    `json:"pins"`
	Facts    []store.Fact   `json:"facts,omitempty"`
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	doc := exportDoc{App: "pocket-day", Version: Version, Exported: store.Now(), User: u.Email}
	doc.Entries, _ = s.store.AllEntries(ctx, u.ID)
	doc.Moments, _ = s.store.ListMoments(ctx, u.ID, "0000-01-01", "9999-12-31", 10000000)
	doc.Pins, _ = s.store.ListPins(ctx, u.ID, "0000-01-01", "9999-12-31")
	if !queryBool(r, "nofacts") {
		doc.Facts, _ = s.store.FactsRange(ctx, u.ID, "0000-01-01", "9999-12-31")
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-day-export.json"`)
	writeJSON(w, http.StatusOK, doc)
}

// handleExportMarkdown writes the journal as one Markdown file, a heading per day.
func (s *Server) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	entries, _ := s.store.AllEntries(ctx, u.ID)
	moments, _ := s.store.ListMoments(ctx, u.ID, "0000-01-01", "9999-12-31", 10000000)
	byDay := map[string][]store.Moment{}
	for _, m := range moments {
		byDay[m.Date] = append(byDay[m.Date], m)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Journal\n\n")
	moods := []string{"", "rough", "meh", "okay", "good", "great"}
	for _, e := range entries {
		fmt.Fprintf(&b, "## %s\n\n", e.Date)
		if e.Highlight != "" {
			fmt.Fprintf(&b, "**%s**\n\n", e.Highlight)
		}
		var meta []string
		if e.Mood > 0 {
			meta = append(meta, "mood: "+moods[e.Mood])
		}
		if e.Tags != "" {
			meta = append(meta, "tags: "+strings.ReplaceAll(e.Tags, ",", ", "))
		}
		if len(meta) > 0 {
			fmt.Fprintf(&b, "_%s_\n\n", strings.Join(meta, " · "))
		}
		if e.Text != "" {
			fmt.Fprintf(&b, "%s\n\n", e.Text)
		}
		for _, m := range byDay[e.Date] {
			if m.Time != "" {
				fmt.Fprintf(&b, "- %s %s\n", m.Time, m.Text)
			} else {
				fmt.Fprintf(&b, "- %s\n", m.Text)
			}
		}
		if len(byDay[e.Date]) > 0 {
			b.WriteString("\n")
		}
		delete(byDay, e.Date)
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="journal.md"`)
	_, _ = w.Write([]byte(b.String()))
}

// handleImportJSON merges an export into the caller's journal: entries by
// date (existing text is kept, missing fields filled), moments and pins added
// when not already present, facts (into the caller's own) only for days that
// have none. Owner ids in the file are ignored: everything becomes the caller's.
func (s *Server) handleImportJSON(w http.ResponseWriter, r *http.Request) {
	var doc exportDoc
	if err := decode(r, &doc); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := r.Context()
	u := userFrom(ctx)
	counts := map[string]int{}
	for _, e := range doc.Entries {
		if !validDate(e.Date) {
			continue
		}
		cur, err := s.store.GetEntry(ctx, u.ID, e.Date)
		if err != nil {
			cur = &store.Entry{UserID: u.ID, Date: e.Date}
		}
		changed := false
		if cur.Highlight == "" && e.Highlight != "" {
			cur.Highlight, changed = e.Highlight, true
		}
		if cur.Text == "" && e.Text != "" {
			cur.Text, changed = e.Text, true
		}
		if cur.Mood == 0 && e.Mood > 0 {
			cur.Mood, changed = e.Mood, true
		}
		if e.Tags != "" {
			t := store.NormalizeTags(cur.Tags + "," + e.Tags)
			if t != cur.Tags {
				cur.Tags, changed = t, true
			}
		}
		if e.Pinned && !cur.Pinned {
			cur.Pinned, changed = true, true
		}
		if changed {
			if err := s.store.UpsertEntry(ctx, cur); err == nil {
				counts["entries"]++
			}
		}
	}
	existing, _ := s.store.ListMoments(ctx, u.ID, "0000-01-01", "9999-12-31", 10000000)
	have := map[string]bool{}
	for _, m := range existing {
		have[m.Date+"|"+m.Time+"|"+m.Text] = true
	}
	for _, m := range doc.Moments {
		if !validDate(m.Date) || strings.TrimSpace(m.Text) == "" || have[m.Date+"|"+m.Time+"|"+m.Text] {
			continue
		}
		m.ID, m.UserID = 0, u.ID
		if err := s.store.CreateMoment(ctx, &m); err == nil {
			counts["moments"]++
		}
	}
	for _, p := range doc.Pins {
		if !validDate(p.Date) || p.Source == "" {
			continue
		}
		p.ID, p.UserID = 0, u.ID
		if err := s.store.CreatePin(ctx, &p); err == nil {
			counts["pins"]++
		}
	}
	// facts go into the caller's own facts, only for days that have none yet
	for _, f := range doc.Facts {
		if !validDate(f.Date) || f.Source == "" || f.Error != "" || len(f.Payload) <= 4 {
			continue
		}
		if s.store.FactSources(ctx, u.ID, f.Date)[f.Source] {
			continue
		}
		ff := f
		if err := s.store.PutFact(ctx, u.ID, &ff); err == nil {
			counts["facts"]++
		}
	}
	writeJSON(w, http.StatusOK, counts)
}
