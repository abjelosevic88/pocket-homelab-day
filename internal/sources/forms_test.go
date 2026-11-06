package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestFormsDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-07" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2026-11-07","count":3,"summary":"3 answers to your forms",
			"responses":[{"id":41,"time":"09:12","form":"Client brief","form_id":2,"emoji":"📋","name":"Ana Kovač","preview":"A new website for the bakery ","number":7,"url":"https://forms.example.org/answers/41","status":"new"},
				{"id":42,"time":"11:40","form":"Client brief","form_id":2,"emoji":"📋","name":"Marko","preview":"Logo and cards","number":8,"url":"https://forms.example.org/answers/42","status":"seen"},
				{"id":43,"time":"18:05","form":"  ","form_id":5,"emoji":"","name":"","preview":"","number":1,"url":"https://forms.example.org/answers/43","status":"new"}],
			"opened":[{"id":9,"title":"Summer party RSVP","emoji":"🎉","url":"https://forms.example.org/f/summer-party"}]}`))
	}))
	defer ts.Close()
	f := NewForms(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := f.Fetch(context.Background(), "2026-11-07", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "3 answers: Ana Kovač and Marko to “Client brief”, Someone to “a form”; opened “Summer party RSVP”"; res.Headline != 3 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(FormsDay)
	if a := d.Responses[0]; a.Time != "09:12" || a.Preview != "A new website for the bakery" || a.URL != "https://forms.example.org/answers/41" || a.FormID != 2 || a.Status != "new" || a.Emoji != "📋" {
		t.Fatalf("answer %+v", a)
	}
	if o := d.Opened[0]; o.ID != 9 || o.URL != "https://forms.example.org/f/summer-party" || o.Emoji != "🎉" {
		t.Fatalf("opened %+v", o)
	}
	if d.Date != "2026-11-07" || d.Count != 3 {
		t.Fatalf("day %+v", d)
	}
}

func TestFormsSummary(t *testing.T) {
	as := func(form string, names ...string) []FormsAnswer {
		var out []FormsAnswer
		for i, n := range names {
			out = append(out, FormsAnswer{ID: int64(i + 1), Form: form, Name: n})
		}
		return out
	}
	many := append(append(append(as("A", "x"), as("B", "y")...), as("C", "z")...), as("D", "w")...)
	for _, c := range []struct {
		d    FormsDay
		want string
	}{
		{FormsDay{}, ""},
		{FormsDay{Count: 1, Responses: as("RSVP", "Lejla")}, "1 answer: Lejla to “RSVP”"},
		{FormsDay{Count: 2, Responses: as("RSVP", "Lejla", "Lejla")}, "2 answers: Lejla to “RSVP”"},
		{FormsDay{Count: 5, Responses: as("Survey", "A", "B", "C", "D", "E")}, "5 answers: A, B and 3 others to “Survey”"},
		{FormsDay{Count: 4, Responses: many}, "4 answers: x to “A”, y to “B”, z to “C” and 1 more form"},
		{FormsDay{Opened: []FormsOpened{{Title: "A"}, {Title: "B"}}}, "Opened 2 forms: “A”, “B”"},
	} {
		if got := formsSummary(c.d); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}

func TestFormsEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-05","count":0,"summary":"","responses":[],"opened":[]}`))
	}))
	defer ts.Close()
	f := NewForms(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := f.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(FormsDay); res.Headline != 0 || res.Summary != "" || d.Responses == nil || d.Opened == nil {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}
