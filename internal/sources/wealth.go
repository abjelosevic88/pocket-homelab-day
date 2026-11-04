package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Wealth ----------------

// Wealth reads what happened to what you own that day: buys, sells,
// dividends, money put in, and values typed in.
type Wealth struct{ Conn pocket.Conn }

// NewWealth creates a fetcher over a connection.
func NewWealth(conn pocket.Conn) *Wealth { return &Wealth{Conn: conn} }

// Name implements Fetcher.
func (w *Wealth) Name() string { return "wealth" }

// Test implements Fetcher.
func (w *Wealth) Test(ctx context.Context) (string, error) {
	cl, err := w.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Wealth " + v, nil
}

// WealthItem is one thing that happened: a buy, a dividend, a value updated.
type WealthItem struct {
	Type     string `json:"type"` // activity | valuation
	ID       int64  `json:"id"`
	Kind     string `json:"kind,omitempty"` // buy | sell | dividend | interest | deposit | withdrawal | fee
	Title    string `json:"title"`
	Amount   int64  `json:"amount"` // minor units
	Currency string `json:"currency"`
	AssetID  int64  `json:"asset_id"`
	Asset    string `json:"asset,omitempty"`
	URL      string `json:"url,omitempty"`
}

// WealthDay is the day in Pocket Wealth.
type WealthDay struct {
	Items []WealthItem `json:"items"`
}

// Fetch implements Fetcher.
func (w *Wealth) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := w.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page WealthDay
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	if page.Items == nil {
		page.Items = []WealthItem{}
	}
	var parts []string
	n := 0
	for _, it := range page.Items {
		if it.Type == "activity" {
			n++
		}
		line := it.Title
		if it.Amount > 0 && it.Type == "activity" {
			line += " (" + minorAmount(it.Amount, it.Currency) + ")"
		}
		parts = append(parts, line)
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: page, Headline: float64(n), Summary: sum}, nil
}
