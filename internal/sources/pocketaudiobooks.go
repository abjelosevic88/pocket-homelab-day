package sources

import (
	"context"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// PocketAudiobooks reads a day of audiobook listening from Pocket Audiobooks
// (its day view, listening:read). It fills the audiobooks source
// ("audiobookshelf"), so the days, the timeline, the year page and the briefs
// read it exactly like Audiobookshelf's own sessions. Pocket Audiobooks has
// every Audiobookshelf app's sessions of the person, books only.
type PocketAudiobooks struct{ Conn pocket.Conn }

func NewPocketAudiobooks(c pocket.Conn) *PocketAudiobooks { return &PocketAudiobooks{Conn: c} }
func (p *PocketAudiobooks) Name() string                  { return "audiobookshelf" }

type audiobooksDay struct {
	Seconds int `json:"seconds"`
	Books   []struct {
		ItemID   string `json:"item_id"`
		Title    string `json:"title"`
		Author   string `json:"author"`
		Seconds  int    `json:"seconds"`
		Duration int    `json:"duration"`
		To       int    `json:"to"`
		First    string `json:"first"`
		Finished bool   `json:"finished"`
	} `json:"books"`
}

func (p *PocketAudiobooks) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var out audiobooksDay
	if err := c.Get(ctx, "/api/v1/day/"+time.Now().Format("2006-01-02"), nil, &out); err != nil {
		return "", err
	}
	return "Pocket Audiobooks answers: " + Plural(out.Seconds/60, "minute", "minutes") + " today", nil
}

func (p *PocketAudiobooks) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var out audiobooksDay
	if err := c.Get(ctx, "/api/v1/day/"+date, nil, &out); err != nil {
		return nil, err
	}
	act := Activity{}
	for _, b := range out.Books {
		if b.Seconds < 60 {
			continue // scrubbing, not listening (as with Audiobookshelf's own sessions)
		}
		it := ActivityItem{Title: b.Title, Sub: b.Author, Ref: b.ItemID, Minutes: Round1(float64(b.Seconds) / 60),
			Position: float64(b.To), Length: float64(b.Duration), Done: b.Finished}
		if t, err := time.Parse(time.RFC3339Nano, b.First); err == nil {
			it.Time = LocalClock(t, loc)
		}
		act.Items = append(act.Items, it)
	}
	act.Finish()
	return &Result{Payload: act, Headline: act.Minutes, Summary: ActivitySummary(act, "title", "titles")}, nil
}
