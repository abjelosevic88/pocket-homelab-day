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

// Jellystat reads Jellyfin's playback history, newest first, and keeps the
// plays that fall on the day. Optionally filtered to one Jellyfin user.
type Jellystat struct {
	BaseURL string
	APIKey  string
	User    string
	HTTP    *http.Client
}

// NewJellystat creates a client.
func NewJellystat(baseURL, key, user string) *Jellystat {
	return &Jellystat{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, User: strings.TrimSpace(user), HTTP: &http.Client{Timeout: 45 * time.Second}}
}

// Name implements Fetcher.
func (c *Jellystat) Name() string { return "jellystat" }

func (c *Jellystat) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" || c.APIKey == "" {
		return errors.New("jellystat is not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-token", c.APIKey)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errors.New("jellystat rejected the API key")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jellystat HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

type jsPage struct {
	CurrentPage int `json:"current_page"`
	Pages       int `json:"pages"`
	Results     []struct {
		ID         string  `json:"Id"`
		UserName   *string `json:"UserName"`
		Client     string  `json:"Client"`
		Device     string  `json:"DeviceName"`
		ItemID     string  `json:"NowPlayingItemId"`
		ItemName   string  `json:"NowPlayingItemName"`
		SeriesName *string `json:"SeriesName"`
		EpisodeID  *string `json:"EpisodeId"`
		Duration   float64 `json:"PlaybackDuration"`
		At         string  `json:"ActivityDateInserted"`
	} `json:"results"`
}

// Test implements Fetcher.
func (c *Jellystat) Test(ctx context.Context) (string, error) {
	var p jsPage
	if err := c.get(ctx, "/api/getHistory", url.Values{"size": {"1"}, "page": {"1"}}, &p); err != nil {
		return "", err
	}
	return fmt.Sprintf("Jellystat reachable, %d pages of history", p.Pages), nil
}

// Play is one thing watched (plays of the same item merged).
type Play struct {
	Title   string `json:"title"`
	Series  string `json:"series,omitempty"`
	Kind    string `json:"kind"` // episode | movie
	Minutes int    `json:"minutes"`
	Time    string `json:"time"` // HH:MM of the first play
	Client  string `json:"client,omitempty"`
	User    string `json:"user,omitempty"`
	ItemID  string `json:"item_id"`
	Plays   int    `json:"plays"`
	Session string `json:"-"`
}

// Watched is the day's screen time.
type Watched struct {
	Count    int    `json:"count"`
	Episodes int    `json:"episodes"`
	Movies   int    `json:"movies"`
	Minutes  int    `json:"minutes"`
	Items    []Play `json:"items"`
}

// Fetch implements Fetcher.
func (c *Jellystat) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	w := Watched{Items: []Play{}}
	byItem := map[string]*Play{}
	var order []string
	done := false
	for page := 1; page <= 400 && !done; page++ {
		var p jsPage
		if err := c.get(ctx, "/api/getHistory", url.Values{"size": {"100"}, "page": {fmt.Sprint(page)}}, &p); err != nil {
			return nil, err
		}
		if len(p.Results) == 0 {
			break
		}
		for _, r := range p.Results {
			at, ok := ParseAny(r.At)
			if !ok {
				continue
			}
			if at.Before(from) { // newest first: once we pass the day's start we are done
				done = true
				break
			}
			if !at.Before(to) {
				continue
			}
			if c.User != "" && (r.UserName == nil || !strings.EqualFold(*r.UserName, c.User)) {
				continue
			}
			if r.Duration < 120 { // autoplay or a quick look is not watching
				continue
			}
			key := r.ItemID
			if key == "" {
				key = r.ItemName
			}
			pl, exists := byItem[key]
			if !exists {
				pl = &Play{Title: r.ItemName, Kind: "movie", Time: LocalClock(at, loc), Client: r.Client, ItemID: r.ItemID}
				if r.SeriesName != nil && *r.SeriesName != "" {
					pl.Series = *r.SeriesName
					pl.Kind = "episode"
				}
				if r.UserName != nil {
					pl.User = *r.UserName
				}
				byItem[key] = pl
				order = append(order, key)
			} else if t := LocalClock(at, loc); t < pl.Time {
				pl.Time = t
			}
			pl.Plays++
			pl.Minutes += int(r.Duration / 60)
		}
		if page >= p.Pages {
			break
		}
	}
	// oldest first
	for i := len(order) - 1; i >= 0; i-- {
		pl := byItem[order[i]]
		w.Items = append(w.Items, *pl)
		w.Minutes += pl.Minutes
		if pl.Kind == "episode" {
			w.Episodes++
		} else {
			w.Movies++
		}
	}
	w.Count = len(w.Items)
	sum := ""
	if w.Count > 0 {
		parts := []string{}
		if w.Episodes > 0 {
			parts = append(parts, Plural(w.Episodes, "episode", "episodes"))
		}
		if w.Movies > 0 {
			parts = append(parts, Plural(w.Movies, "film", "films"))
		}
		sum = strings.Join(parts, ", ")
		if w.Minutes > 0 {
			sum += " · " + HoursMinutes(w.Minutes)
		}
	}
	return &Result{Payload: w, Headline: float64(w.Minutes), Summary: sum}, nil
}
