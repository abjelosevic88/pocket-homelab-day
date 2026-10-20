package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Documents ----------------

// Documents reads the documents added (scanned, mailed in, uploaded) on the
// day from Pocket Documents. It replaced Paperless-ngx; the days frozen while
// Day read Paperless were renamed to this source by migration 0005.
type Documents struct{ Conn pocket.Conn }

// NewDocuments creates a fetcher over a connection (pocket.Fixed for a URL and token).
func NewDocuments(conn pocket.Conn) *Documents { return &Documents{Conn: conn} }

// Name implements Fetcher.
func (p *Documents) Name() string { return "documents" }

// Test implements Fetcher.
func (p *Documents) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Documents " + v, nil
}

// documentsPage is how many documents one list call asks for (the API's maximum).
const documentsPage = 500

// Fetch implements Fetcher.
func (p *Documents) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	from, to, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	type doc struct {
		ID            int64  `json:"id"`
		Title         string `json:"title"`
		OriginalName  string `json:"original_name"`
		AddedAt       string `json:"added_at"`
		Source        string `json:"source"`
		Correspondent string `json:"correspondent_name"`
		DocumentType  string `json:"document_type_name"`
	}
	// added_at is stored in UTC and the filter takes whole UTC dates, so ask
	// for every UTC date the local day touches and keep the local day below.
	q := url.Values{
		"added_from": {from.UTC().Format("2006-01-02")},
		"added_to":   {to.Add(-time.Nanosecond).UTC().Format("2006-01-02")},
		"sort":       {"added"},
		"limit":      {strconv.Itoa(documentsPage)},
		"no_facets":  {"1"},
	}
	var docs []doc
	for page := 0; page < 20; page++ {
		var res struct {
			Results []doc `json:"results"`
			Count   int   `json:"count"`
		}
		q.Set("offset", strconv.Itoa(page*documentsPage))
		if err := cl.Get(ctx, "/api/v1/documents", q, &res); err != nil {
			return nil, err
		}
		docs = append(docs, res.Results...)
		if len(res.Results) < documentsPage || len(docs) >= res.Count {
			break
		}
	}
	act := Activity{}
	for _, d := range docs {
		at, ok := ParseAny(d.AddedAt)
		if !ok || at.Before(from) || !at.Before(to) {
			continue
		}
		// Brought over from Paperless or restored from an export: added to
		// Pocket Documents that day, but not something filed that day.
		if d.Source == "paperless" || d.Source == "import" {
			continue
		}
		title := d.Title
		if title == "" {
			title = d.OriginalName
		}
		act.Items = append(act.Items, ActivityItem{
			Time:  LocalClock(at, loc),
			Title: title,
			Sub:   JoinNonEmpty(" · ", d.Correspondent, d.DocumentType),
			Group: d.DocumentType,
			Ref:   strconv.FormatInt(d.ID, 10),
			// relative: the day page puts it under where Pocket Documents lives
			URL: fmt.Sprintf("/documents/%d", d.ID),
		})
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		sum = Plural(act.Count, "document", "documents") + " · " + act.Items[0].Title
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}
