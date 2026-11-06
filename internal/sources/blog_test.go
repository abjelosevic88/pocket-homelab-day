package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestBlogDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-06" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2026-11-06",
			"published":[{"id":12,"title":"Spring in Sarajevo","kind":"post","status":"published","url":"https://blog.example.org/spring-in-sarajevo","words":1240,"excerpt":"The river was loud. ","tags":["travel"],"blog":"Notes","time":"09:30"}],
			"written":[{"id":13,"title":"  ","kind":"post","status":"draft","url":"https://blog.example.org/p/13?preview=k","words":80,"excerpt":"","tags":[],"blog":"Notes","time":"21:10"}],
			"views":312,"comments":4}`))
	}))
	defer ts.Close()
	b := NewBlog(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := b.Fetch(context.Background(), "2026-11-06", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Published “Spring in Sarajevo” (1,240 words); Wrote “Untitled” (a draft); 312 readers, 4 comments on the blog"; res.Headline != 1 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(BlogDay)
	if p := d.Published[0]; p.URL != "https://blog.example.org/spring-in-sarajevo" || p.Time != "09:30" || p.Excerpt != "The river was loud." || p.Blog != "Notes" || len(p.Tags) != 1 {
		t.Fatalf("published %+v", p)
	}
	if w := d.Written[0]; w.Title != "Untitled" || w.Status != "draft" || w.Time != "21:10" {
		t.Fatalf("written %+v", w)
	}
	if d.Views != 312 || d.Comments != 4 {
		t.Fatalf("day %+v", d)
	}
}

func TestBlogSummary(t *testing.T) {
	ps := func(titles ...string) []BlogPost {
		var out []BlogPost
		for i, s := range titles {
			out = append(out, BlogPost{ID: int64(i + 1), Title: s, Kind: "post"})
		}
		return out
	}
	for _, c := range []struct {
		d    BlogDay
		want string
	}{
		{BlogDay{Views: 40}, ""},
		{BlogDay{Comments: 1, Views: 1}, "1 reader, 1 comment on the blog"},
		{BlogDay{Published: []BlogPost{{Title: "About", Kind: "page"}}}, "Published the page “About”"},
		{BlogDay{Published: ps("A", "B", "C", "D", "E")}, "Published 5 posts: “A”, “B”, “C” and 2 more"},
		{BlogDay{Written: ps("A", "B")}, "Wrote 2 drafts: “A”, “B”"},
	} {
		if got := blogSummary(c.d); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}

func TestBlogEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-05","published":[],"written":[],"views":0,"comments":0}`))
	}))
	defer ts.Close()
	b := NewBlog(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := b.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(BlogDay); res.Headline != 0 || res.Summary != "" || d.Published == nil || len(d.Written) != 0 {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}
