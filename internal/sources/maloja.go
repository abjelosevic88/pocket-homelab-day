package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Maloja reads the day's scrobbles. Its read API needs no key.
type Maloja struct {
	BaseURL string
	HTTP    *http.Client
}

// NewMaloja creates a client.
func NewMaloja(baseURL string) *Maloja {
	return &Maloja{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Maloja) Name() string { return "maloja" }

func (c *Maloja) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" {
		return errors.New("maloja is not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("maloja HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Test implements Fetcher.
func (c *Maloja) Test(ctx context.Context) (string, error) {
	var out struct {
		Status string `json:"status"`
		Amount int    `json:"amount"`
	}
	if err := c.get(ctx, "/apis/mlj_1/numscrobbles", url.Values{"in": {"alltime"}}, &out); err != nil {
		return "", err
	}
	return fmt.Sprintf("Maloja reachable, %d scrobbles all time", out.Amount), nil
}

// Listen is one scrobble.
type Listen struct {
	Time   string `json:"time"` // HH:MM
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Album  string `json:"album,omitempty"`
}

// Music is the day's listening.
type Music struct {
	Listens int      `json:"listens"`
	Minutes int      `json:"minutes"`
	Artists []Count  `json:"artists"`
	Tracks  []Count  `json:"tracks"`
	Albums  []Count  `json:"albums"`
	First   string   `json:"first"`
	Last    string   `json:"last"`
	Items   []Listen `json:"items"`
}

// Fetch implements Fetcher.
func (c *Maloja) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	m := Music{Items: []Listen{}, Artists: []Count{}, Tracks: []Count{}, Albums: []Count{}}
	artists, tracks, albums := map[string]int{}, map[string]int{}, map[string]int{}
	// Maloja's from/to are whole days in its own timezone; ask a day wider and filter by the instant.
	day := strings.ReplaceAll(date, "-", "/")
	prev := strings.ReplaceAll(from.AddDate(0, 0, -1).Format("2006-01-02"), "-", "/")
	next := strings.ReplaceAll(to.Format("2006-01-02"), "-", "/")
	for page := 0; page < 40; page++ {
		var out struct {
			Status string `json:"status"`
			List   []struct {
				Time  int64 `json:"time"`
				Track struct {
					Artists []string `json:"artists"`
					Title   string   `json:"title"`
					Album   *struct {
						Title string `json:"albumtitle"`
					} `json:"album"`
					Length *float64 `json:"length"`
				} `json:"track"`
				Duration *float64 `json:"duration"`
			} `json:"list"`
			Pagination struct {
				NextPage *string `json:"next_page"`
			} `json:"pagination"`
		}
		q := url.Values{"from": {prev}, "to": {next}, "perpage": {"500"}, "page": {fmt.Sprint(page)}}
		if err := c.get(ctx, "/apis/mlj_1/scrobbles", q, &out); err != nil {
			return nil, err
		}
		if out.Status != "ok" && out.Status != "" {
			return nil, fmt.Errorf("maloja: %s", out.Status)
		}
		for _, s := range out.List {
			ts := time.Unix(s.Time, 0)
			if ts.Before(from) || !ts.Before(to) {
				continue
			}
			l := Listen{Time: LocalClock(ts, loc), Artist: strings.Join(s.Track.Artists, ", "), Title: s.Track.Title}
			if s.Track.Album != nil {
				l.Album = s.Track.Album.Title
			}
			m.Items = append(m.Items, l)
			for _, a := range s.Track.Artists {
				artists[a]++
			}
			tracks[l.Artist+" – "+l.Title]++
			if l.Album != "" {
				albums[l.Album]++
			}
			switch {
			case s.Duration != nil && *s.Duration > 0:
				m.Minutes += int(*s.Duration / 60)
			case s.Track.Length != nil && *s.Track.Length > 0:
				m.Minutes += int(*s.Track.Length / 60)
			default:
				m.Minutes += 4 // the usual song
			}
		}
		if out.Pagination.NextPage == nil || len(out.List) == 0 {
			break
		}
	}
	_ = day
	// Maloja returns newest first; put the day in order.
	for i, j := 0, len(m.Items)-1; i < j; i, j = i+1, j-1 {
		m.Items[i], m.Items[j] = m.Items[j], m.Items[i]
	}
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
