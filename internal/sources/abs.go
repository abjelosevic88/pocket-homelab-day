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

// Audiobookshelf reads listening sessions (audiobooks and ABS podcasts) for
// every user, or only the users named in Extra (comma separated). Sessions
// under a minute are scrubbing, not listening.
type Audiobookshelf struct {
	BaseURL string
	Token   string
	Users   []string
	HTTP    *http.Client
}

// NewAudiobookshelf creates a fetcher.
func NewAudiobookshelf(baseURL, token, users string) *Audiobookshelf {
	var us []string
	for _, u := range strings.Split(users, ",") {
		if u = strings.ToLower(strings.TrimSpace(u)); u != "" {
			us = append(us, u)
		}
	}
	return &Audiobookshelf{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Users: us, HTTP: &http.Client{Timeout: 40 * time.Second}}
}

// Name implements Fetcher.
func (c *Audiobookshelf) Name() string { return "audiobookshelf" }

func (c *Audiobookshelf) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" || c.Token == "" {
		return errors.New("audiobookshelf is not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errors.New("audiobookshelf rejected the token (it needs an admin key to see every user's sessions)")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("audiobookshelf HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Test implements Fetcher.
func (c *Audiobookshelf) Test(ctx context.Context) (string, error) {
	var out struct {
		Total int `json:"total"`
	}
	if err := c.get(ctx, "/api/sessions", url.Values{"itemsPerPage": {"1"}, "page": {"0"}}, &out); err != nil {
		return "", err
	}
	return fmt.Sprintf("Audiobookshelf reachable, %d listening sessions", out.Total), nil
}

// Fetch implements Fetcher.
func (c *Audiobookshelf) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	names := map[string]string{}
	if len(c.Users) > 0 {
		var us struct {
			Users []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"users"`
		}
		if err := c.get(ctx, "/api/users", nil, &us); err == nil {
			for _, u := range us.Users {
				names[u.ID] = strings.ToLower(u.Username)
			}
		}
	}
	keep := func(userID string) bool {
		if len(c.Users) == 0 {
			return true
		}
		for _, u := range c.Users {
			if names[userID] == u {
				return true
			}
		}
		return false
	}
	type agg struct {
		it    ActivityItem
		start int64
	}
	byItem := map[string]*agg{}
	var order []string
	for page := 0; page < 50; page++ {
		var out struct {
			NumPages int `json:"numPages"`
			Sessions []struct {
				UserID        string  `json:"userId"`
				LibraryItemID string  `json:"libraryItemId"`
				EpisodeID     *string `json:"episodeId"`
				MediaType     string  `json:"mediaType"`
				Title         string  `json:"displayTitle"`
				Author        string  `json:"displayAuthor"`
				Date          string  `json:"date"`
				TimeListening float64 `json:"timeListening"`
				CurrentTime   float64 `json:"currentTime"`
				Duration      float64 `json:"duration"`
				StartedAt     int64   `json:"startedAt"`
				Meta          struct {
					Title string `json:"title"`
				} `json:"mediaMetadata"`
			} `json:"sessions"`
		}
		if err := c.get(ctx, "/api/sessions", url.Values{"itemsPerPage": {"100"}, "page": {fmt.Sprint(page)}}, &out); err != nil {
			return nil, err
		}
		older := 0
		for _, s := range out.Sessions {
			if s.Date < date {
				older++
			}
			if s.Date != date || s.TimeListening < 60 || !keep(s.UserID) {
				continue
			}
			key := s.LibraryItemID
			if s.EpisodeID != nil {
				key += "/" + *s.EpisodeID
			}
			a := byItem[key]
			if a == nil {
				it := ActivityItem{Title: s.Title, Sub: s.Author, Ref: key, Length: s.Duration}
				if s.MediaType == "podcast" && s.Meta.Title != "" && s.Meta.Title != s.Title {
					it.Sub = s.Meta.Title // the show, not the episode author
				}
				a = &agg{it: it, start: s.StartedAt}
				byItem[key] = a
				order = append(order, key)
			}
			a.it.Minutes += s.TimeListening / 60
			if s.CurrentTime > a.it.Position {
				a.it.Position = s.CurrentTime
			}
			if s.StartedAt > 0 && (a.start == 0 || s.StartedAt < a.start) {
				a.start = s.StartedAt
			}
			if s.Duration > 0 && s.CurrentTime >= s.Duration-60 {
				a.it.Done = true
			}
		}
		// pages run newest first; once a whole page is older than the day we are done
		if page+1 >= out.NumPages || (len(out.Sessions) > 0 && older == len(out.Sessions)) {
			break
		}
	}
	act := Activity{}
	for _, k := range order {
		a := byItem[k]
		a.it.Minutes = Round1(a.it.Minutes)
		if a.start > 0 {
			a.it.Time = LocalClock(time.UnixMilli(a.start), loc)
		}
		act.Items = append(act.Items, a.it)
	}
	act.Finish()
	return &Result{Payload: act, Headline: act.Minutes, Summary: ActivitySummary(act, "title", "titles")}, nil
}
