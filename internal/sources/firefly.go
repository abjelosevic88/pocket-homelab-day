package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Firefly reads the day's withdrawals out of Firefly III.
type Firefly struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewFirefly creates a client.
func NewFirefly(baseURL, token string) *Firefly {
	return &Firefly{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements Fetcher.
func (c *Firefly) Name() string { return "firefly" }

func (c *Firefly) get(ctx context.Context, path string, q url.Values, v any) error {
	if c.BaseURL == "" || c.Token == "" {
		return errors.New("firefly is not configured")
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
	req.Header.Set("Accept", "application/vnd.api+json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("firefly rejected the token (401)")
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("firefly HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Test implements Fetcher.
func (c *Firefly) Test(ctx context.Context) (string, error) {
	var out struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/api/v1/about", nil, &out); err != nil {
		return "", err
	}
	return "Firefly III " + out.Data.Version, nil
}

// Spend is one withdrawal split.
type Spend struct {
	ID          string   `json:"id"`
	Time        string   `json:"time"`
	Description string   `json:"description"`
	Amount      float64  `json:"amount"`
	Currency    string   `json:"currency"`
	Category    string   `json:"category,omitempty"`
	Budget      string   `json:"budget,omitempty"`
	Destination string   `json:"destination,omitempty"`
	Source      string   `json:"source,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// Money is the day's spending.
type Money struct {
	Count      int                `json:"count"`
	Totals     map[string]float64 `json:"totals"` // currency → amount
	Categories []Count            `json:"categories"`
	Biggest    *Spend             `json:"biggest,omitempty"`
	Items      []Spend            `json:"items"`
	Main       string             `json:"main_currency"`
}

// Fetch implements Fetcher.
func (c *Firefly) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	m := Money{Totals: map[string]float64{}, Items: []Spend{}, Categories: []Count{}}
	cats := map[string]int{}
	for page := 1; page < 50; page++ {
		var res struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Transactions []struct {
						JournalID   string   `json:"transaction_journal_id"`
						Type        string   `json:"type"`
						Date        string   `json:"date"`
						Description string   `json:"description"`
						Amount      string   `json:"amount"`
						Currency    string   `json:"currency_code"`
						Category    *string  `json:"category_name"`
						Budget      *string  `json:"budget_name"`
						Source      *string  `json:"source_name"`
						Destination *string  `json:"destination_name"`
						Tags        []string `json:"tags"`
					} `json:"transactions"`
				} `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		q := url.Values{"start": {date}, "end": {date}, "type": {"withdrawal"}, "limit": {"100"}, "page": {strconv.Itoa(page)}}
		if err := c.get(ctx, "/api/v1/transactions", q, &res); err != nil {
			return nil, err
		}
		for _, g := range res.Data {
			for _, t := range g.Attributes.Transactions {
				if t.Type != "withdrawal" {
					continue
				}
				amt, _ := strconv.ParseFloat(t.Amount, 64)
				sp := Spend{ID: t.JournalID, Description: t.Description, Amount: amt, Currency: t.Currency, Tags: t.Tags}
				if ts, ok := ParseAny(t.Date); ok {
					sp.Time = LocalClock(ts, loc)
					if sp.Time == "00:00" {
						sp.Time = ""
					}
				}
				sp.Category = deref(t.Category)
				sp.Budget = deref(t.Budget)
				sp.Source = deref(t.Source)
				sp.Destination = deref(t.Destination)
				m.Items = append(m.Items, sp)
				m.Totals[sp.Currency] += amt
				if sp.Category != "" {
					cats[sp.Category]++
				}
				if m.Biggest == nil || (sp.Currency == m.Biggest.Currency && sp.Amount > m.Biggest.Amount) {
					b := sp
					m.Biggest = &b
				}
			}
		}
		if page >= res.Meta.Pagination.TotalPages {
			break
		}
	}
	sort.SliceStable(m.Items, func(i, j int) bool { return m.Items[i].Time < m.Items[j].Time })
	m.Count = len(m.Items)
	m.Categories = TopCounts(cats, 5)
	for cur, v := range m.Totals {
		m.Totals[cur] = Round2(v)
		if m.Main == "" || m.Totals[cur] > m.Totals[m.Main] {
			m.Main = cur
		}
	}
	headline := 0.0
	sum := ""
	if m.Count > 0 {
		headline = m.Totals[m.Main]
		var parts []string
		curs := make([]string, 0, len(m.Totals))
		for cur := range m.Totals {
			curs = append(curs, cur)
		}
		sort.Strings(curs)
		for _, cur := range curs {
			parts = append(parts, fmt.Sprintf("%.2f %s", m.Totals[cur], cur))
		}
		sum = strings.Join(parts, " + ") + " in " + Plural(m.Count, "transaction", "transactions")
	}
	return &Result{Payload: m, Headline: headline, Summary: sum}, nil
}

// Round2 rounds to cents.
func Round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
