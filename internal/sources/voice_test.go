package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestVoiceDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-05" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-05",
			"memos": []map[string]any{
				{"id": 12, "title": "Dentist, dry cleaning and photos for Amra", "kind": "todo", "recorded_at": "2026-11-05T07:12:00Z", "time": "08:12", "duration_ms": 21400,
					"summary": "Book the dentist, pick up the dry cleaning and send Amra the photos.", "text": "Remind me to book the dentist…", "status": "done",
					"sent": []map[string]any{
						{"app": "tasks", "kind": "task", "ref": "41", "title": "Book the dentist", "url": "https://tasks.x/t/41"},
						{"app": "tasks", "kind": "task", "ref": "42", "title": "Pick up the dry cleaning", "url": "https://tasks.x/t/42"},
						{"app": "tasks", "kind": "task", "ref": "43", "title": "Send Amra the photos", "url": "https://tasks.x/t/43"},
						{"app": "calendar", "kind": "event", "ref": "9", "title": "Dentist", "url": "https://calendar.x/e/9"},
					},
					"url": "https://voice.x/memos/12"},
				{"id": 13, "title": "Lunch with Marko", "kind": "note", "time": "12:40", "duration_ms": 9000, "status": "working", "sent": []any{}, "url": "https://voice.x/memos/13"},
				{"id": 14, "title": "", "kind": "idea", "time": "13:05", "duration_ms": 3000, "status": "failed", "sent": []any{}, "url": "https://voice.x/memos/14"},
				{"id": 15, "title": "Evening walk", "kind": "journal", "recorded_at": "2026-11-05T20:40:00Z", "time": "21:40", "duration_ms": 125000,
					"summary": "A slow evening by the river with Amra; the bike is finally fixed.", "text": "So today was slow…", "status": "done",
					"sent": []map[string]any{{"app": "notes", "kind": "daily", "ref": "2026-11-05", "title": "5 November", "url": "https://notes.x/daily/2026-11-05"}},
					"url":  "https://voice.x/memos/15"},
				{"id": 16, "title": "Garage receipt", "kind": "note", "time": "17:02", "duration_ms": 6000, "status": "done",
					"sent": []map[string]any{{"app": "money", "ref": "77", "title": "Car wash", "url": "https://money.x/t/77"}}, "url": "https://voice.x/memos/16"},
			},
			"count": 5, "duration_ms": 164400,
		})
	}))
	defer ts.Close()
	v := NewVoice(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := v.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := "Voice journal at 21:40: A slow evening by the river with Amra; the bike is finally fixed (2:05) → your daily note; " +
		"Voice memo at 08:12: Dentist, dry cleaning and photos for Amra (0:21) → 3 tasks, a calendar event; " +
		"Voice memo at 17:02: Garage receipt (0:06) → an expense"
	if res.Headline != 3 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(VoiceDay)
	if len(d.Memos) != 3 || d.DurationMS != 152400 {
		t.Fatalf("memos (the one being written out and the failed one left out): %+v", d)
	}
	m := d.Memos[0]
	if m.ID != 12 || m.Time != "08:12" || m.Length != "0:21" || m.Went != "3 tasks, a calendar event" || m.URL != "https://voice.x/memos/12" ||
		!strings.HasPrefix(m.Summary, "Book the dentist") || len(m.Sent) != 4 || m.Sent[3].URL != "https://calendar.x/e/9" {
		t.Errorf("memo %+v", m)
	}
	if e := d.Memos[2]; e.Sent[0].Kind != "expense" || e.Went != "an expense" {
		t.Errorf("a memo sent to Pocket Money without a kind: %+v", e)
	}
}

func TestVoiceLines(t *testing.T) {
	for _, c := range []struct {
		m    VoiceMemo
		want string
	}{
		{VoiceMemo{Title: "Shopping list", Kind: "shopping", Time: "18:03", Length: "0:12"}, "Voice memo at 18:03: Shopping list (0:12)"},
		{VoiceMemo{Kind: "idea", Summary: "A shelf for the hallway"}, "Voice memo: A shelf for the hallway"},
		{VoiceMemo{Kind: "note", Text: "Call the bank about the card"}, "Voice memo: Call the bank about the card"},
		{VoiceMemo{Title: "Today", Kind: "journal", Time: "22:10", Text: "Long day at work, then dinner with Ana at Kibe Mahala."}, "Voice journal at 22:10: Long day at work, then dinner with Ana at Kibe Mahala"},
		{VoiceMemo{Title: "Today", Kind: "journal"}, "Voice journal: Today"},
		{VoiceMemo{Time: "07:00"}, "Voice memo at 07:00"},
	} {
		if got := voiceLine(c.m); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
	if got := voiceWent([]VoiceSent{{Kind: "note"}, {Kind: "note"}, {Kind: "daily"}, {Kind: "daily"}, {Kind: "event"}}); got != "a calendar event, 2 notes, your daily note" {
		t.Errorf("went %q", got)
	}
	for ms, want := range map[int64]string{0: "", 400: "", 21400: "0:21", 245000: "4:05", 3723000: "1:02:03"} {
		if got := voiceLength(ms); got != want {
			t.Errorf("length %d: %q want %q", ms, got, want)
		}
	}
	long := strings.Repeat("word ", 60)
	if got := clipWords(long, 200); !strings.HasSuffix(got, "word…") || len([]rune(got)) > 201 {
		t.Errorf("clip %q", got)
	}
}

// More than three memos: the first three and how many more.
func TestVoiceSummaryCapped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var memos []map[string]any
		for i, title := range []string{"One", "Two", "Three", "Four", "Five"} {
			memos = append(memos, map[string]any{"id": i + 1, "title": title, "kind": "note", "time": "09:0" + string(rune('0'+i)), "status": "done", "sent": []any{}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"memos": memos})
	}))
	defer ts.Close()
	v := NewVoice(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := v.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Voice memo at 09:00: One; Voice memo at 09:01: Two; Voice memo at 09:02: Three and 2 more memos"; res.Headline != 5 || res.Summary != want {
		t.Fatalf("got %v %q", res.Headline, res.Summary)
	}
}
