package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// PinePods reads the podcast listening history. PinePods keeps one row per
// episode with the LAST time it was played and the position reached, so the
// fetcher returns the episodes last played on the day with their position;
// the server turns positions into minutes per day (see adjustListening).
type PinePods struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	userID  int
}

// NewPinePods creates a fetcher.
func NewPinePods(baseURL, key string) *PinePods {
	return &PinePods{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *PinePods) Name() string { return "pinepods" }

func (c *PinePods) get(ctx context.Context, path string, v any) error {
	if c.BaseURL == "" || c.APIKey == "" {
		return errors.New("pinepods is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errors.New("pinepods rejected the API key")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("pinepods HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

func (c *PinePods) user(ctx context.Context) (int, error) {
	if c.userID > 0 {
		return c.userID, nil
	}
	var u struct {
		ID int `json:"retrieved_id"`
	}
	if err := c.get(ctx, "/api/data/get_user", &u); err != nil {
		return 0, err
	}
	if u.ID == 0 {
		return 0, errors.New("pinepods: could not tell which user the key belongs to")
	}
	c.userID = u.ID
	return u.ID, nil
}

// Test implements Fetcher.
func (c *PinePods) Test(ctx context.Context) (string, error) {
	id, err := c.user(ctx)
	if err != nil {
		return "", err
	}
	var st struct {
		Played int `json:"PodcastsPlayed"`
		Added  int `json:"PodcastsAdded"`
		Time   int `json:"TimeListened"`
	}
	_ = c.get(ctx, fmt.Sprintf("/api/data/get_stats?user_id=%d", id), &st)
	return fmt.Sprintf("PinePods reachable: %d podcasts, %d episodes played", st.Added, st.Played), nil
}

type pinepodsRow struct {
	EpisodeID int     `json:"episodeid"`
	Title     string  `json:"episodetitle"`
	Podcast   string  `json:"podcastname"`
	Artwork   string  `json:"episodeartwork"`
	URL       string  `json:"episodeurl"`
	Length    float64 `json:"episodeduration"`
	Position  float64 `json:"listenduration"`
	Completed bool    `json:"completed"`
	ListenAt  string  `json:"listendate"` // server-local wall time, no zone
}

// Fetch implements Fetcher.
func (c *PinePods) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	id, err := c.user(ctx)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []pinepodsRow `json:"data"`
	}
	if err := c.get(ctx, "/api/data/user_history/"+strconv.Itoa(id), &out); err != nil {
		return nil, err
	}
	a := Activity{}
	for _, r := range out.Data {
		if len(r.ListenAt) < 10 || r.ListenAt[:10] != date || r.Position <= 0 {
			continue
		}
		it := ActivityItem{Title: r.Title, Sub: r.Podcast, Ref: strconv.Itoa(r.EpisodeID), Position: r.Position, Length: r.Length, Done: r.Completed, Image: r.Artwork, Minutes: Round1(r.Position / 60)}
		if len(r.ListenAt) >= 16 {
			it.Time = r.ListenAt[11:16]
		}
		a.Items = append(a.Items, it)
	}
	a.Finish()
	return &Result{Payload: a, Headline: a.Minutes, Summary: ActivitySummary(a, "episode", "episodes")}, nil
}

// ActivitySummary is "3 episodes · 1h20 · mostly X".
func ActivitySummary(a Activity, one, many string) string {
	if a.Count == 0 {
		return ""
	}
	s := Plural(a.Count, one, many)
	if a.Minutes >= 1 {
		s += " · " + HoursMinutes(int(a.Minutes+0.5))
	}
	if len(a.Top) > 0 && a.Top[0].Name != "" {
		s += " · " + a.Top[0].Name
	}
	return s
}
