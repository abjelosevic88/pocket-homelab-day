package sources

import (
	"context"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Blog ----------------

// Blog reads the day in Pocket Blog: the posts that went live that day (each
// linking to the post itself), the posts begun that day and not out yet, and
// how many readers and comments the blog had.
type Blog struct{ Conn pocket.Conn }

// NewBlog creates a fetcher over a connection.
func NewBlog(conn pocket.Conn) *Blog { return &Blog{Conn: conn} }

// Name implements Fetcher.
func (b *Blog) Name() string { return "blog" }

// Test implements Fetcher.
func (b *Blog) Test(ctx context.Context) (string, error) {
	cl, err := b.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Blog " + v, nil
}

// BlogPost is a post published or written that day.
type BlogPost struct {
	ID      int64    `json:"id"`
	Title   string   `json:"title"`
	Kind    string   `json:"kind,omitempty"`   // post | page
	Status  string   `json:"status,omitempty"` // draft | scheduled | published
	URL     string   `json:"url,omitempty"`    // the public post (a draft's carries ?preview=)
	Words   int      `json:"words,omitempty"`
	Excerpt string   `json:"excerpt,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Blog    string   `json:"blog,omitempty"`
	Time    string   `json:"time,omitempty"` // HH:MM it went live (published) or was begun (written)
}

// BlogDay is the day in Pocket Blog.
type BlogDay struct {
	Published []BlogPost `json:"published"`
	Written   []BlogPost `json:"written"`
	Views     int        `json:"views"`
	Comments  int        `json:"comments"`
}

// Fetch implements Fetcher.
func (b *Blog) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := b.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Published []BlogPost `json:"published"`
		Written   []BlogPost `json:"written"`
		Views     int        `json:"views"`
		Comments  int        `json:"comments"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	clean := func(in []BlogPost) []BlogPost {
		out := []BlogPost{}
		for _, p := range in {
			p.Title = firstNonEmptyStr(strings.TrimSpace(p.Title), "Untitled")
			p.URL = strings.TrimSpace(p.URL)
			p.Excerpt = strings.TrimSpace(p.Excerpt)
			out = append(out, p)
		}
		return out
	}
	d := BlogDay{Published: clean(page.Published), Written: clean(page.Written), Views: page.Views, Comments: page.Comments}
	return &Result{Payload: d, Headline: float64(len(d.Published)), Summary: blogSummary(d)}, nil
}

// blogSummary: "Published “Spring in Sarajevo” (1,240 words); 312 readers,
// 4 comments"; only drafts: "Wrote “Notes on rain” (a draft)". Readers alone
// do not make a line.
func blogSummary(d BlogDay) string {
	quote := func(ps []BlogPost, max int) string {
		var t []string
		for i, p := range ps {
			if i == max {
				return strings.Join(t, ", ") + " and " + Plural(len(ps)-max, "more", "more")
			}
			t = append(t, "“"+p.Title+"”")
		}
		return strings.Join(t, ", ")
	}
	var parts []string
	switch n := len(d.Published); {
	case n == 1:
		s := "Published “" + d.Published[0].Title + "”"
		if d.Published[0].Kind == "page" {
			s = "Published the page “" + d.Published[0].Title + "”"
		}
		if w := d.Published[0].Words; w > 0 {
			s += " (" + Thousands(w) + " words)"
		}
		parts = append(parts, s)
	case n > 1:
		parts = append(parts, "Published "+Plural(n, "post", "posts")+": "+quote(d.Published, 3))
	}
	if len(d.Written) > 0 {
		if len(d.Written) == 1 {
			parts = append(parts, "Wrote “"+d.Written[0].Title+"” (a draft)")
		} else {
			parts = append(parts, "Wrote "+Plural(len(d.Written), "draft", "drafts")+": "+quote(d.Written, 3))
		}
	}
	if len(parts) == 0 && d.Comments == 0 {
		return ""
	}
	var stats []string
	if d.Views > 0 {
		stats = append(stats, Plural(d.Views, "reader", "readers"))
	}
	if d.Comments > 0 {
		stats = append(stats, Plural(d.Comments, "comment", "comments"))
	}
	if len(stats) > 0 {
		parts = append(parts, strings.Join(stats, ", ")+" on the blog")
	}
	return strings.Join(parts, "; ")
}
