package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
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
type Invoicing struct{ Conn pocket.Conn }

// NewInvoicing creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewInvoicing(conn pocket.Conn) *Invoicing { return &Invoicing{Conn: conn} }

// Name implements Fetcher.
func (p *Invoicing) Name() string { return "invoicing" }

// Test implements Fetcher.
func (p *Invoicing) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Invoicing " + v, nil
}

// Fetch implements Fetcher.
func (p *Invoicing) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	act := Activity{}
	var inv []struct {
		Number   string  `json:"number"`
		Client   string  `json:"client_name"`
		Status   string  `json:"status"`
		Issue    string  `json:"issue_date"`
		Total    float64 `json:"total"`
		Currency string  `json:"currency"`
	}
	if err := getList(ctx, cl, "/api/v1/invoices", url.Values{"from": {date}, "to": {date}, "limit": {"100"}}, &inv); err != nil {
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
	if err := getList(ctx, cl, "/api/v1/payments", url.Values{"from": {date}, "to": {date}}, &pays); err == nil {
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
type Split struct{ Conn pocket.Conn }

// NewSplit creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewSplit(conn pocket.Conn) *Split { return &Split{Conn: conn} }

// Name implements Fetcher.
func (p *Split) Name() string { return "split" }

// Test implements Fetcher.
func (p *Split) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Split " + v, nil
}

// Fetch implements Fetcher.
func (p *Split) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var groups []struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
		Members  []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"members"`
	}
	if err := getList(ctx, cl, "/api/v1/groups", nil, &groups); err != nil {
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
		if err := getList(ctx, cl, fmt.Sprintf("/api/v1/groups/%d/expenses", g.ID), url.Values{"from": {date}, "to": {date}}, &ex); err != nil {
			continue
		}
		who := map[int64]string{}
		var full struct {
			Members []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"members"`
		}
		if cl.Get(ctx, fmt.Sprintf("/api/v1/groups/%d", g.ID), nil, &full) == nil {
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
type Inventory struct{ Conn pocket.Conn }

// NewInventory creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewInventory(conn pocket.Conn) *Inventory { return &Inventory{Conn: conn} }

// Name implements Fetcher.
func (p *Inventory) Name() string { return "inventory" }

// Test implements Fetcher.
func (p *Inventory) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Inventory " + v, nil
}

// Fetch implements Fetcher.
func (p *Inventory) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ItemName  string   `json:"item_name"`
		Delta     *float64 `json:"delta"`
		Note      string   `json:"note"`
		Action    string   `json:"action"`
		User      string   `json:"user_name"`
		CreatedAt string   `json:"created_at"`
	}
	if err := getList(ctx, cl, "/api/v1/activity", url.Values{"limit": {"1000"}}, &rows); err != nil {
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

// ---------------- Pocket Puzzlecraft ----------------

// Puzzlecraft reads the puzzles solved on the day — the person's own (the
// puzzles of the day, their crosswords and sudoku) and by friends playing
// what they shared — and the escape rooms or hunts planned that day.
type Puzzlecraft struct{ Conn pocket.Conn }

// NewPuzzlecraft creates a fetcher over a connection.
func NewPuzzlecraft(conn pocket.Conn) *Puzzlecraft { return &Puzzlecraft{Conn: conn} }

// Name implements Fetcher.
func (p *Puzzlecraft) Name() string { return "puzzlecraft" }

// Test implements Fetcher.
func (p *Puzzlecraft) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Puzzlecraft " + v, nil
}

// Fetch implements Fetcher.
func (p *Puzzlecraft) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var day struct {
		Minutes float64 `json:"minutes"`
		Items   []struct {
			Time  string `json:"time"`
			Title string `json:"title"`
			Sub   string `json:"sub"`
			URL   string `json:"url"`
			Kind  string `json:"kind"`
			Mine  bool   `json:"mine"`
		} `json:"items"`
		Planned []struct {
			Title string `json:"title"`
			Time  string `json:"time"`
			Kind  string `json:"kind"`
			Place string `json:"place"`
			URL   string `json:"url"`
		} `json:"planned"`
	}
	if err := cl.Do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &day); err != nil {
		return nil, err
	}
	act := Activity{}
	kindName := map[string]string{"crossword": "Crossword", "wordsearch": "Word search", "sudoku": "Sudoku", "nonogram": "Picture puzzle", "cryptogram": "Secret code",
		"scramble": "Word scramble", "quiz": "Quiz", "maze": "Maze", "logic": "Logic grid", "ladder": "Word ladder", "jigsaw": "Jigsaw",
		"escape": "Escape room", "hunt": "Treasure hunt", "mystery": "Mystery", "party": "Party game"}
	mine := 0
	for _, x := range day.Items {
		group := firstNonEmptyStr(kindName[x.Kind], "Puzzle")
		if !x.Mine {
			group = "Solved by others"
		} else {
			mine++
		}
		act.Items = append(act.Items, ActivityItem{Time: x.Time, Title: x.Title, Sub: x.Sub, Group: group, Done: true, URL: x.URL})
	}
	for _, x := range day.Planned {
		act.Items = append(act.Items, ActivityItem{Time: x.Time, Title: x.Title, Sub: JoinNonEmpty(" · ", firstNonEmptyStr(kindName[x.Kind], "Game"), x.Place, "planned"), Group: "Planned", URL: x.URL})
	}
	act.Finish()
	act.Minutes = day.Minutes
	sum := ""
	if mine > 0 {
		sum = Plural(mine, "puzzle solved", "puzzles solved")
	}
	if others := len(day.Items) - mine; others > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(others, "solved by others", "solved by others"))
	}
	if len(day.Planned) > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(len(day.Planned), "game planned", "games planned"))
	}
	return &Result{Payload: act, Headline: float64(mine), Summary: sum}, nil
}

// ---------------- Pocket Capsule ----------------

// Capsule reads the day's time capsules: the ones started, sealed, opened
// (or broken open early), written back to and put things in, and the ones
// that open that day. Pocket Capsule never says what is inside a sealed one.
type Capsule struct{ Conn pocket.Conn }

// NewCapsule creates a fetcher over a connection.
func NewCapsule(conn pocket.Conn) *Capsule { return &Capsule{Conn: conn} }

// Name implements Fetcher.
func (p *Capsule) Name() string { return "capsule" }

// Test implements Fetcher.
func (p *Capsule) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Capsule " + v, nil
}

// capsuleGroup names what happened to a capsule, for the day's groups.
var capsuleGroup = map[string]string{"made": "Started", "sealed": "Sealed", "opened": "Opened", "early": "Opened", "ready": "Opens today", "opens": "Opens today", "reflected": "Wrote back", "added": "Put in"}

// Fetch implements Fetcher.
func (p *Capsule) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var day struct {
		Items []struct {
			Time      string `json:"time"`
			Kind      string `json:"kind"`
			Text      string `json:"text"`
			Emoji     string `json:"emoji"`
			CapsuleID int64  `json:"capsule_id"`
			URL       string `json:"url"`
		} `json:"items"`
	}
	if err := cl.Do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &day); err != nil {
		return nil, err
	}
	act := Activity{}
	counts := map[string]int{}
	for _, x := range day.Items {
		if strings.TrimSpace(x.Text) == "" {
			continue
		}
		group := firstNonEmptyStr(capsuleGroup[x.Kind], "Time capsule")
		counts[group]++
		opened := x.Kind == "opened" || x.Kind == "early"
		act.Items = append(act.Items, ActivityItem{Time: x.Time, Title: JoinNonEmpty(" ", x.Emoji, x.Text), Group: group, Done: opened, Ref: fmt.Sprint(x.CapsuleID), URL: x.URL})
	}
	act.Finish()
	sum := ""
	for _, g := range [][2]string{{"Opened", "Opened"}, {"Sealed", "Sealed"}, {"Started", "Started"}, {"Put in", "Put things in"}, {"Wrote back", "Wrote back to"}} {
		if n := counts[g[0]]; n > 0 {
			sum = JoinNonEmpty(" · ", sum, g[1]+" "+Plural(n, "time capsule", "time capsules"))
		}
	}
	if n := counts["Opens today"]; n > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(n, "time capsule opens", "time capsules open"))
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}

// ---------------- Pocket Math ----------------

// Math reads the day's maths: notebooks worked on or started, answers kept
// from ready calculations, and practice rounds (by the person and children).
type Math struct{ Conn pocket.Conn }

// NewMath creates a fetcher over a connection.
func NewMath(conn pocket.Conn) *Math { return &Math{Conn: conn} }

// Name implements Fetcher.
func (p *Math) Name() string { return "math" }

// Test implements Fetcher.
func (p *Math) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Math " + v, nil
}

var mathGroup = map[string]string{"notebook": "Notebooks", "answer": "Worked out", "practice": "Practice"}

// Fetch implements Fetcher.
func (p *Math) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var day struct {
		Items []struct {
			Time  string `json:"time"`
			Kind  string `json:"kind"`
			Text  string `json:"text"`
			Emoji string `json:"emoji"`
			URL   string `json:"url"`
		} `json:"items"`
	}
	if err := cl.Do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &day); err != nil {
		return nil, err
	}
	act := Activity{}
	counts := map[string]int{}
	for i, x := range day.Items {
		if strings.TrimSpace(x.Text) == "" {
			continue
		}
		group := firstNonEmptyStr(mathGroup[x.Kind], "Maths")
		counts[x.Kind]++
		act.Items = append(act.Items, ActivityItem{Time: x.Time, Title: JoinNonEmpty(" ", x.Emoji, x.Text), Group: group, Done: x.Kind == "answer", Ref: fmt.Sprint(i), URL: x.URL})
	}
	act.Finish()
	sum := ""
	if n := counts["notebook"]; n > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(n, "notebook", "notebooks"))
	}
	if n := counts["answer"]; n > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(n, "answer kept", "answers kept"))
	}
	if n := counts["practice"]; n > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(n, "practice round", "practice rounds"))
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}

// ---------------- Pocket Karaoke ----------------

// Karaoke reads the day's karaoke: the nights held and the songs sung (who
// sang what, with the score), practice songs on their own.
type Karaoke struct{ Conn pocket.Conn }

// NewKaraoke creates a fetcher over a connection.
func NewKaraoke(conn pocket.Conn) *Karaoke { return &Karaoke{Conn: conn} }

// Name implements Fetcher.
func (p *Karaoke) Name() string { return "karaoke" }

// Test implements Fetcher.
func (p *Karaoke) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Karaoke " + v, nil
}

// Fetch implements Fetcher.
func (p *Karaoke) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var day struct {
		Items []struct {
			Kind     string `json:"kind"`
			Text     string `json:"text"`
			Time     string `json:"time"`
			URL      string `json:"url"`
			Practice bool   `json:"practice"`
		} `json:"items"`
	}
	if err := cl.Do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &day); err != nil {
		return nil, err
	}
	act := Activity{}
	nights, sung, practice := 0, 0, 0
	for i, x := range day.Items {
		if strings.TrimSpace(x.Text) == "" {
			continue
		}
		clock := ""
		if len(x.Time) >= 5 && x.Time[2] == ':' {
			clock = x.Time[:5]
		}
		group := "Sung"
		switch {
		case x.Kind == "night":
			group = "Nights"
			nights++
		case x.Practice:
			group = "Practice"
			practice++
		default:
			sung++
		}
		act.Items = append(act.Items, ActivityItem{Time: clock, Title: x.Text, Group: group, Done: x.Kind == "song", Ref: fmt.Sprint(i), URL: x.URL})
	}
	act.Finish()
	sum := ""
	if sung > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(sung, "song sung", "songs sung"))
	}
	if practice > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(practice, "practice song", "practice songs"))
	}
	if nights > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(nights, "night", "nights"))
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}

// ---------------- Pocket Print Farm ----------------

// PrintFarm reads the day's 3D printing: the prints done, failed or
// started, and the printers' upkeep done.
type PrintFarm struct{ Conn pocket.Conn }

// NewPrintFarm creates a fetcher over a connection.
func NewPrintFarm(conn pocket.Conn) *PrintFarm { return &PrintFarm{Conn: conn} }

// Name implements Fetcher.
func (p *PrintFarm) Name() string { return "printfarm" }

// Test implements Fetcher.
func (p *PrintFarm) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Print Farm " + v, nil
}

// Fetch implements Fetcher.
func (p *PrintFarm) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var day struct {
		Items []struct {
			Kind    string  `json:"kind"`
			Text    string  `json:"text"`
			Status  string  `json:"status"`
			Time    string  `json:"time"`
			Printer string  `json:"printer"`
			Seconds float64 `json:"seconds"`
			URL     string  `json:"url"`
		} `json:"items"`
	}
	if err := cl.Do(ctx, http.MethodGet, "/api/v1/day/"+date, nil, nil, &day); err != nil {
		return nil, err
	}
	act := Activity{}
	done, failed, upkeep := 0, 0, 0
	for i, x := range day.Items {
		if strings.TrimSpace(x.Text) == "" {
			continue
		}
		clock := ""
		if len(x.Time) >= 5 && x.Time[2] == ':' {
			clock = x.Time[:5]
		}
		group := "Printed"
		switch {
		case x.Kind == "upkeep":
			group = "Upkeep"
			upkeep++
		case x.Kind == "model":
			group = "Models added"
		case x.Status == "failed" || x.Status == "cancelled":
			group = "Failed"
			failed++
		case x.Status == "done":
			done++
		default:
			group = "Printing"
		}
		act.Items = append(act.Items, ActivityItem{Time: clock, Title: x.Text, Sub: x.Printer, Group: group, Minutes: x.Seconds / 60, Done: x.Status == "done", Ref: fmt.Sprint(i), URL: x.URL})
	}
	act.Finish()
	sum := ""
	if done > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(done, "print done", "prints done"))
	}
	if failed > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(failed, "failed", "failed"))
	}
	if upkeep > 0 {
		sum = JoinNonEmpty(" · ", sum, Plural(upkeep, "upkeep done", "upkeep done"))
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}
