package sources

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// Projects reads the person's work on a day from Pocket Projects
// (GET /api/v1/day/{date}): one item per issue they worked on — its title,
// project, a summary of what happened ("moved to In Review · 2 comments ·
// 1 commit · 1 h 20 min tracked") and the events behind it — plus the
// project updates they posted. Every item opens in Pocket Projects.
type Projects struct{ Conn pocket.Conn }

// NewProjects creates a client over a connection (a Pocket Core link or a token).
func NewProjects(conn pocket.Conn) *Projects { return &Projects{Conn: conn} }

// Name implements Fetcher.
func (p *Projects) Name() string { return "projects" }

// Test implements Fetcher: today's work, the same call a pull makes.
func (p *Projects) Test(ctx context.Context) (string, error) {
	c, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	var d projectsDay
	if err := c.Get(ctx, "/api/v1/day/"+time.Now().Format("2006-01-02"), nil, &d); err != nil {
		return "", err
	}
	return "Pocket Projects reachable", nil
}

type projectsEvent struct {
	Time string `json:"time"`
	Kind string `json:"kind"`
	Text string `json:"text"`
	URL  string `json:"url"`
}

type projectsDay struct {
	Issues []struct {
		Identifier string          `json:"identifier"`
		Title      string          `json:"title"`
		URL        string          `json:"url"`
		Team       string          `json:"team"`
		Project    string          `json:"project"`
		Status     string          `json:"status"`
		Done       bool            `json:"done"`
		Minutes    int             `json:"minutes"`
		First      string          `json:"first"`
		Last       string          `json:"last"`
		Summary    string          `json:"summary"`
		Events     []projectsEvent `json:"events"`
	} `json:"issues"`
	Projects []struct {
		Name   string          `json:"name"`
		URL    string          `json:"url"`
		Events []projectsEvent `json:"events"`
	} `json:"projects"`
}

// Fetch implements Fetcher.
func (p *Projects) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	if _, _, err := DayBounds(date, loc); err != nil {
		return nil, err
	}
	c, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var d projectsDay
	if err := c.Get(ctx, "/api/v1/day/"+date, url.Values{"tz": {loc.String()}}, &d); err != nil {
		return nil, err
	}
	act := projectsActivity(d)
	return &Result{Payload: act, Headline: float64(act.Count), Summary: projectsSummary(act)}, nil
}

func projectsActivity(d projectsDay) Activity {
	act := Activity{}
	events := func(in []projectsEvent) []ActivityEvent {
		out := make([]ActivityEvent, 0, len(in))
		for _, e := range in {
			out = append(out, ActivityEvent{Time: e.Time, Kind: e.Kind, Text: e.Text, URL: e.URL})
		}
		return out
	}
	for _, is := range d.Issues {
		group := firstNonEmptyStr(is.Project, is.Team)
		it := ActivityItem{Time: is.First, End: is.Last, Title: is.Identifier + " · " + is.Title, Sub: group, Group: group,
			Minutes: float64(is.Minutes), Done: is.Done, Ref: is.Identifier, URL: is.URL, Note: is.Summary, Events: events(is.Events)}
		if it.End == it.Time {
			it.End = ""
		}
		act.Items = append(act.Items, it)
	}
	for _, pr := range d.Projects {
		it := ActivityItem{Title: "Project update · " + pr.Name, Sub: pr.Name, Group: pr.Name, Ref: "project:" + pr.Name, URL: pr.URL, Events: events(pr.Events)}
		if len(pr.Events) > 0 {
			it.Time = pr.Events[0].Time
			it.Note = strings.TrimPrefix(pr.Events[0].Text, "Posted an update ")
		}
		act.Items = append(act.Items, it)
	}
	act.Finish()
	return act
}

// projectsSummary is the one line of the source: "4 issues, 2 done, 3 h 10 min tracked".
func projectsSummary(a Activity) string {
	if a.Count == 0 {
		return ""
	}
	s := Plural(a.Count, "issue", "issues")
	if a.Done > 0 {
		s += ", " + Plural(a.Done, "done", "done")
	}
	if a.Minutes >= 1 {
		s += ", " + HoursMinutes(int(a.Minutes+0.5)) + " tracked"
	}
	return s
}

func firstNonEmptyStr(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
