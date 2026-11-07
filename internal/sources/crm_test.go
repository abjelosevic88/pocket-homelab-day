package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestCRMDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/day/2026-11-07":
			_, _ = w.Write([]byte(`{"date":"2026-11-07","activities":[
				{"id":1,"deal_id":3,"kind":"created","title":"Deal added","body":"","at":"2026-11-07T08:05:00Z","deal":"Booking site","deal_url":"https://crm.example.org/deals/3"},
				{"id":2,"deal_id":2,"kind":"won","title":"Won 🎉","body":"","at":"2026-11-07T09:15:00.123Z","deal":"Online shop for cakes","deal_url":"https://crm.example.org/deals/2"},
				{"id":3,"deal_id":3,"kind":"meeting","title":"First meeting","body":"A long note about the rooms, the prices, the booking calendar, the photos they still have to take and the second language they may want later on","at":"2026-11-07T10:00:00Z","deal":"Booking site","deal_url":"https://crm.example.org/deals/3"},
				{"id":4,"deal_id":5,"kind":"email","title":"Sent the price","body":"","at":"2026-11-07T12:00:00Z","deal":"Logo","deal_url":"https://crm.example.org/deals/5"},
				{"id":5,"deal_id":5,"kind":"stage","title":"Moved from Lead to Proposal sent","body":"","at":"2026-11-07T12:01:00Z","deal":"Logo","deal_url":"https://crm.example.org/deals/5"},
				{"id":6,"deal_id":9,"kind":"lost","title":"Lost","body":"Too expensive","at":"2026-11-07T16:00:00Z","deal":"","deal_url":""}],
				"counts":{"created":1,"won":1,"meeting":1,"email":1,"stage":1,"lost":1},
				"due":[{"id":5,"title":"Logo","company":"Studio A","contact_name":"Ana","next_action":"Call back","next_due":"2026-11-07","stage_name":"Proposal sent","amount":800,"currency":"EUR","url":"https://crm.example.org/deals/5"}],
				"won":1,"lost":1,"created":1,"url":"https://crm.example.org/"}`))
		case "/api/v1/deals/2":
			_, _ = w.Write([]byte(`{"deal":{"id":2,"title":"Online shop for cakes","company":"Pekara Marić"}}`))
		case "/api/v1/deals/3":
			_, _ = w.Write([]byte(`{"deal":{"id":3,"title":"Booking site","company":""}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := NewCRM(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	res, err := c.Fetch(context.Background(), "2026-11-07", loc)
	if err != nil {
		t.Fatal(err)
	}
	want := "Won: Online shop for cakes (Pekara Marić); lost: a deal; added: Booking site; 1 meeting, 1 email: Booking site, Logo; 1 deal moved on; 1 next step was due"
	if res.Headline != 6 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(CRMDay)
	if a := d.Activities[1]; a.Time != "10:15" || a.Company != "Pekara Marić" || a.URL != "https://crm.example.org/deals/2" {
		t.Fatalf("won line %+v", a)
	}
	if a := d.Activities[2]; len([]rune(a.Note)) > 121 || a.Note == "" {
		t.Fatalf("a talk's note is clipped: %q", a.Note)
	}
	if a := d.Activities[5]; a.Note != "" || a.Deal != "a deal" {
		t.Fatalf("a lost line keeps no note: %+v", a)
	}
	if x := d.Due[0]; x.NextAction != "Call back" || x.Company != "Studio A" || x.URL == "" {
		t.Fatalf("due %+v", x)
	}
	if d.Won != 1 || d.Lost != 1 || d.Created != 1 || d.URL != "https://crm.example.org/" {
		t.Fatalf("day %+v", d)
	}
}

func TestCRMSummary(t *testing.T) {
	act := func(kind string, deals ...string) []CRMActivity {
		var out []CRMActivity
		for i, d := range deals {
			out = append(out, CRMActivity{ID: int64(i + 1), Kind: kind, Deal: d})
		}
		return out
	}
	for _, c := range []struct {
		d    CRMDay
		want string
	}{
		{CRMDay{}, ""},
		{CRMDay{Activities: act("call", "Booking site")}, "Call — Booking site"},
		{CRMDay{Activities: append(act("call", "A", "B"), act("step", "A")...)}, "2 calls: A, B; 1 step done"},
		{CRMDay{Activities: act("won", "A", "B", "C", "D", "E")}, "Won: A, B, C and 2 more"},
		{CRMDay{Activities: act("note", "A", "A")}, "2 notes"},
		{CRMDay{Due: []CRMDue{{Title: "A"}, {Title: "B"}, {Title: "C"}}}, "3 next steps were due"},
	} {
		if got := crmSummary(c.d); got != c.want {
			t.Errorf("%+v: got %q want %q", c.d, got, c.want)
		}
	}
}

func TestCRMEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-05","activities":[],"counts":{},"due":[],"won":0,"lost":0,"created":0,"url":"https://crm.example.org/"}`))
	}))
	defer ts.Close()
	c := NewCRM(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := c.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(CRMDay); res.Headline != 0 || res.Summary != "" || d.Activities == nil || d.Due == nil {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}
