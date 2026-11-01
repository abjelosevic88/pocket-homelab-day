package sources

import (
	"context"
	"net/url"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// PocketWatch reads a day of watching from Pocket Watch (its history,
// watching:read): what the person watched in Jellyfin, through Watch's own
// player or any other Jellyfin app. It fills the watched source
// ("jellystat"), so the days, the timeline, the year page and the briefs
// read it exactly like Jellystat's.
type PocketWatch struct{ Conn pocket.Conn }

func NewPocketWatch(c pocket.Conn) *PocketWatch { return &PocketWatch{Conn: c} }
func (p *PocketWatch) Name() string             { return "jellystat" }

type watchHistory struct {
	Total int `json:"total"`
	Items []struct {
		ItemID    string `json:"item_id"`
		Kind      string `json:"kind"` // movie | episode
		Title     string `json:"title"`
		Series    string `json:"series"`
		WatchedAt string `json:"watched_at"`
		Seconds   int    `json:"seconds"`
		Device    string `json:"device"`
	} `json:"items"`
}

func (p *PocketWatch) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var out watchHistory
	if err := c.Get(ctx, "/api/v1/history", url.Values{"limit": {"1"}}, &out); err != nil {
		return "", err
	}
	return "Pocket Watch answers: " + Plural(out.Total, "title watched", "titles watched"), nil
}

// Fetch implements Fetcher. Watch files a watch under the day in its own
// time zone, so a day either side is asked for and the times decide.
func (p *PocketWatch) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{"from": {from.AddDate(0, 0, -1).Format("2006-01-02")}, "to": {to.AddDate(0, 0, 1).Format("2006-01-02")}, "limit": {"500"}}
	var h watchHistory
	if err := c.Get(ctx, "/api/v1/history", q, &h); err != nil {
		return nil, err
	}
	byItem := map[string]*Play{}
	var order []string
	for _, r := range h.Items { // newest first
		at, ok := ParseAny(r.WatchedAt)
		if !ok || at.Before(from) || !at.Before(to) {
			continue
		}
		if r.Seconds < 120 { // autoplay, a quick look or a bulk "mark watched" is not watching
			continue
		}
		key := r.ItemID
		if key == "" {
			key = r.Title
		}
		pl, exists := byItem[key]
		if !exists {
			pl = &Play{Title: r.Title, Kind: "movie", Time: LocalClock(at, loc), Client: r.Device, ItemID: r.ItemID}
			if r.Kind == "episode" || r.Series != "" {
				pl.Series = r.Series
				pl.Kind = "episode"
			}
			byItem[key] = pl
			order = append(order, key)
		} else if t := LocalClock(at, loc); t < pl.Time {
			pl.Time = t
		}
		pl.Plays++
		pl.Minutes += r.Seconds / 60
	}
	return watchedResult(byItem, order, "watch"), nil
}
