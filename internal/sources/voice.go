package sources

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Voice ----------------

// Voice reads the day in Pocket Voice: the memos the person recorded, what
// they said (a journal memo's summary or words, the others' titles) and where
// each went (tasks, a calendar event, a note, an expense).
type Voice struct{ Conn pocket.Conn }

// NewVoice creates a fetcher over a connection.
func NewVoice(conn pocket.Conn) *Voice { return &Voice{Conn: conn} }

// Name implements Fetcher.
func (v *Voice) Name() string { return "voice" }

// Test implements Fetcher.
func (v *Voice) Test(ctx context.Context) (string, error) {
	cl, err := v.Conn(ctx)
	if err != nil {
		return "", err
	}
	ver, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Voice " + ver, nil
}

// VoiceSent is where a memo went: a task, a calendar event, a note, today's
// daily note or an expense in another Pocket app.
type VoiceSent struct {
	App   string `json:"app"`  // notes | tasks | calendar | money
	Kind  string `json:"kind"` // note | daily | task | event | expense
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

// VoiceMemo is a memo recorded that day and written out.
type VoiceMemo struct {
	ID         int64       `json:"id"`
	Title      string      `json:"title"`
	Kind       string      `json:"kind,omitempty"` // note todo idea journal meeting shopping reminder
	Time       string      `json:"time,omitempty"` // HH:MM
	DurationMS int64       `json:"duration_ms,omitempty"`
	Length     string      `json:"length,omitempty"` // "0:21"
	Summary    string      `json:"summary,omitempty"`
	Text       string      `json:"text,omitempty"` // the tidy text or the words, at most 600 characters
	Sent       []VoiceSent `json:"sent"`
	Went       string      `json:"went,omitempty"` // "3 tasks, a calendar event"
	URL        string      `json:"url,omitempty"`
	Line       string      `json:"line"` // "Voice memo at 08:12: Dentist, dry cleaning and photos for Amra (0:21) → 3 tasks"
}

// VoiceDay is the day in Pocket Voice: the memos written out, oldest first.
type VoiceDay struct {
	Memos      []VoiceMemo `json:"memos"`
	DurationMS int64       `json:"duration_ms"`
}

// Fetch implements Fetcher.
func (v *Voice) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := v.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Memos []struct {
			ID         int64       `json:"id"`
			Title      string      `json:"title"`
			Kind       string      `json:"kind"`
			RecordedAt string      `json:"recorded_at"`
			Time       string      `json:"time"`
			DurationMS int64       `json:"duration_ms"`
			Summary    string      `json:"summary"`
			Text       string      `json:"text"`
			Status     string      `json:"status"`
			Sent       []VoiceSent `json:"sent"`
			URL        string      `json:"url"`
		} `json:"memos"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := VoiceDay{Memos: []VoiceMemo{}}
	var journal, others []string
	for _, r := range page.Memos {
		if r.Status != "" && r.Status != "done" {
			continue // still being written out, or it could not be: the next pull has it
		}
		m := VoiceMemo{ID: r.ID, Title: strings.TrimSpace(r.Title), Kind: r.Kind, Time: firstNonBlank(clockOf(r.Time), clockIn(r.RecordedAt, loc)),
			DurationMS: r.DurationMS, Length: voiceLength(r.DurationMS), Summary: strings.TrimSpace(r.Summary), Text: strings.TrimSpace(r.Text),
			Sent: []VoiceSent{}, URL: r.URL}
		for _, s := range r.Sent {
			if s.Kind == "" {
				s.Kind = voiceAppKind[s.App]
			}
			m.Sent = append(m.Sent, s)
		}
		m.Went = voiceWent(m.Sent)
		m.Line = voiceLine(m)
		d.Memos = append(d.Memos, m)
		d.DurationMS += m.DurationMS
		if m.Kind == "journal" {
			journal = append(journal, m.Line)
		} else {
			others = append(others, m.Line)
		}
	}
	// a journal memo is the person's own account of the day: it leads
	parts := append(journal, others...)
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = strings.Join(parts[:3], "; ") + " and " + Plural(len(parts)-3, "more memo", "more memos")
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Memos)), Summary: sum}, nil
}

// voiceLine: "Voice memo at 08:12: Dentist, dry cleaning and photos for Amra
// (0:21) → 3 tasks, a calendar event"; a journal memo says what was said:
// "Voice journal at 21:40: A slow Sunday by the river with Amra (2:05)".
func voiceLine(m VoiceMemo) string {
	s, what := "Voice memo", m.Title
	if m.Kind == "journal" {
		s, what = "Voice journal", firstNonBlank(m.Summary, clipWords(m.Text, 200), m.Title)
	}
	if what == "" {
		what = firstNonBlank(m.Summary, clipWords(m.Text, 80))
	}
	if strings.HasSuffix(what, ".") && !strings.HasSuffix(what, "..") {
		what = strings.TrimSuffix(what, ".") // a sentence, but the line goes on
	}
	if m.Time != "" {
		s += " at " + m.Time
	}
	if what != "" {
		s += ": " + what
	}
	if m.Length != "" {
		s += " (" + m.Length + ")"
	}
	if m.Went != "" {
		s += " → " + m.Went
	}
	return s
}

// voiceAppKind is what a memo sent to an app became when Voice does not say.
var voiceAppKind = map[string]string{"tasks": "task", "calendar": "event", "money": "expense", "notes": "note"}

// voiceWent says where a memo went: "3 tasks, a calendar event", "your daily
// note", "a note, an expense".
func voiceWent(sent []VoiceSent) string {
	n := map[string]int{}
	for _, s := range sent {
		n[s.Kind]++
	}
	var out []string
	for _, k := range []struct{ kind, one, many string }{
		{"task", "a task", "tasks"}, {"event", "a calendar event", "calendar events"}, {"expense", "an expense", "expenses"},
		{"note", "a note", "notes"}, {"daily", "your daily note", ""},
	} {
		switch c := n[k.kind]; {
		case c == 0:
		case c == 1 || k.many == "":
			out = append(out, k.one)
		default:
			out = append(out, fmt.Sprintf("%d %s", c, k.many))
		}
	}
	return strings.Join(out, ", ")
}

// voiceLength: "0:21", "4:05", "1:02:03"; "" for no length.
func voiceLength(ms int64) string {
	sec := (ms + 500) / 1000
	if sec <= 0 {
		return ""
	}
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// clipWords shortens a text to about n characters at a word, with "…".
func clipWords(t string, n int) string {
	t = strings.Join(strings.Fields(t), " ")
	if utf8.RuneCountInString(t) <= n {
		return t
	}
	s := string([]rune(t)[:n])
	if i := strings.LastIndex(s, " "); i > len(s)/2 {
		s = s[:i]
	}
	return strings.TrimRight(s, ",;:.-–— ") + "…"
}
