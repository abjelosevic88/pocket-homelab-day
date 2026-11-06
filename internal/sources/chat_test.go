package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestChatDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-04" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2026-11-04","messages":12,"mine":3,"photos":2,"voice":1,
			"conversations":[{"conversation_id":4,"title":"Weekend hikers","messages":4,"mine":1},{"conversation_id":2,"title":"Family","messages":8,"mine":2}],
			"plans":[{"conversation":"Family","from":"Ana","text":"Dinner at Kibe Mahala on Friday at 20:00? ","date":"2026-11-06","time":"20:00","message_id":91,"conversation_id":2}],
			"shared":[{"conversation":"Family","from":"Ana","app":"kitchen","title":"Grandma's stuffed peppers","subtitle":"Dinner · 4 servings","message_id":95,"conversation_id":2}],
			"summary":"12 messages, 3 by you: Family (8), Weekend hikers (4); 2 photos"}`))
	}))
	defer ts.Close()
	c := NewChat(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := c.Fetch(context.Background(), "2026-11-04", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Chatted in Family (8 messages), Weekend hikers (4); 2 photos, 1 voice message; plans: Friday 20:00 (Family); 1 thing shared"; res.Headline != 12 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	if strings.Contains(res.Summary, "Kibe") || strings.Contains(res.Summary, "Dinner") {
		t.Fatalf("the summary carries the words of a message: %q", res.Summary)
	}
	d := res.Payload.(ChatDay)
	if d.Date != "2026-11-04" || d.Mine != 3 || d.Photos != 2 || d.Voice != 1 || d.Conversations[0].ID != 2 || d.Conversations[0].Mine != 2 {
		t.Fatalf("day %+v", d)
	}
	if p := d.Plans[0]; p.Text != "Dinner at Kibe Mahala on Friday at 20:00?" || p.MessageID != 91 || p.ConversationID != 2 || p.From != "Ana" {
		t.Fatalf("plan %+v", p)
	}
	if s := d.Shared[0]; s.App != "kitchen" || s.Title != "Grandma's stuffed peppers" || s.MessageID != 95 {
		t.Fatalf("shared %+v", s)
	}
}

func TestChatPlanWhen(t *testing.T) {
	for _, c := range []struct {
		p    ChatPlan
		want string
	}{
		{ChatPlan{Date: "2026-11-04", Time: "20:00"}, "20:00 that day"},
		{ChatPlan{Date: "2026-11-04"}, "that day"},
		{ChatPlan{Date: "2026-11-05"}, "Thursday"},
		{ChatPlan{Date: "2026-11-11", Time: "09:30"}, "11 Nov 09:30"},
		{ChatPlan{Date: "2027-01-03"}, "3 Jan 2027"},
		{ChatPlan{Time: "19:00"}, "19:00"},
	} {
		if got := chatPlanWhen("2026-11-04", c.p); got != c.want {
			t.Errorf("%+v: got %q want %q", c.p, got, c.want)
		}
	}
}

func TestChatSummary(t *testing.T) {
	cs := func(titles ...string) []ChatConversation {
		var out []ChatConversation
		for i, s := range titles {
			out = append(out, ChatConversation{ID: int64(i + 1), Title: s, Messages: 10 - i})
		}
		return out
	}
	for _, c := range []struct {
		d    ChatDay
		want string
	}{
		{ChatDay{}, ""},
		{ChatDay{Messages: 1, Conversations: []ChatConversation{{ID: 1, Title: "Ana", Messages: 1}}}, "Chatted in Ana (1 message)"},
		{ChatDay{Messages: 34, Conversations: cs("A", "B", "C", "D", "E")}, "Chatted in A (10 messages), B (9), C (8) and 2 more"},
		{ChatDay{Date: "2026-11-04", Messages: 3, Conversations: cs("A"), Shared: []ChatShared{{Title: "x"}, {Title: "y"}},
			Plans: []ChatPlan{{Conversation: "A", Date: "2026-11-04", Time: "20:00"}, {Conversation: "A"}, {Conversation: "B", Date: "2026-11-06"}, {Conversation: "C", Date: "2026-11-07"}}},
			"Chatted in A (10 messages); plans: 20:00 that day (A), a plan (A), Friday (B) and 1 more; 2 things shared"},
	} {
		if got := chatSummary(c.d); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}

func TestChatEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-05","messages":0,"mine":0,"photos":0,"voice":0,"conversations":[],"plans":[],"shared":[],"summary":"No messages."}`))
	}))
	defer ts.Close()
	c := NewChat(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := c.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(ChatDay); res.Headline != 0 || res.Summary != "" || d.Conversations == nil || d.Plans == nil || len(d.Shared) != 0 {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}
