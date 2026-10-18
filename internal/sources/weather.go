package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"
)

// The day's weather comes from Open-Meteo (free, no key): the forecast API
// answers the last three months, the archive API anything older.
const (
	openMeteoForecast = "https://api.open-meteo.com/v1/forecast"
	openMeteoArchive  = "https://archive-api.open-meteo.com/v1/archive"
	openMeteoGeocode  = "https://geocoding-api.open-meteo.com/v1/search"
	dailyVars         = "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,sunshine_duration,daylight_duration,wind_speed_10m_max"
)

// Weather fetches one place's daily summary.
type Weather struct {
	Lat, Lon  float64
	Place     string
	UserAgent string
	HTTP      *http.Client
}

// NewWeather creates a client for the home location.
func NewWeather(lat, lon float64, place, ua string) *Weather {
	return &Weather{Lat: lat, Lon: lon, Place: place, UserAgent: ua, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Name implements Fetcher.
func (c *Weather) Name() string { return "weather" }

// Configured reports whether a home location is set.
func (c *Weather) Configured() bool { return c != nil && (c.Lat != 0 || c.Lon != 0) }

// Day is one day of weather.
type Day struct {
	Code      int      `json:"code"`
	Condition string   `json:"condition"`
	TMax      *float64 `json:"t_max"`
	TMin      *float64 `json:"t_min"`
	PrecipMM  *float64 `json:"precip_mm"`
	SunshineH *float64 `json:"sunshine_h"`
	DaylightH *float64 `json:"daylight_h"`
	WindMax   *float64 `json:"wind_max"`
	Place     string   `json:"place"`
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
}

// Test implements Fetcher.
func (c *Weather) Test(ctx context.Context) (string, error) {
	if !c.Configured() {
		return "", errors.New("set a home location first")
	}
	d, err := c.fetchAt(ctx, c.Lat, c.Lon, c.Place, time.Now().Format("2006-01-02"))
	if err != nil {
		return "", err
	}
	if d == nil {
		return "Open-Meteo reachable", nil
	}
	return fmt.Sprintf("Open-Meteo reachable: %s today at %s", ConditionLabel(d.Condition), c.Place), nil
}

// Fetch implements Fetcher (home location).
func (c *Weather) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	return c.FetchAt(ctx, date, c.Lat, c.Lon, c.Place)
}

// FetchAt fetches the weather for a date at a place (used when Dawarich knows where the day was).
func (c *Weather) FetchAt(ctx context.Context, date string, lat, lon float64, place string) (*Result, error) {
	if lat == 0 && lon == 0 {
		return nil, errors.New("no location")
	}
	d, err := c.fetchAt(ctx, lat, lon, place, date)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, errors.New("open-meteo has nothing for that day yet")
	}
	sum := ConditionLabel(d.Condition)
	if d.TMax != nil && d.TMin != nil {
		sum += fmt.Sprintf(", %.0f–%.0f °C", *d.TMin, *d.TMax)
	}
	if d.PrecipMM != nil && *d.PrecipMM >= 1 {
		sum += fmt.Sprintf(", %.0f mm", *d.PrecipMM)
	}
	h := 0.0
	if d.TMax != nil {
		h = *d.TMax
	}
	return &Result{Payload: d, Headline: h, Summary: sum}, nil
}

func (c *Weather) fetchAt(ctx context.Context, lat, lon float64, place, date string) (*Day, error) {
	base := openMeteoForecast
	if date < time.Now().AddDate(0, 0, -85).Format("2006-01-02") {
		base = openMeteoArchive
	}
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("daily", dailyVars)
	q.Set("timezone", "auto")
	q.Set("start_date", date)
	q.Set("end_date", date)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		if e.Reason == "" {
			e.Reason = res.Status
		}
		return nil, fmt.Errorf("open-meteo: %s", e.Reason)
	}
	var body struct {
		Daily struct {
			Time     []string   `json:"time"`
			Code     []*float64 `json:"weather_code"`
			TMax     []*float64 `json:"temperature_2m_max"`
			TMin     []*float64 `json:"temperature_2m_min"`
			Precip   []*float64 `json:"precipitation_sum"`
			Sunshine []*float64 `json:"sunshine_duration"`
			Daylight []*float64 `json:"daylight_duration"`
			Wind     []*float64 `json:"wind_speed_10m_max"`
		} `json:"daily"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("open-meteo: %w", err)
	}
	at := func(a []*float64, i int) *float64 {
		if i < len(a) && a[i] != nil {
			v := math.Round(*a[i]*10) / 10
			return &v
		}
		return nil
	}
	for i, d := range body.Daily.Time {
		if d != date {
			continue
		}
		code := -1
		if cd := at(body.Daily.Code, i); cd != nil {
			code = int(*cd)
		}
		if code < 0 && at(body.Daily.TMax, i) == nil {
			return nil, nil
		}
		w := &Day{Code: code, Condition: ConditionFromCode(code), TMax: at(body.Daily.TMax, i), TMin: at(body.Daily.TMin, i), PrecipMM: at(body.Daily.Precip, i), WindMax: at(body.Daily.Wind, i), Place: place, Lat: lat, Lon: lon}
		if s := at(body.Daily.Sunshine, i); s != nil {
			h := math.Round(*s/360) / 10
			w.SunshineH = &h
		}
		if s := at(body.Daily.Daylight, i); s != nil {
			h := math.Round(*s/360) / 10
			w.DaylightH = &h
		}
		return w, nil
	}
	return nil, nil
}

// ConditionFromCode maps a WMO code to a word.
func ConditionFromCode(code int) string {
	switch {
	case code == 0:
		return "clear"
	case code == 1 || code == 2:
		return "partly"
	case code == 3:
		return "cloudy"
	case code == 45 || code == 48:
		return "fog"
	case code >= 51 && code <= 67, code >= 80 && code <= 82:
		return "rain"
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return "snow"
	case code >= 95:
		return "storm"
	}
	return ""
}

// ConditionLabel is the word shown for a condition.
func ConditionLabel(c string) string {
	switch c {
	case "clear":
		return "Clear"
	case "partly":
		return "Partly cloudy"
	case "cloudy":
		return "Overcast"
	case "fog":
		return "Fog"
	case "rain":
		return "Rain"
	case "snow":
		return "Snow"
	case "storm":
		return "Thunderstorm"
	}
	return "Weather"
}

// Geocode looks a place name up.
func Geocode(ctx context.Context, ua, q string) ([]Place, error) {
	u := openMeteoGeocode + "?" + url.Values{"name": {q}, "count": {"8"}, "language": {"en"}, "format": {"json"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var body struct {
		Results []struct {
			Name    string  `json:"name"`
			Admin1  string  `json:"admin1"`
			Country string  `json:"country"`
			Lat     float64 `json:"latitude"`
			Lon     float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := []Place{}
	for _, r := range body.Results {
		out = append(out, Place{Name: r.Name, Region: JoinNonEmpty(", ", r.Admin1, r.Country), Lat: r.Lat, Lon: r.Lon})
	}
	return out, nil
}

// Place is a geocoding hit.
type Place struct {
	Name   string  `json:"name"`
	Region string  `json:"region"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}
