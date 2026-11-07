package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket CRM ----------------

// CRM reads the day in Pocket CRM: the deals you added, won or lost, the
// talks you had (calls, meetings, emails), the steps you ticked off and the
// next steps that were due, each linking to its deal. Only the lines' titles:
// the notes written on a deal stay in Pocket CRM.
type CRM struct{ Conn pocket.Conn }

// NewCRM creates a fetcher over a connection.
func NewCRM(conn pocket.Conn) *CRM { return &CRM{Conn: conn} }

// Name implements Fetcher.
func (c *CRM) Name() string { return "crm" }

// Test implements Fetcher.
func (c *CRM) Test(ctx context.Context) (string, error) {
	cl, err := c.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket CRM " + v, nil
}

// CRMActivity is a line on a deal's timeline that day.
type CRMActivity struct {
	ID      int64  `json:"id"`
	DealID  int64  `json:"deal_id"`
	Kind    string `json:"kind"` // note|call|meeting|email|step|stage|won|lost|made|created
	Title   string `json:"title"`
	Note    string `json:"note,omitempty"` // a short clip of the line's body, for the card only
	Time    string `json:"time,omitempty"` // HH:MM local
	Deal    string `json:"deal"`
	Company string `json:"company,omitempty"` // for the deals added, won or lost
	URL     string `json:"url,omitempty"`     // the deal in Pocket CRM
}

// CRMDue is a deal whose next step was due that day.
type CRMDue struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Company    string  `json:"company,omitempty"`
	Contact    string  `json:"contact,omitempty"`
	NextAction string  `json:"next_action,omitempty"`
	Stage      string  `json:"stage,omitempty"`
	Amount     float64 `json:"amount,omitempty"`
	Currency   string  `json:"currency,omitempty"`
	URL        string  `json:"url,omitempty"`
}

// CRMDay is the day in Pocket CRM.
type CRMDay struct {
	Date       string         `json:"date"`
	Activities []CRMActivity  `json:"activities"`
	Counts     map[string]int `json:"counts"`
	Due        []CRMDue       `json:"due"`
	Won        int            `json:"won"`
	Lost       int            `json:"lost"`
	Created    int            `json:"created"`
	URL        string         `json:"url,omitempty"`
}

type crmDeal struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	StageName   string  `json:"stage_name"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	ContactName string  `json:"contact_name"`
	Company     string  `json:"company"`
	NextAction  string  `json:"next_action"`
	NextDue     string  `json:"next_due"`
	URL         string  `json:"url"`
}

// Fetch implements Fetcher.
func (c *CRM) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := c.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Activities []struct {
			ID      int64  `json:"id"`
			DealID  int64  `json:"deal_id"`
			Kind    string `json:"kind"`
			Title   string `json:"title"`
			Body    string `json:"body"`
			At      string `json:"at"`
			Deal    string `json:"deal"`
			DealURL string `json:"deal_url"`
		} `json:"activities"`
		Counts  map[string]int `json:"counts"`
		Due     []crmDeal      `json:"due"`
		Won     int            `json:"won"`
		Lost    int            `json:"lost"`
		Created int            `json:"created"`
		URL     string         `json:"url"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := CRMDay{Date: date, Activities: []CRMActivity{}, Counts: map[string]int{}, Due: []CRMDue{},
		Won: page.Won, Lost: page.Lost, Created: page.Created, URL: strings.TrimSpace(page.URL)}
	for _, a := range page.Activities {
		x := CRMActivity{ID: a.ID, DealID: a.DealID, Kind: strings.TrimSpace(a.Kind), Title: clipWords(a.Title, 120),
			Deal: firstNonEmptyStr(strings.TrimSpace(a.Deal), "a deal"), URL: strings.TrimSpace(a.DealURL)}
		if x.Kind == "note" || x.Kind == "call" || x.Kind == "meeting" || x.Kind == "email" || x.Kind == "step" {
			x.Note = clipWords(a.Body, 120)
		}
		if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(a.At)); err == nil {
			x.Time = LocalClock(t, loc)
		}
		d.Activities = append(d.Activities, x)
	}
	for k, n := range page.Counts {
		d.Counts[k] = n
	}
	if len(d.Counts) == 0 {
		for _, a := range d.Activities {
			d.Counts[a.Kind]++
		}
	}
	d.Won, d.Lost, d.Created = max(d.Won, d.Counts["won"]), max(d.Lost, d.Counts["lost"]), max(d.Created, d.Counts["created"])
	for _, x := range page.Due {
		d.Due = append(d.Due, CRMDue{ID: x.ID, Title: firstNonEmptyStr(strings.TrimSpace(x.Title), "a deal"), Company: strings.TrimSpace(x.Company),
			Contact: strings.TrimSpace(x.ContactName), NextAction: clipWords(x.NextAction, 120), Stage: x.StageName,
			Amount: x.Amount, Currency: x.Currency, URL: strings.TrimSpace(x.URL)})
	}
	// the company of the deals added, won or lost, for "Won: Online shop (Pekara Marić)";
	// a deal that cannot be read keeps its title alone
	company := map[int64]string{}
	looked := 0
	for i, a := range d.Activities {
		if a.Kind != "won" && a.Kind != "lost" && a.Kind != "created" {
			continue
		}
		co, ok := company[a.DealID]
		if !ok && looked < 12 && a.DealID > 0 {
			looked++
			var one struct {
				Deal crmDeal `json:"deal"`
			}
			if err := cl.Get(ctx, fmt.Sprintf("/api/v1/deals/%d", a.DealID), nil, &one); err == nil {
				co = strings.TrimSpace(one.Deal.Company)
			}
			company[a.DealID] = co
		}
		d.Activities[i].Company = co
	}
	return &Result{Payload: d, Headline: float64(len(d.Activities)), Summary: crmSummary(d)}, nil
}

// crmName is a deal's title with its company: "Online shop for cakes (Pekara Marić)".
func crmName(a CRMActivity) string {
	if a.Company != "" && !strings.Contains(a.Deal, a.Company) {
		return a.Deal + " (" + a.Company + ")"
	}
	return a.Deal
}

// crmList: "A, B, C and 2 more" — each deal once, in the order it came.
func crmList(names []string) string {
	out := uniq(names)
	if len(out) > 3 {
		return strings.Join(out[:3], ", ") + " and " + fmt.Sprintf("%d more", len(out)-3)
	}
	return strings.Join(out, ", ")
}

// crmSummary: "Won: Online shop for cakes (Pekara Marić); added: Booking site;
// call — Booking site; 2 steps done; 3 next steps were due". What happened to
// which deal, never what was written on it.
func crmSummary(d CRMDay) string {
	by := map[string][]string{}
	for _, a := range d.Activities {
		by[a.Kind] = append(by[a.Kind], crmName(a))
	}
	var parts []string
	if len(by["won"]) > 0 {
		parts = append(parts, "won: "+crmList(by["won"]))
	}
	if len(by["lost"]) > 0 {
		parts = append(parts, "lost: "+crmList(by["lost"]))
	}
	if len(by["created"]) > 0 {
		parts = append(parts, "added: "+crmList(by["created"]))
	}
	// the talks: "call — Booking site" for one, "2 calls, 1 meeting: Booking site, Logo" for more
	var talks, deals []string
	n := 0
	for _, k := range []struct{ kind, one, many string }{{"call", "call", "calls"}, {"meeting", "meeting", "meetings"}, {"email", "email", "emails"}} {
		if c := len(by[k.kind]); c > 0 {
			n += c
			talks = append(talks, Plural(c, k.one, k.many))
			deals = append(deals, by[k.kind]...)
		}
	}
	switch {
	case n == 1:
		for _, k := range []string{"call", "meeting", "email"} {
			if len(by[k]) == 1 {
				parts = append(parts, k+" — "+by[k][0])
			}
		}
	case n > 1:
		parts = append(parts, strings.Join(talks, ", ")+": "+crmList(deals))
	}
	if c := len(by["step"]); c > 0 {
		parts = append(parts, Plural(c, "step", "steps")+" done")
	}
	if c := len(by["stage"]); c > 0 {
		parts = append(parts, Plural(len(uniq(by["stage"])), "deal", "deals")+" moved on")
	}
	if c := len(by["note"]); c > 0 {
		parts = append(parts, Plural(c, "note", "notes"))
	}
	switch c := len(d.Due); {
	case c == 1:
		parts = append(parts, "1 next step was due")
	case c > 1:
		parts = append(parts, fmt.Sprintf("%d next steps were due", c))
	}
	s := strings.Join(parts, "; ")
	if s != "" {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
