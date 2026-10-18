package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// getList GETs a list that comes either bare ([…]) or wrapped ({"items": […]},
// {"expenses": […]}, {"data": […]}) and decodes it into out (a slice pointer).
func getList(ctx context.Context, c *pocket.Client, path string, q url.Values, out any) error {
	var raw json.RawMessage
	if err := c.Get(ctx, path, q, &raw); err != nil {
		return err
	}
	if len(raw) > 0 && raw[0] == '[' {
		return json.Unmarshal(raw, out)
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return err
	}
	for _, k := range []string{"items", "expenses", "data", "results", "invoices", "payments"} {
		if v, ok := wrap[k]; ok && len(v) > 0 && v[0] == '[' {
			return json.Unmarshal(v, out)
		}
	}
	return nil
}

// ---------------- Pocket Invoicing ----------------

// Invoicing reads invoices issued and payments received on the day.
type Invoicing struct{ C *pocket.Client }

// NewInvoicing creates a fetcher.
func NewInvoicing(baseURL, token string) *Invoicing {
	return &Invoicing{C: pocket.New(baseURL, token, 20*time.Second)}
}

// Name implements Fetcher.
func (p *Invoicing) Name() string { return "invoicing" }

// Test implements Fetcher.
func (p *Invoicing) Test(ctx context.Context) (string, error) {
	v, err := p.C.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Invoicing " + v, nil
}

// Fetch implements Fetcher.
func (p *Invoicing) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	act := Activity{}
	var inv []struct {
		Number   string  `json:"number"`
		Client   string  `json:"client_name"`
		Status   string  `json:"status"`
		Issue    string  `json:"issue_date"`
		Total    float64 `json:"total"`
		Currency string  `json:"currency"`
	}
	if err := getList(ctx, p.C, "/api/v1/invoices", url.Values{"from": {date}, "to": {date}, "limit": {"100"}}, &inv); err != nil {
		return nil, err
	}
	for _, x := range inv {
		if !strings.HasPrefix(x.Issue, date) || x.Status == "draft" {
			continue
		}
		act.Items = append(act.Items, ActivityItem{Title: "Invoice " + x.Number, Sub: JoinNonEmpty(" · ", x.Client, x.Status), Group: x.Client, Amount: x.Total, Currency: x.Currency, Ref: "inv/" + x.Number})
	}
	var pays []struct {
		Date     string  `json:"date"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
		Invoice  string  `json:"invoice_number"`
		Client   string  `json:"client_name"`
	}
	if err := getList(ctx, p.C, "/api/v1/payments", url.Values{"from": {date}, "to": {date}}, &pays); err == nil {
		for _, x := range pays {
			if strings.HasPrefix(x.Date, date) {
				act.Items = append(act.Items, ActivityItem{Title: "Paid " + x.Invoice, Sub: JoinNonEmpty(" · ", x.Client, "payment received"), Group: x.Client, Done: true, Ref: "pay/" + x.Invoice})
				_ = x.Amount // a payment is the same money as its invoice; keep the day's amount to what was issued
			}
		}
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		sum = Plural(act.Count, "invoice event", "invoice events")
		if act.Amount > 0 {
			sum += fmt.Sprintf(" · %.2f %s invoiced", act.Amount, act.Cur)
		}
	}
	return &Result{Payload: act, Headline: act.Amount, Summary: sum}, nil
}

// ---------------- Pocket Split ----------------

// Split reads the shared expenses dated on the day, across every group.
type Split struct{ C *pocket.Client }

// NewSplit creates a fetcher.
func NewSplit(baseURL, token string) *Split {
	return &Split{C: pocket.New(baseURL, token, 20*time.Second)}
}

// Name implements Fetcher.
func (p *Split) Name() string { return "split" }

// Test implements Fetcher.
func (p *Split) Test(ctx context.Context) (string, error) {
	v, err := p.C.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Split " + v, nil
}

// Fetch implements Fetcher.
func (p *Split) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	var groups []struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
		Members  []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"members"`
	}
	if err := getList(ctx, p.C, "/api/v1/groups", nil, &groups); err != nil {
		return nil, err
	}
	act := Activity{}
	for _, g := range groups {
		var ex []struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
			Date     string `json:"date"`
			PaidBy   int64  `json:"paid_by"`
			Category string `json:"category"`
		}
		if err := getList(ctx, p.C, fmt.Sprintf("/api/v1/groups/%d/expenses", g.ID), url.Values{"from": {date}, "to": {date}}, &ex); err != nil {
			continue
		}
		who := map[int64]string{}
		var full struct {
			Members []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"members"`
		}
		if p.C.Get(ctx, fmt.Sprintf("/api/v1/groups/%d", g.ID), nil, &full) == nil {
			for _, m := range full.Members {
				who[m.ID] = m.Name
			}
		}
		for _, e := range ex {
			if !strings.HasPrefix(e.Date, date) {
				continue
			}
			act.Items = append(act.Items, ActivityItem{Title: e.Title, Sub: JoinNonEmpty(" · ", g.Name, "paid by "+who[e.PaidBy]), Group: g.Name, Amount: float64(e.Amount) / math.Pow10(2), Currency: e.Currency, Ref: fmt.Sprint(e.ID)})
		}
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		sum = fmt.Sprintf("%s · %.2f %s", Plural(act.Count, "shared expense", "shared expenses"), act.Amount, act.Cur)
	}
	return &Result{Payload: act, Headline: act.Amount, Summary: sum}, nil
}

// ---------------- Pocket Inventory ----------------

// Inventory reads stock changes and new items on the day.
type Inventory struct{ C *pocket.Client }

// NewInventory creates a fetcher.
func NewInventory(baseURL, token string) *Inventory {
	return &Inventory{C: pocket.New(baseURL, token, 20*time.Second)}
}

// Name implements Fetcher.
func (p *Inventory) Name() string { return "inventory" }

// Test implements Fetcher.
func (p *Inventory) Test(ctx context.Context) (string, error) {
	v, err := p.C.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Inventory " + v, nil
}

// Fetch implements Fetcher.
func (p *Inventory) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	var rows []struct {
		ItemName  string   `json:"item_name"`
		Delta     *float64 `json:"delta"`
		Note      string   `json:"note"`
		Action    string   `json:"action"`
		User      string   `json:"user_name"`
		CreatedAt string   `json:"created_at"`
	}
	if err := getList(ctx, p.C, "/api/v1/activity", url.Values{"limit": {"1000"}}, &rows); err != nil {
		return nil, err
	}
	act := Activity{}
	for _, r := range rows {
		at, ok := ParseAny(r.CreatedAt)
		if !ok || at.In(loc).Format("2006-01-02") != date || r.ItemName == "" {
			continue
		}
		sub := r.Note
		if r.Delta != nil && *r.Delta != 0 {
			sub = JoinNonEmpty(" · ", fmt.Sprintf("%+g", *r.Delta), sub)
		} else if r.Action != "" {
			sub = JoinNonEmpty(" · ", r.Action, sub)
		}
		act.Items = append(act.Items, ActivityItem{Time: LocalClock(at, loc), Title: r.ItemName, Sub: sub, Group: r.ItemName})
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		sum = Plural(act.Count, "inventory change", "inventory changes") + " · " + act.Items[0].Title
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}
