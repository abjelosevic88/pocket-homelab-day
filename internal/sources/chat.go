package sources

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Chat ----------------

// Chat reads the day in Pocket Chat: how busy each conversation was, the
// plans people suggested (a day and maybe a time found in a message) and the
// cards shared from other Pocket apps. The words of the messages stay in the
// Chat card: the summary and the AI's brief say which conversations, when the
// plans are for and what was shared, never what anyone wrote.
type Chat struct{ Conn pocket.Conn }

// NewChat creates a fetcher over a connection.
func NewChat(conn pocket.Conn) *Chat { return &Chat{Conn: conn} }

// Name implements Fetcher.
func (c *Chat) Name() string { return "chat" }

// Test implements Fetcher.
func (c *Chat) Test(ctx context.Context) (string, error) {
	cl, err := c.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Chat " + v, nil
}

// ChatConversation is a conversation with how many messages it had that day.
type ChatConversation struct {
	ID       int64  `json:"conversation_id"`
	Title    string `json:"title"`
	Messages int    `json:"messages"`
	Mine     int    `json:"mine"`
}

// ChatPlan is a day (and maybe a time) someone suggested in a message.
type ChatPlan struct {
	Conversation   string `json:"conversation"`
	From           string `json:"from,omitempty"`
	Text           string `json:"text,omitempty"` // the message, for the Chat card only
	Date           string `json:"date,omitempty"` // YYYY-MM-DD the plan is for
	Time           string `json:"time,omitempty"` // HH:MM
	MessageID      int64  `json:"message_id"`
	ConversationID int64  `json:"conversation_id"`
}

// ChatShared is a card shared into a conversation from another Pocket app.
type ChatShared struct {
	Conversation   string `json:"conversation"`
	From           string `json:"from,omitempty"`
	App            string `json:"app,omitempty"` // the app's slug: kitchen, calendar, places…
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle,omitempty"`
	MessageID      int64  `json:"message_id"`
	ConversationID int64  `json:"conversation_id"`
}

// ChatDay is the day in Pocket Chat.
type ChatDay struct {
	Date          string             `json:"date"`
	Messages      int                `json:"messages"`
	Mine          int                `json:"mine"`
	Photos        int                `json:"photos"`
	Voice         int                `json:"voice"`
	Conversations []ChatConversation `json:"conversations"`
	Plans         []ChatPlan         `json:"plans"`
	Shared        []ChatShared       `json:"shared"`
}

// Fetch implements Fetcher.
func (c *Chat) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := c.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Messages      int                `json:"messages"`
		Mine          int                `json:"mine"`
		Photos        int                `json:"photos"`
		Voice         int                `json:"voice"`
		Conversations []ChatConversation `json:"conversations"`
		Plans         []ChatPlan         `json:"plans"`
		Shared        []ChatShared       `json:"shared"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := ChatDay{Date: date, Messages: page.Messages, Mine: page.Mine, Photos: page.Photos, Voice: page.Voice,
		Conversations: []ChatConversation{}, Plans: []ChatPlan{}, Shared: []ChatShared{}}
	title := func(s string) string { return firstNonEmptyStr(strings.TrimSpace(s), "a conversation") }
	for _, cv := range page.Conversations {
		if cv.Messages <= 0 {
			continue
		}
		cv.Title = title(cv.Title)
		d.Conversations = append(d.Conversations, cv)
	}
	sort.SliceStable(d.Conversations, func(i, j int) bool { return d.Conversations[i].Messages > d.Conversations[j].Messages })
	for _, p := range page.Plans {
		p.Conversation, p.From, p.Text = title(p.Conversation), strings.TrimSpace(p.From), strings.TrimSpace(p.Text)
		d.Plans = append(d.Plans, p)
	}
	for _, s := range page.Shared {
		s.Conversation, s.From, s.App = title(s.Conversation), strings.TrimSpace(s.From), strings.TrimSpace(s.App)
		s.Title, s.Subtitle = firstNonEmptyStr(strings.TrimSpace(s.Title), "Untitled"), strings.TrimSpace(s.Subtitle)
		d.Shared = append(d.Shared, s)
	}
	return &Result{Payload: d, Headline: float64(d.Messages), Summary: chatSummary(d)}, nil
}

// chatPlanWhen says when a plan is for, seen from the day it was made:
// "20:00 that day", "Friday 20:00", "12 Dec", "3 Jan 2027".
func chatPlanWhen(day string, p ChatPlan) string {
	t, err := time.Parse("2006-01-02", p.Date)
	d, err2 := time.Parse("2006-01-02", day)
	if err != nil || err2 != nil {
		return p.Time
	}
	var w string
	switch n := int(t.Sub(d).Hours() / 24); {
	case n == 0:
		return JoinNonEmpty(" ", p.Time, "that day")
	case n > 0 && n < 7:
		w = t.Format("Monday")
	case t.Year() == d.Year():
		w = t.Format("2 Jan")
	default:
		w = t.Format("2 Jan 2006")
	}
	return JoinNonEmpty(" ", w, p.Time)
}

// chatSummary: "Chatted in Family (8 messages), Weekend hikers (4); 2 photos;
// plans: Friday 20:00 (Family); 1 thing shared". The plans say when and
// where, never the words of the message.
func chatSummary(d ChatDay) string {
	if d.Messages == 0 && len(d.Conversations) == 0 {
		return ""
	}
	var parts []string
	if len(d.Conversations) > 0 {
		var cs []string
		for i, cv := range d.Conversations {
			if i == 3 {
				cs[len(cs)-1] += " and " + Plural(len(d.Conversations)-3, "more", "more")
				break
			}
			if i == 0 {
				cs = append(cs, cv.Title+" ("+Plural(cv.Messages, "message", "messages")+")")
			} else {
				cs = append(cs, cv.Title+" ("+strconv.Itoa(cv.Messages)+")")
			}
		}
		parts = append(parts, "Chatted in "+strings.Join(cs, ", "))
	} else {
		parts = append(parts, "Chatted ("+Plural(d.Messages, "message", "messages")+")")
	}
	var media []string
	if d.Photos > 0 {
		media = append(media, Plural(d.Photos, "photo", "photos"))
	}
	if d.Voice > 0 {
		media = append(media, Plural(d.Voice, "voice message", "voice messages"))
	}
	if len(media) > 0 {
		parts = append(parts, strings.Join(media, ", "))
	}
	if len(d.Plans) > 0 {
		var ps []string
		for i, p := range d.Plans {
			if i == 3 {
				ps[len(ps)-1] += " and " + Plural(len(d.Plans)-3, "more", "more")
				break
			}
			ps = append(ps, JoinNonEmpty(" ", firstNonEmptyStr(chatPlanWhen(d.Date, p), "a plan"), "("+p.Conversation+")"))
		}
		parts = append(parts, "plans: "+strings.Join(ps, ", "))
	}
	if len(d.Shared) > 0 {
		parts = append(parts, Plural(len(d.Shared), "thing", "things")+" shared")
	}
	return strings.Join(parts, "; ")
}
