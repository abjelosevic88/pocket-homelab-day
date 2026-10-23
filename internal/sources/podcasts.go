package sources

import (
	"context"
	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
	"net/url"
	"strconv"
	"time"
)

// Podcasts uses per-day listening totals, never a resume position or episode length.
type Podcasts struct{ Conn pocket.Conn }

func NewPodcasts(c pocket.Conn) *Podcasts { return &Podcasts{Conn: c} }
func (p *Podcasts) Name() string          { return "podcasts" }
func (p *Podcasts) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Listens []any `json:"listens"`
	}
	err = c.Get(ctx, "/api/v1/listens", url.Values{"date": {time.Now().Format("2006-01-02")}}, &out)
	return "Pocket Podcasts listening history reachable", err
}
func (p *Podcasts) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	if _, _, err := DayBounds(date, loc); err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Listens []struct {
			EpisodeID int64  `json:"episode_id"`
			Title     string `json:"title"`
			Podcast   string `json:"podcast"`
			Seconds   int    `json:"seconds"`
			Duration  int    `json:"duration"`
			FirstAt   string `json:"first_at"`
			LastAt    string `json:"last_at"`
		} `json:"listens"`
	}
	if err = c.Get(ctx, "/api/v1/listens", url.Values{"date": {date}}, &out); err != nil {
		return nil, err
	}
	a := Activity{}
	for _, v := range out.Listens {
		if v.Seconds <= 0 {
			continue
		}
		it := ActivityItem{Title: v.Title, Sub: v.Podcast, Ref: strconv.FormatInt(v.EpisodeID, 10), Minutes: float64(v.Seconds) / 60, Length: float64(v.Duration)}
		if t, ok := ParseAny(v.FirstAt); ok {
			it.Time = LocalClock(t, loc)
		}
		if t, ok := ParseAny(v.LastAt); ok {
			it.End = LocalClock(t, loc)
		}
		a.Items = append(a.Items, it)
	}
	a.Finish()
	return &Result{Payload: a, Headline: a.Minutes, Summary: ActivitySummary(a, "episode", "episodes")}, nil
}
