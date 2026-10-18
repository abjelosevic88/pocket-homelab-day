package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Immich finds the photos and videos taken on a day and proxies thumbnails
// so the browser never needs the API key.
type Immich struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewImmich creates a client.
func NewImmich(baseURL, key string) *Immich {
	return &Immich{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Immich) Name() string { return "immich" }

// Configured reports whether URL and key are set.
func (c *Immich) Configured() bool { return c != nil && c.BaseURL != "" && c.APIKey != "" }

func (c *Immich) req(ctx context.Context, method, path string, body any) (*http.Response, error) {
	if !c.Configured() {
		return nil, errors.New("immich is not configured")
	}
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTP.Do(req)
}

// Test implements Fetcher.
func (c *Immich) Test(ctx context.Context) (string, error) {
	res, err := c.req(ctx, http.MethodGet, "/api/server/version", nil)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return "", errors.New("immich rejected the API key (401)")
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("immich HTTP %d", res.StatusCode)
	}
	var v struct{ Major, Minor, Patch int }
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		return "", err
	}
	return fmt.Sprintf("Immich %d.%d.%d", v.Major, v.Minor, v.Patch), nil
}

// Photo is one asset.
type Photo struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // IMAGE | VIDEO
	TakenAt  string   `json:"taken_at"`
	Time     string   `json:"time"` // HH:MM local
	Lat      *float64 `json:"lat,omitempty"`
	Lng      *float64 `json:"lng,omitempty"`
	City     string   `json:"city,omitempty"`
	Country  string   `json:"country,omitempty"`
	Favorite bool     `json:"favorite"`
	Duration string   `json:"duration,omitempty"`
	FileName string   `json:"file_name,omitempty"`
}

// Photos is the day's camera roll.
type Photos struct {
	Count     int      `json:"count"`
	Videos    int      `json:"videos"`
	Favorites int      `json:"favorites"`
	Places    []string `json:"places"`
	First     string   `json:"first"`
	Last      string   `json:"last"`
	Items     []Photo  `json:"items"`
}

// Fetch implements Fetcher.
func (c *Immich) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	p := Photos{Items: []Photo{}, Places: []string{}}
	places := map[string]int{}
	for page := 1; page < 50; page++ {
		body := map[string]any{"takenAfter": from.UTC().Format(time.RFC3339), "takenBefore": to.UTC().Format(time.RFC3339), "size": 250, "page": page, "withExif": true, "order": "asc", "visibility": "timeline"}
		res, err := c.req(ctx, http.MethodPost, "/api/search/metadata", body)
		if err != nil {
			return nil, err
		}
		if res.StatusCode == http.StatusUnauthorized {
			res.Body.Close()
			return nil, errors.New("immich rejected the API key (401)")
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return nil, fmt.Errorf("immich search HTTP %d", res.StatusCode)
		}
		var r struct {
			Assets struct {
				Items []struct {
					ID               string          `json:"id"`
					Type             string          `json:"type"`
					OriginalFileName string          `json:"originalFileName"`
					LocalDateTime    string          `json:"localDateTime"`
					IsFavorite       bool            `json:"isFavorite"`
					Duration         json.RawMessage `json:"duration"`
					ExifInfo         *struct {
						Latitude  *float64 `json:"latitude"`
						Longitude *float64 `json:"longitude"`
						City      *string  `json:"city"`
						Country   *string  `json:"country"`
					} `json:"exifInfo"`
				} `json:"items"`
				NextPage *string `json:"nextPage"`
			} `json:"assets"`
		}
		err = json.NewDecoder(res.Body).Decode(&r)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, it := range r.Assets.Items {
			// localDateTime is wall time at the place the photo was taken; the search already filtered the day.
			ph := Photo{ID: it.ID, Type: it.Type, TakenAt: it.LocalDateTime, Favorite: it.IsFavorite, FileName: it.OriginalFileName}
			if len(it.LocalDateTime) >= 16 {
				ph.Time = it.LocalDateTime[11:16]
			}
			if it.Type == "VIDEO" {
				p.Videos++
				ph.Duration = durationText(it.Duration)
			}
			if it.IsFavorite {
				p.Favorites++
			}
			if it.ExifInfo != nil {
				ph.Lat, ph.Lng = it.ExifInfo.Latitude, it.ExifInfo.Longitude
				if it.ExifInfo.City != nil {
					ph.City = *it.ExifInfo.City
				}
				if it.ExifInfo.Country != nil {
					ph.Country = *it.ExifInfo.Country
				}
				if ph.City != "" {
					places[ph.City]++
				}
			}
			p.Items = append(p.Items, ph)
		}
		if r.Assets.NextPage == nil || len(r.Assets.Items) == 0 {
			break
		}
	}
	p.Count = len(p.Items)
	if p.Count > 0 {
		p.First, p.Last = p.Items[0].Time, p.Items[p.Count-1].Time
	}
	for _, c := range TopCounts(places, 5) {
		p.Places = append(p.Places, c.Name)
	}
	sum := ""
	if p.Count > 0 {
		sum = Plural(p.Count-p.Videos, "photo", "photos")
		if p.Videos > 0 {
			sum += ", " + Plural(p.Videos, "video", "videos")
		}
		if len(p.Places) > 0 {
			sum += " · " + strings.Join(p.Places, ", ")
		}
	}
	return &Result{Payload: p, Headline: float64(p.Count), Summary: sum}, nil
}

// Thumbnail streams a thumbnail (size thumbnail|preview) to w.
func (c *Immich) Thumbnail(ctx context.Context, w http.ResponseWriter, id, size string) error {
	if size != "preview" {
		size = "thumbnail"
	}
	if strings.ContainsAny(id, "/?&") {
		return errors.New("bad asset id")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/assets/"+id+"/thumbnail?size="+size, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "image/*")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("immich thumbnail HTTP %d", res.StatusCode)
	}
	w.Header().Set("Content-Type", res.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "private, max-age=604800")
	_, err = io.Copy(w, res.Body)
	return err
}

// durationText accepts Immich's duration as "0:00:12.000" (older) or milliseconds (3.x).
func durationText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if len(s) >= 7 && strings.HasPrefix(s, "0:") {
			return s[2:7]
		}
		return s
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		sec := int(f/1000 + 0.5)
		return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
	}
	return ""
}
