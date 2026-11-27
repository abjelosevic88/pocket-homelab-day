package sources

import (
	"context"
	"math"
	"strings"
	"time"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// ---------------- Any app that says what happened on a day ----------------

// Capability reads the day from an app Day has no code for. An app that
// offers the "day" capability (Pocket Core, docs/capabilities.md) answers
// GET /api/v1/pocket/day/{date} in one shape, and Pocket Core tells Day who
// offers it: so the app becomes a source on its own, with the label, the
// words for its things and what its number measures as the app said them.
// The day is kept as an Activity, the payload every activity source shares.
type Capability struct {
	// Source is the source's id: the app's slug in Pocket Core.
	Source string
	// App is the name people know the app by.
	App string
	// Meta is how the app's things are named and counted.
	Meta ActivityMeta
	// Day asks the app for one day, as the person (Pocket Core's DayIn). A
	// link that is off comes back as pocket.ErrNotConnected.
	Day func(ctx context.Context, date string, loc *time.Location) (*core.Day, error)
	// Check says how the link to the app stands, without asking the app.
	Check func(ctx context.Context) (string, error)
}

// Name implements Fetcher.
func (c *Capability) Name() string { return c.Source }

// Test implements Fetcher. There is no token to try the app with outside a
// fetch, so it reports how the link stands in Pocket Core.
func (c *Capability) Test(ctx context.Context) (string, error) {
	if c.Check == nil {
		return c.App, nil
	}
	return c.Check(ctx)
}

// Fetch implements Fetcher.
func (c *Capability) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	day, err := c.Day(ctx, date, loc)
	if err != nil {
		return nil, err
	}
	act := DayActivity(day)
	sum := ""
	if day != nil {
		sum = oneLine(day.Summary, 300)
	}
	if sum == "" {
		one, many := c.Meta.One, c.Meta.Many
		if one == "" || many == "" {
			one, many = "item", "items"
		}
		sum = ActivitySummary(act, one, many)
	}
	return &Result{Payload: act, Headline: ActivityHeadline(act, c.Meta.Unit), Summary: sum}, nil
}

// dayMaxItems is the most items a day carries (the contract's own limit).
const dayMaxItems = 200

// DayActivity turns an app's day (the capability's shape) into an Activity:
// kind becomes the group, money goes from minor units to the amount itself,
// the id becomes the reference pins use. Items without a title are left out,
// a time that is not HH:MM counts as "sometime that day". A day with nothing
// gives an empty Activity, which shows as nothing.
func DayActivity(day *core.Day) Activity {
	var act Activity
	if day != nil {
		for _, it := range day.Items {
			title := oneLine(it.Title, 300)
			if title == "" {
				continue
			}
			x := ActivityItem{
				Time: clockOrEmpty(it.Time), End: clockOrEmpty(it.End), Title: title, Sub: oneLine(it.Sub, 300), Group: strings.TrimSpace(it.Kind),
				Minutes: it.Minutes, Done: it.Done, Ref: strings.TrimSpace(it.ID), Qty: it.Qty,
				URL: strings.TrimSpace(it.URL), Image: strings.TrimSpace(it.Image), Note: strings.TrimSpace(it.Note),
			}
			if x.Time == "" {
				x.End = ""
			}
			if it.Amount != 0 {
				x.Currency = strings.ToUpper(strings.TrimSpace(it.Currency))
				x.Amount = float64(it.Amount) / math.Pow10(minorUnits(x.Currency))
			}
			act.Items = append(act.Items, x)
			if len(act.Items) == dayMaxItems {
				break
			}
		}
		for _, m := range day.Metrics {
			label, value := oneLine(m.Label, 40), oneLine(m.Value, 40)
			if label == "" || value == "" {
				continue
			}
			act.Metrics = append(act.Metrics, ActivityMetric{Label: label, Value: value})
			if len(act.Metrics) == 4 {
				break
			}
		}
	}
	act.Finish()
	return act
}

// clockOrEmpty keeps a local HH:MM and drops anything else.
func clockOrEmpty(t string) string {
	t = strings.TrimSpace(t)
	if len(t) != 5 || t[2] != ':' {
		return ""
	}
	h, m := t[:2], t[3:]
	if h < "00" || h > "23" || m < "00" || m > "59" {
		return ""
	}
	for _, c := range h + m {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return t
}

// minorUnits is how many decimals a currency's smallest unit has: 2 for most,
// none for the yen and its like, 3 for the dinars.
func minorUnits(cur string) int {
	switch cur {
	case "BIF", "CLP", "DJF", "GNF", "ISK", "JPY", "KMF", "KRW", "PYG", "RWF", "UGX", "UYI", "VND", "VUV", "XAF", "XOF", "XPF":
		return 0
	case "BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND":
		return 3
	}
	return 2
}

// oneLine folds whitespace and cuts to max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max-1])) + "…"
	}
	return s
}
