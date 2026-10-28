package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// PocketRequests reads a day of requests from Pocket Requests (its day view,
// requests:read): the films, shows and albums the person asked for, and the
// ones that arrived. It fills the requests source ("jellyseerr"), so the
// days, the timeline, the year page and the briefs read it exactly like
// Jellyseerr's.
type PocketRequests struct{ Conn pocket.Conn }

func NewPocketRequests(c pocket.Conn) *PocketRequests { return &PocketRequests{Conn: c} }
func (p *PocketRequests) Name() string                { return "jellyseerr" }

func (p *PocketRequests) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var out struct {
		Total int `json:"total"`
	}
	if err := c.Get(ctx, "/api/v1/requests", url.Values{"limit": {"1"}}, &out); err != nil {
		return "", err
	}
	return "Pocket Requests answers: " + Plural(out.Total, "request", "requests"), nil
}

type requestsDayItem struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Year        int    `json:"year"`
	Poster      string `json:"poster"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	AvailableAt string `json:"available_at"`
	OpenURL     string `json:"open_url"`
}

// requestKindLabel is the group a request counts under, as Jellyseerr's.
func requestKindLabel(kind string) string {
	switch kind {
	case "series":
		return "series"
	case "album":
		return "album"
	}
	return "film"
}

func (p *PocketRequests) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	from, _, err := DayBounds(date, loc)
	if err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	_, off := from.Zone()
	var out struct {
		Requested []requestsDayItem `json:"requested"`
		Arrived   []requestsDayItem `json:"arrived"`
	}
	if err := c.Get(ctx, "/api/v1/day/"+date, url.Values{"tz": {strconv.Itoa(off / 60)}}, &out); err != nil {
		return nil, err
	}
	act := Activity{Provider: "requests"}
	item := func(r requestsDayItem, at, sub string, done bool, ref string) ActivityItem {
		label := requestKindLabel(r.Kind)
		it := ActivityItem{Title: r.Title, Sub: sub + " " + label, Group: label, Done: done, Ref: fmt.Sprintf("%s/%d", ref, r.ID), URL: r.OpenURL, Image: r.Poster}
		if done {
			it.Sub = "now available"
		}
		if r.Kind == "album" && r.Subtitle != "" {
			it.Sub += " · " + r.Subtitle
		}
		if t, ok := ParseAny(at); ok {
			it.Time = LocalClock(t, loc)
		}
		return it
	}
	for _, r := range out.Requested {
		it := item(r, r.CreatedAt, "requested", false, "req")
		if r.Status == "declined" {
			it.Sub += " · declined"
		}
		act.Items = append(act.Items, it)
	}
	for _, r := range out.Arrived {
		act.Items = append(act.Items, item(r, r.AvailableAt, "", true, "avail"))
	}
	act.Finish()
	sum := ""
	if act.Count > 0 {
		req, av := 0, 0
		for _, it := range act.Items {
			if it.Done {
				av++
			} else {
				req++
			}
		}
		parts := []string{}
		if req > 0 {
			parts = append(parts, Plural(req, "request", "requests"))
		}
		if av > 0 {
			parts = append(parts, fmt.Sprintf("%d now available", av))
		}
		sum = strings.Join(parts, " · ")
		if act.Items[0].Title != "" {
			sum += " · " + act.Items[0].Title
		}
	}
	return &Result{Payload: act, Headline: float64(act.Count), Summary: sum}, nil
}
