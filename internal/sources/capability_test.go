package sources

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

func TestDayActivity(t *testing.T) {
	day := &core.Day{Date: "2026-11-27", Items: []core.DayItem{
		{ID: "9", Title: "Evening walk", Kind: "walk", Qty: 3.5},
		{ID: "4", Time: "07:40", End: "08:10", Title: "  Bread\n recipe ", Sub: "Recipes", Kind: "written", Minutes: 30, Done: true, Amount: 450, Currency: "eur", Qty: 2, URL: "https://notes.example/note/4", Image: "https://notes.example/i/4.jpg", Note: "For Sunday"},
		{ID: "5", Title: " "}, // no title: left out
		{ID: "6", Time: "25:99", End: "09:00", Title: "Odd time"},             // not a time: sometime that day
		{ID: "7", Time: "09:00", Title: "Tea", Amount: 1200, Currency: "JPY"}, // the yen has no cents
		{ID: "8", Time: "12:00", Title: "Rent share", Amount: 1500, Currency: "KWD", Kind: "written"},
	}, Metrics: []core.Metric{{Label: "Words", Value: "412"}, {Label: "", Value: "x"}, {Label: "A", Value: "1"}, {Label: "B", Value: "2"}, {Label: "C", Value: "3"}, {Label: "D", Value: "4"}}}
	a := DayActivity(day)
	if a.Count != 5 || a.Done != 1 || a.Minutes != 30 || a.Qty != 5.5 {
		t.Fatalf("totals: %+v", a)
	}
	// money is summed per currency; the largest total is the day's
	if a.Cur != "JPY" || a.Amount != 1200 {
		t.Errorf("amount: %v %s", a.Amount, a.Cur)
	}
	want := ActivityItem{Time: "07:40", End: "08:10", Title: "Bread recipe", Sub: "Recipes", Group: "written", Minutes: 30, Done: true, Ref: "4", Amount: 4.5, Currency: "EUR", Qty: 2, URL: "https://notes.example/note/4", Image: "https://notes.example/i/4.jpg", Note: "For Sunday"}
	if fmt.Sprintf("%+v", a.Items[0]) != fmt.Sprintf("%+v", want) {
		t.Errorf("item:\n got %+v\nwant %+v", a.Items[0], want)
	}
	// by time, the untimed ones last in the order given
	var order []string
	for _, it := range a.Items {
		order = append(order, it.Ref)
	}
	if fmt.Sprint(order) != "[4 7 8 9 6]" {
		t.Errorf("order: %v", order)
	}
	if it := a.Items[4]; it.Time != "" || it.End != "" {
		t.Errorf("a bad time was kept: %+v", it)
	}
	if it := a.Items[2]; it.Amount != 1.5 || it.Currency != "KWD" {
		t.Errorf("three decimals: %+v", it)
	}
	if len(a.Top) != 2 || a.Top[0] != (Count{Name: "written", Count: 2}) {
		t.Errorf("groups: %+v", a.Top)
	}
	if len(a.Metrics) != 4 || a.Metrics[0] != (ActivityMetric{Label: "Words", Value: "412"}) {
		t.Errorf("metrics: %+v", a.Metrics)
	}

	// nothing that day, in the three ways an app can say it
	for _, d := range []*core.Day{nil, {}, {Items: []core.DayItem{{Title: ""}}}} {
		if e := DayActivity(d); e.Count != 0 || e.Items == nil || len(e.Items) != 0 || len(e.Metrics) != 0 {
			t.Errorf("empty day: %+v", e)
		}
	}
	// never more than the contract's limit
	big := &core.Day{}
	for i := 0; i < 300; i++ {
		big.Items = append(big.Items, core.DayItem{Title: fmt.Sprint("n", i)})
	}
	if n := DayActivity(big).Count; n != 200 {
		t.Errorf("%d items kept", n)
	}
}

func TestCapabilityFetch(t *testing.T) {
	ctx := context.Background()
	day := &core.Day{Summary: " 3 notes\n written ", Items: []core.DayItem{{Title: "A", Kind: "written", Minutes: 20}, {Title: "B", Kind: "written", Minutes: 25, Amount: 990, Currency: "EUR", Qty: 4}}}
	var gotDate string
	var gotLoc *time.Location
	c := &Capability{Source: "notes", App: "Pocket Notes", Meta: ActivityMeta{Label: "Notes", One: "note", Many: "notes", Unit: "count"},
		Day: func(_ context.Context, date string, loc *time.Location) (*core.Day, error) {
			gotDate, gotLoc = date, loc
			return day, nil
		}}
	if c.Name() != "notes" {
		t.Errorf("name: %s", c.Name())
	}
	res, err := c.Fetch(ctx, "2026-11-27", time.UTC)
	if err != nil || gotDate != "2026-11-27" || gotLoc != time.UTC {
		t.Fatalf("fetch: %v %s %v", err, gotDate, gotLoc)
	}
	// the app's own line, on one line
	if res.Summary != "3 notes written" || res.Headline != 2 {
		t.Errorf("result: %+v", res)
	}
	if a, ok := res.Payload.(Activity); !ok || a.Count != 2 {
		t.Errorf("payload: %#v", res.Payload)
	}
	// the number follows what the source measures
	for unit, want := range map[string]float64{"": 2, "count": 2, "min": 45, "amount": 9.9, "qty": 4} {
		c.Meta.Unit = unit
		if res, _ := c.Fetch(ctx, "2026-11-27", nil); res.Headline != want {
			t.Errorf("headline for %q: %v, want %v", unit, res.Headline, want)
		}
	}
	// no line from the app: the one every activity source gets
	day.Summary = ""
	if res, _ := c.Fetch(ctx, "2026-11-27", nil); res.Summary != "2 notes · 45 min · written" {
		t.Errorf("fallback summary: %q", res.Summary)
	}
	c.Meta.One, c.Meta.Many = "", ""
	if res, _ := c.Fetch(ctx, "2026-11-27", nil); res.Summary != "2 items · 45 min · written" {
		t.Errorf("fallback summary without words: %q", res.Summary)
	}
	// nothing that day: no line, no number, an empty list
	day.Items = nil
	res, err = c.Fetch(ctx, "2026-11-27", nil)
	if err != nil || res.Summary != "" || res.Headline != 0 || res.Payload.(Activity).Count != 0 {
		t.Errorf("empty day: %+v %v", res, err)
	}
	// a failure is the caller's to judge
	boom := errors.New("boom")
	c.Day = func(context.Context, string, *time.Location) (*core.Day, error) { return nil, boom }
	if _, err := c.Fetch(ctx, "2026-11-27", nil); !errors.Is(err, boom) {
		t.Errorf("error: %v", err)
	}

	// Test reports the link, never the app
	if msg, err := c.Test(ctx); err != nil || msg != "Pocket Notes" {
		t.Errorf("test without a check: %q %v", msg, err)
	}
	c.Check = func(context.Context) (string, error) { return "", errors.New("the link is off") }
	if _, err := c.Test(ctx); err == nil || err.Error() != "the link is off" {
		t.Errorf("test: %v", err)
	}
}

func TestMoreActivities(t *testing.T) {
	var none More
	if none.Is("wardrobe") || !none.Is("podcasts") || len(none.Names()) != 0 {
		t.Error("a nil More knows only the built-in sources")
	}
	m := More{"wardrobe": {Label: "Wardrobe"}, "atlas": {Label: "Atlas"}, "podcasts": {Label: "Not this one"}}
	if a, ok := m.Meta("wardrobe"); !ok || a.Label != "Wardrobe" {
		t.Errorf("brought: %+v %v", a, ok)
	}
	if a, _ := m.Meta("podcasts"); a.Label != "Podcasts" {
		t.Errorf("a built-in source was replaced: %+v", a)
	}
	if fmt.Sprint(m.Names()) != "[atlas wardrobe]" {
		t.Errorf("names: %v", m.Names())
	}
}
