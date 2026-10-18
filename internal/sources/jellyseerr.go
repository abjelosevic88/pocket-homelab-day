package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Jellyseerr reads the film and show requests made on the day, and the
// requested titles that became available that day.
type Jellyseerr struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewJellyseerr creates a fetcher.
func NewJellyseerr(baseURL, key string) *Jellyseerr {
	return &Jellyseerr{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Jellyseerr) Name() string { return "jellyseerr" }

var seerrTitles sync.Map // "movie/123" → title; requests carry no titles

func (c *Jellyseerr) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" || c.APIKey == "" {
		return errors.New("jellyseerr is not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errors.New("jellyseerr rejected the API key")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jellyseerr HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Test implements Fetcher.
func (c *Jellyseerr) Test(ctx context.Context) (string, error) {
	var out struct {
		PageInfo struct {
			Results int `json:"results"`
		} `json:"pageInfo"`
	}
	if err := c.get(ctx, "/api/v1/request", url.Values{"take": {"1"}}, &out); err != nil {
		return "", err
	}
	return fmt.Sprintf("Jellyseerr reachable, %d requests", out.PageInfo.Results), nil
}

func (c *Jellyseerr) title(ctx context.Context, kind string, tmdb int) string {
	key := fmt.Sprintf("%s/%d", kind, tmdb)
	if v, ok := seerrTitles.Load(key); ok {
		return v.(string)
	}
	var out struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	}
	if err := c.get(ctx, "/api/v1/"+kind+"/"+fmt.Sprint(tmdb), nil, &out); err != nil {
		return ""
	}
	t := out.Title
	if t == "" {
		t = out.Name
	}
	if t != "" {
		seerrTitles.Store(key, t)
	}
	return t
}

// Fetch implements Fetcher.
func (c *Jellyseerr) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	act := Activity{}
	for skip := 0; skip < 5000; skip += 100 {
		var out struct {
			PageInfo struct {
				Pages int `json:"pages"`
			} `json:"pageInfo"`
			Results []struct {
				Type      string `json:"type"`
				Status    int    `json:"status"`
				CreatedAt string `json:"createdAt"`
				By        struct {
					Name string `json:"displayName"`
				} `json:"requestedBy"`
				Media struct {
					TmdbID  int    `json:"tmdbId"`
					Status  int    `json:"status"`
					AddedAt string `json:"mediaAddedAt"`
				} `json:"media"`
			} `json:"results"`
		}
		if err := c.get(ctx, "/api/v1/request", url.Values{"take": {"100"}, "skip": {fmt.Sprint(skip)}, "sort": {"added"}, "filter": {"all"}}, &out); err != nil {
			return nil, err
		}
		oldest := "9999"
		for _, r := range out.Results {
			created, ok := ParseAny(r.CreatedAt)
			if !ok {
				continue
			}
			cd := created.In(loc).Format("2006-01-02")
			if cd < oldest {
				oldest = cd
			}
			kind := "movie"
			label := "film"
			if r.Type == "tv" {
				kind, label = "tv", "series"
			}
			title := ""
			if cd == date {
				title = c.title(ctx, kind, r.Media.TmdbID)
				it := ActivityItem{Time: LocalClock(created, loc), Title: title, Sub: "requested " + label, Group: label, Ref: fmt.Sprintf("req/%s/%d", kind, r.Media.TmdbID)}
				if r.Status == 3 {
					it.Sub += " · declined"
				}
				act.Items = append(act.Items, it)
			}
			if added, ok := ParseAny(r.Media.AddedAt); ok && added.In(loc).Format("2006-01-02") == date {
				if title == "" {
					title = c.title(ctx, kind, r.Media.TmdbID)
				}
				act.Items = append(act.Items, ActivityItem{Time: LocalClock(added, loc), Title: title, Sub: "now available", Group: label, Done: true, Ref: fmt.Sprintf("avail/%s/%d", kind, r.Media.TmdbID)})
			}
		}
		// sorted by added, newest first: stop once a page is entirely older than the day
		// (availability can come later than the request, so look one page further)
		if len(out.Results) < 100 || oldest < AddDaysISO(date, -60) {
			break
		}
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		req, av := 0, 0
		for _, it := range act.Items {
			if it.Done {
				av++
			} else {
				req++
			}
		}
		parts := []string{}
		if req > 0 {
			parts = append(parts, Plural(req, "request", "requests"))
		}
		if av > 0 {
			parts = append(parts, fmt.Sprintf("%d now available", av))
		}
		sum = strings.Join(parts, " · ")
		if act.Items[0].Title != "" {
			sum += " · " + act.Items[0].Title
		}
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}

// AddDaysISO shifts a YYYY-MM-DD date.
func AddDaysISO(d string, n int) string {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		return d
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}
