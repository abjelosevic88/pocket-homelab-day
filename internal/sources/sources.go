// Package sources pulls one day's worth of knowledge out of each connected
// app: where you were (Dawarich), the photos (Immich), what played (Maloja),
// what you watched (Jellystat), what you spent (Pocket Money, through its
// Firefly-compatible API), the check-in
// (Pocket Habits), the books (Pocket Bookshelf), the trip (Pocket Travel),
// what you cooked (Pocket Kitchen), the fill-ups (Pocket Garage) and the
// weather (Open-Meteo). Every fetcher returns a payload, one headline number
// and a one-line summary; the store freezes them per day.
package sources

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Result is what a fetcher returns for one date.
type Result struct {
	Payload  any
	Headline float64
	Summary  string
}

// Fetcher knows one source.
type Fetcher interface {
	// Name is the source id (dawarich, immich, …).
	Name() string
	// Fetch returns the day's facts. loc is the local timezone the day is measured in.
	Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error)
	// Test checks the connection and returns a short description.
	Test(ctx context.Context) (string, error)
}

// DayBounds returns the local start (inclusive) and end (exclusive) of a date.
func DayBounds(date string, loc *time.Location) (time.Time, time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return t, t.AddDate(0, 0, 1), nil
}

// Count is a name with how often it occurred.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// TopCounts sorts a map into the n most frequent names.
func TopCounts(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// Round1 rounds to one decimal.
func Round1(f float64) float64 { return math.Round(f*10) / 10 }

// Plural returns "1 photo" / "3 photos".
func Plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// JoinNonEmpty joins the non-empty parts with sep.
func JoinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// LocalClock formats an instant as HH:MM in loc.
func LocalClock(ts time.Time, loc *time.Location) string {
	if ts.IsZero() {
		return ""
	}
	return ts.In(loc).Format("15:04")
}

// LocalDate preserves the calendar date alongside a location's display clock.
func LocalDate(ts time.Time, loc *time.Location) string {
	if ts.IsZero() {
		return ""
	}
	return ts.In(loc).Format("2006-01-02")
}

// ParseAny parses the timestamp shapes the sources use.
func ParseAny(s string) (time.Time, bool) {
	for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// HoursMinutes renders minutes as "45 min" or "2h05".
func HoursMinutes(m int) string {
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%dh%02d", m/60, m%60)
}
