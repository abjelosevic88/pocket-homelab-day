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

// Paperless reads the documents added (scanned, mailed in, uploaded) on the day.
type Paperless struct {
	BaseURL string
	Token   string
	Public  string
	HTTP    *http.Client
}

// NewPaperless creates a fetcher; public is the browser URL for links.
func NewPaperless(baseURL, token, public string) *Paperless {
	return &Paperless{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Public: strings.TrimRight(public, "/"), HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Paperless) Name() string { return "paperless" }

func (c *Paperless) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" || c.Token == "" {
		return errors.New("paperless is not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+c.Token)
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errors.New("paperless rejected the token")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("paperless HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Test implements Fetcher.
func (c *Paperless) Test(ctx context.Context) (string, error) {
	var out struct {
		Count int `json:"count"`
	}
	if err := c.get(ctx, "/api/documents/", url.Values{"page_size": {"1"}, "fields": {"id"}}, &out); err != nil {
		return "", err
	}
	return fmt.Sprintf("Paperless reachable, %d documents", out.Count), nil
}

func (c *Paperless) names(ctx context.Context, path string) map[int]string {
	out := map[int]string{}
	var res struct {
		Results []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if c.get(ctx, path, url.Values{"page_size": {"1000"}}, &res) == nil {
		for _, r := range res.Results {
			out[r.ID] = r.Name
		}
	}
	return out
}

// Fetch implements Fetcher.
func (c *Paperless) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	var res struct {
		Results []struct {
			ID            int    `json:"id"`
			Title         string `json:"title"`
			Added         string `json:"added"`
			Correspondent *int   `json:"correspondent"`
			DocumentType  *int   `json:"document_type"`
		} `json:"results"`
	}
	// Paperless runs in UTC: ask for the local day as an instant range.
	q := url.Values{"added__gte": {from.UTC().Format(time.RFC3339)}, "added__lt": {to.UTC().Format(time.RFC3339)}, "page_size": {"100"}, "ordering": {"added"},
		"fields": {"id,title,added,correspondent,document_type"}}
	if err := c.get(ctx, "/api/documents/", q, &res); err != nil {
		return nil, err
	}
	act := Activity{}
	if len(res.Results) > 0 {
		corr := c.names(ctx, "/api/correspondents/")
		types := c.names(ctx, "/api/document_types/")
		for _, d := range res.Results {
			at, _ := ParseAny(d.Added)
			if !at.IsZero() && (at.Before(from) || !at.Before(to)) {
				continue
			}
			it := ActivityItem{Time: LocalClock(at, loc), Title: d.Title, Ref: fmt.Sprint(d.ID)}
			if d.Correspondent != nil {
				it.Sub = corr[*d.Correspondent]
			}
			if d.DocumentType != nil {
				it.Group = types[*d.DocumentType]
				it.Sub = JoinNonEmpty(" · ", it.Sub, it.Group)
			}
			if c.Public != "" {
				it.URL = fmt.Sprintf("%s/documents/%d/details", c.Public, d.ID)
			}
			act.Items = append(act.Items, it)
		}
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		sum = Plural(act.Count, "document", "documents") + " · " + act.Items[0].Title
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}
