package sources

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Forms ----------------

// Forms reads the day in Pocket Forms: the answers people sent to your forms
// (who answered which form, with the answer's one-line preview, each linking
// to it in your Forms inbox) and the forms you opened for answers that day.
// Never the whole answers: Forms only gives the first line.
type Forms struct{ Conn pocket.Conn }

// NewForms creates a fetcher over a connection.
func NewForms(conn pocket.Conn) *Forms { return &Forms{Conn: conn} }

// Name implements Fetcher.
func (f *Forms) Name() string { return "forms" }

// Test implements Fetcher.
func (f *Forms) Test(ctx context.Context) (string, error) {
	cl, err := f.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Forms " + v, nil
}

// FormsAnswer is an answer that came in to one of your forms that day.
type FormsAnswer struct {
	ID      int64  `json:"id"`
	Time    string `json:"time,omitempty"` // HH:MM it came in
	Form    string `json:"form"`
	FormID  int64  `json:"form_id"`
	Emoji   string `json:"emoji,omitempty"`
	Name    string `json:"name"`              // who answered (their name, else their email, else "Answer #n")
	Preview string `json:"preview,omitempty"` // the answer's one line
	Number  int    `json:"number,omitempty"`
	URL     string `json:"url,omitempty"` // the answer in Pocket Forms
	Status  string `json:"status,omitempty"`
}

// FormsOpened is a form opened for answers that day.
type FormsOpened struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Emoji string `json:"emoji,omitempty"`
	URL   string `json:"url,omitempty"` // the form's public page
}

// FormsDay is the day in Pocket Forms.
type FormsDay struct {
	Date      string        `json:"date"`
	Count     int           `json:"count"`
	Responses []FormsAnswer `json:"responses"`
	Opened    []FormsOpened `json:"opened"`
}

// Fetch implements Fetcher.
func (f *Forms) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := f.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Count     int           `json:"count"`
		Responses []FormsAnswer `json:"responses"`
		Opened    []FormsOpened `json:"opened"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := FormsDay{Date: date, Responses: []FormsAnswer{}, Opened: []FormsOpened{}}
	for _, a := range page.Responses {
		if a.Status == "spam" {
			continue
		}
		a.Form = firstNonEmptyStr(strings.TrimSpace(a.Form), "a form")
		a.Name = firstNonEmptyStr(strings.TrimSpace(a.Name), "Someone")
		a.Preview = clipWords(a.Preview, 160)
		a.URL = strings.TrimSpace(a.URL)
		d.Responses = append(d.Responses, a)
	}
	for _, o := range page.Opened {
		o.Title = firstNonEmptyStr(strings.TrimSpace(o.Title), "Untitled form")
		o.URL = strings.TrimSpace(o.URL)
		d.Opened = append(d.Opened, o)
	}
	// the count is every answer of the day; the list stops at a couple of hundred
	d.Count = max(page.Count, len(d.Responses))
	return &Result{Payload: d, Headline: float64(d.Count), Summary: formsSummary(d)}, nil
}

// formsSummary: "3 answers: Ana Kovač and Marko to “Client brief”, Lejla to
// “RSVP”; opened “Summer party”". Who answered which form, never the answers.
func formsSummary(d FormsDay) string {
	var parts []string
	if d.Count > 0 {
		// the forms in the order their first answer came in, each with who answered
		var order []string
		who := map[string][]string{}
		for _, a := range d.Responses {
			if _, ok := who[a.Form]; !ok {
				order = append(order, a.Form)
			}
			if !slices.Contains(who[a.Form], a.Name) {
				who[a.Form] = append(who[a.Form], a.Name)
			}
		}
		var fs []string
		for i, form := range order {
			if i == 3 {
				fs[len(fs)-1] += " and " + Plural(len(order)-3, "more form", "more forms")
				break
			}
			names := who[form]
			n := strings.Join(names, ", ")
			switch {
			case len(names) > 3:
				n = strings.Join(names[:2], ", ") + " and " + Plural(len(names)-2, "other", "others")
			case len(names) > 1:
				n = strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
			}
			fs = append(fs, n+" to “"+form+"”")
		}
		s := Plural(d.Count, "answer", "answers")
		if len(fs) > 0 {
			s += ": " + strings.Join(fs, ", ")
		}
		parts = append(parts, s)
	}
	switch n := len(d.Opened); {
	case n == 1:
		parts = append(parts, "opened “"+d.Opened[0].Title+"”")
	case n > 1:
		var t []string
		for i, o := range d.Opened {
			if i == 3 {
				t[len(t)-1] += " and " + Plural(n-3, "more", "more")
				break
			}
			t = append(t, "“"+o.Title+"”")
		}
		parts = append(parts, "opened "+Plural(n, "form", "forms")+": "+strings.Join(t, ", "))
	}
	s := strings.Join(parts, "; ")
	if s != "" {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
