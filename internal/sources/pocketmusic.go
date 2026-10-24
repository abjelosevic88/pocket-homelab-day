package sources

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// PocketMusic reads a day of listening from Pocket Music (its day view,
// listening:read). It fills the music source ("maloja"), so the days, the
// timeline, the year page and the briefs read it exactly like Maloja's.
type PocketMusic struct{ Conn pocket.Conn }

func NewPocketMusic(c pocket.Conn) *PocketMusic { return &PocketMusic{Conn: c} }
func (p *PocketMusic) Name() string             { return "maloja" }

func (p *PocketMusic) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Totals struct {
			Plays int `json:"plays"`
		} `json:"totals"`
	}
	if err := c.Get(ctx, "/api/v1/day/"+time.Now().Format("2006-01-02"), nil, &out); err != nil {
		return "", err
	}
	return "Pocket Music answers: " + Plural(out.Totals.Plays, "song", "songs") + " today", nil
}

func (p *PocketMusic) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, _, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	_, off := from.Zone()
	var out struct {
		Totals struct {
			Plays   int   `json:"plays"`
			Seconds int64 `json:"seconds"`
		} `json:"totals"`
		Listens []struct {
			Title    string `json:"title"`
			Artist   string `json:"artist"`
			Album    string `json:"album"`
			PlayedAt string `json:"played_at"`
			Duration int    `json:"duration"`
			MSPlayed int64  `json:"ms_played"`
		} `json:"listens"`
	}
	if err := c.Get(ctx, "/api/v1/day/"+date, url.Values{"tz": {strconv.Itoa(off / 60)}}, &out); err != nil {
		return nil, err
	}
	m := Music{Items: []Listen{}, Artists: []Count{}, Tracks: []Count{}, Albums: []Count{}}
	artists, tracks, albums := map[string]int{}, map[string]int{}, map[string]int{}
	secs := int64(0)
	for _, l := range out.Listens {
		it := Listen{Artist: l.Artist, Title: l.Title, Album: l.Album}
		if t, ok := ParseAny(l.PlayedAt); ok {
			it.Time = LocalClock(t, loc)
		}
		m.Items = append(m.Items, it)
		artists[l.Artist]++
		tracks[l.Artist+" – "+l.Title]++
		if l.Album != "" {
			albums[l.Album]++
		}
		switch {
		case l.MSPlayed > 0:
			secs += l.MSPlayed / 1000
		case l.Duration > 0:
			secs += int64(l.Duration)
		default:
			secs += 240 // the usual song
		}
	}
	m.Minutes = int(secs / 60)
	m.Listens = len(m.Items)
	if m.Listens > 0 {
		m.First, m.Last = m.Items[0].Time, m.Items[m.Listens-1].Time
	}
	m.Artists = TopCounts(artists, 5)
	m.Tracks = TopCounts(tracks, 5)
	m.Albums = TopCounts(albums, 3)
	sum := ""
	if m.Listens > 0 {
		sum = Plural(m.Listens, "listen", "listens")
		if len(m.Artists) > 0 {
			sum += " · mostly " + m.Artists[0].Name
		}
	}
	return &Result{Payload: m, Headline: float64(m.Listens), Summary: sum}, nil
}
