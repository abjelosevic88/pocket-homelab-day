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

// Dawarich reads the day's timeline: distance, moving time, how (walking,
// driving…) and the stops it recognised.
type Dawarich struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewDawarich creates a client.
func NewDawarich(baseURL, key string) *Dawarich {
	return &Dawarich{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: key, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Dawarich) Name() string { return "dawarich" }

func (c *Dawarich) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" || c.APIKey == "" {
		return errors.New("dawarich is not configured")
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("api_key", c.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	// Dawarich behind APPLICATION_PROTOCOL=https redirects plain http unless told it was https.
	req.Header.Set("X-Forwarded-Proto", "https")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("dawarich rejected the API key (401)")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("dawarich HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Test implements Fetcher.
func (c *Dawarich) Test(ctx context.Context) (string, error) {
	var out map[string]any
	if err := c.get(ctx, "/api/v1/health", nil, &out); err != nil {
		return "", err
	}
	return "Dawarich reachable", nil
}

// Visit is a stop Dawarich recognised.
type Visit struct {
	Name     string   `json:"name"`
	Start    string   `json:"start"` // HH:MM
	End      string   `json:"end"`
	Minutes  int      `json:"minutes"`
	Lat      *float64 `json:"lat,omitempty"`
	Lng      *float64 `json:"lng,omitempty"`
	Status   string   `json:"status,omitempty"`
	Kind     string   `json:"kind"` // visit | journey
	Mode     string   `json:"mode,omitempty"`
	Distance float64  `json:"distance_km,omitempty"`
}

// Where is the day's movement.
type Where struct {
	DistanceKm    float64            `json:"distance_km"`
	MovingMinutes int                `json:"moving_minutes"`
	StillMinutes  int                `json:"still_minutes"`
	Modes         map[string]float64 `json:"modes"` // mode → km
	Places        int                `json:"places"`
	Journeys      int                `json:"journeys"`
	Entries       []Visit            `json:"entries"`
	Bounds        *[4]float64        `json:"bounds,omitempty"` // swLat, swLng, neLat, neLng
	FirstMove     string             `json:"first_move"`
	LastMove      string             `json:"last_move"`
	// Provider is where the day came from: "movement" (Pocket Movement) or
	// "dawarich"; "" on days recorded before Pocket Movement existed.
	Provider string `json:"provider,omitempty"`
}

// Fetch implements Fetcher.
func (c *Dawarich) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	var out struct {
		Days []struct {
			Date    string `json:"date"`
			Summary struct {
				TotalDistance float64            `json:"total_distance"`
				DistanceUnit  string             `json:"distance_unit"`
				PlacesVisited int                `json:"places_visited"`
				Moving        int                `json:"time_moving_minutes"`
				Stationary    int                `json:"time_stationary_minutes"`
				Modes         map[string]float64 `json:"mode_distances"`
			} `json:"summary"`
			Bounds *struct {
				SwLat float64 `json:"sw_lat"`
				SwLng float64 `json:"sw_lng"`
				NeLat float64 `json:"ne_lat"`
				NeLng float64 `json:"ne_lng"`
			} `json:"bounds"`
			Entries []struct {
				Type      string   `json:"type"`
				Name      string   `json:"name"`
				Status    string   `json:"status"`
				StartedAt string   `json:"started_at"`
				EndedAt   string   `json:"ended_at"`
				Duration  int      `json:"duration"`
				Distance  float64  `json:"distance"`
				Mode      string   `json:"dominant_mode"`
				Lat       *float64 `json:"latitude"`
				Lng       *float64 `json:"longitude"`
				Moving    int      `json:"moving_duration"` // seconds, journeys only
			} `json:"entries"`
		} `json:"days"`
	}
	q := url.Values{"start_at": {from.UTC().Format(time.RFC3339)}, "end_at": {to.Add(-time.Second).UTC().Format(time.RFC3339)}}
	if err := c.get(ctx, "/api/v1/timeline", q, &out); err != nil {
		return nil, err
	}
	w := Where{Modes: map[string]float64{}, Entries: []Visit{}, Provider: "dawarich"}
	journeyMoving := 0 // seconds; Dawarich's day summary counts gaps inside tracks as moving
	for _, d := range out.Days {
		if d.Date != date && len(out.Days) > 1 {
			continue
		}
		w.DistanceKm += d.Summary.TotalDistance
		if strings.HasPrefix(strings.ToLower(d.Summary.DistanceUnit), "mi") {
			w.DistanceKm = d.Summary.TotalDistance * 1.609344
		}
		w.MovingMinutes += d.Summary.Moving
		w.StillMinutes += d.Summary.Stationary
		w.Places += d.Summary.PlacesVisited
		for k, v := range d.Summary.Modes {
			w.Modes[k] += v
		}
		if d.Bounds != nil {
			w.Bounds = &[4]float64{d.Bounds.SwLat, d.Bounds.SwLng, d.Bounds.NeLat, d.Bounds.NeLng}
		}
		for _, e := range d.Entries {
			st, _ := ParseAny(e.StartedAt)
			en, _ := ParseAny(e.EndedAt)
			v := Visit{Kind: e.Type, Name: e.Name, Status: e.Status, Start: LocalClock(st, loc), End: LocalClock(en, loc), Minutes: e.Duration / 60, Lat: e.Lat, Lng: e.Lng, Mode: e.Mode, Distance: Round1(e.Distance)}
			if e.Type == "journey" {
				w.Journeys++
				journeyMoving += e.Moving
				if w.FirstMove == "" {
					w.FirstMove = v.Start
				}
				w.LastMove = v.End
			} else if e.Status == "declined" {
				continue
			}
			w.Entries = append(w.Entries, v)
		}
	}
	if journeyMoving > 0 {
		w.MovingMinutes = journeyMoving / 60
	}
	w.DistanceKm = Round1(w.DistanceKm)
	for k, v := range w.Modes {
		w.Modes[k] = Round1(v)
	}
	return &Result{Payload: w, Headline: w.DistanceKm, Summary: whereSummary(w)}, nil
}
