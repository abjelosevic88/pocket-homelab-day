package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// Birthdays reads the birthdays and anniversaries that fall on a day from
// Pocket Contacts (GET /api/v1/birthdays?from=<day>&days=1), the same list its
// own reminders use: one all-day item per person, with the age they turn.
type Birthdays struct{ Conn pocket.Conn }

// NewBirthdays creates a client over a connection (a Pocket Core link or a token).
func NewBirthdays(conn pocket.Conn) *Birthdays { return &Birthdays{Conn: conn} }

// Name implements Fetcher.
func (p *Birthdays) Name() string { return "birthdays" }

type contactsBirthday struct {
	Contact struct {
		ID int64  `json:"id"`
		FN string `json:"fn"`
	} `json:"contact"`
	Kind string `json:"kind"` // birthday | anniversary
	Date string `json:"date"`
	Age  *int   `json:"age"`
}

func (p *Birthdays) list(ctx context.Context, date string) ([]contactsBirthday, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var out []contactsBirthday
	err = c.Get(ctx, "/api/v1/birthdays", url.Values{"from": {date}, "days": {"1"}}, &out)
	return out, err
}

// Test implements Fetcher: today's birthdays, the same call a pull makes.
func (p *Birthdays) Test(ctx context.Context) (string, error) {
	if _, err := p.list(ctx, time.Now().Format("2006-01-02")); err != nil {
		return "", err
	}
	return "Pocket Contacts reachable", nil
}

// Fetch implements Fetcher.
func (p *Birthdays) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	if _, _, err := DayBounds(date, loc); err != nil {
		return nil, err
	}
	rows, err := p.list(ctx, date)
	if err != nil {
		return nil, err
	}
	act := Activity{}
	for _, b := range rows {
		if b.Date != date || strings.TrimSpace(b.Contact.FN) == "" {
			continue
		}
		it := ActivityItem{Title: "🎂 " + b.Contact.FN, Sub: "birthday", Group: "birthdays", Ref: fmt.Sprint(b.Contact.ID)}
		if b.Kind == "anniversary" {
			it.Title, it.Sub, it.Group = "💍 "+b.Contact.FN, "anniversary", "anniversaries"
		}
		if b.Age != nil && *b.Age > 0 {
			it.Note = fmt.Sprintf("turns %d", *b.Age)
			if b.Kind == "anniversary" {
				it.Note = Plural(*b.Age, "year", "years")
			}
		}
		act.Items = append(act.Items, it)
	}
	act.Finish()
	return &Result{Payload: act, Headline: float64(act.Count), Summary: Plural(act.Count, "birthday", "birthdays")}, nil
}
