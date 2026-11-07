package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func trailsServer(t *testing.T, asked *[]string, nextIsOut bool) *httptest.Server {
	today := time.Now().In(time.UTC)
	d := func(n int) string { return today.AddDate(0, 0, n).Format("2006-01-02") }
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*asked = append(*asked, r.URL.Path+"?"+r.URL.RawQuery)
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/api/v1/summary":
			next := `{"id":21,"title":"Run by the river","date":"` + d(2) + `","start_time":"08:00","activity":"run","days":2,"url":"https://trails.example.org/outings/21"}`
			if nextIsOut {
				next = `{"id":20,"title":"Hike: Trebević","date":"` + d(0) + `","start_time":"08:00","activity":"hike","days":0,"url":"https://trails.example.org/outings/20"}`
			}
			_, _ = w.Write([]byte(`{"planned":3,"to_rate":1,"routes":7,
				"out":[{"id":20,"title":"","route_name":"Trebević","activity":"hike","date":"` + d(0) + `","start_time":"08:00","status":"out","back_by":"` + d(0) + `T17:30","people":[{"name":"Ana"}],"minutes":270}],
				"next":` + next + `}`))
		case r.URL.Path == "/api/v1/outings" && q.Get("status") == "planned":
			_, _ = w.Write([]byte(`[{"id":22,"title":"Bjelašnica ridge","activity":"hike","date":"` + d(5) + `","start_time":"07:30","status":"planned"}]`))
		case r.URL.Path == "/api/v1/outings" && q.Get("status") == "done":
			if q.Get("order") != "desc" || q.Get("to") != d(-1) || q.Get("from") != d(-14) {
				http.Error(w, "bad query "+r.URL.RawQuery, 400)
				return
			}
			_, _ = w.Write([]byte(`[{"id":15,"title":"Evening walk","activity":"walk","date":"` + d(-2) + `","status":"done","distance_m":5400,"gain_m":60,"minutes":75,"rating":4,"people":[]}]`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/day/"):
			_, _ = w.Write([]byte(`{"date":"x","count":4,"outings":[
				{"id":3,"title":"Run by the river","status":"planned","activity":"run","start_time":"18:00","distance_m":8000,"gain_m":20,"minutes":45,"rating":0,"people":"","text":"8.0 km · 20 m up · 45 min","route":"River loop","url":"https://trails.example.org/outings/3"},
				{"id":2,"title":"Hike: Trebević from Sarajevo","status":"done","activity":"hike","start_time":"08:00","distance_m":12400,"gain_m":980,"minutes":270,"rating":5,"people":"Ana","text":"12.4 km · 980 m up · 4 h 30 min · with Ana","route":"Trebević from Sarajevo","url":"https://trails.example.org/outings/2"},
				{"id":4,"title":"Ride to the lake","status":"cancelled","activity":"bike","start_time":"","text":"","url":""},
				{"id":5,"title":"","status":"done","activity":"walk","start_time":"","route":"Old town","text":"3.1 km · 40 m up · 50 min","url":"https://trails.example.org/outings/5"},
				{"id":6,"title":"Ski tour","status":"done","activity":"ski","start_time":"","text":"6.0 km · 700 m up","url":"https://trails.example.org/outings/6"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTrailsPastDay(t *testing.T) {
	var asked []string
	ts := trailsServer(t, &asked, false)
	defer ts.Close()
	tr := NewTrails(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := tr.Fetch(context.Background(), "2026-09-01", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := "Hike: Trebević from Sarajevo (12.4 km · 980 m up · 4 h 30 min · with Ana); Walk: Old town (3.1 km · 40 m up · 50 min); Ski tour (6.0 km · 700 m up) and 1 more"
	if res.Summary != want || res.Headline != 3 {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(TrailsDay)
	if d.Today != nil || len(asked) != 1 || len(d.Outings) != 4 || d.Outings[0].Time != "18:00" || d.Outings[1].People != "Ana" || d.Outings[1].URL != "https://trails.example.org/outings/2" {
		t.Fatalf("a past day reads only the day (the outing called off left out): %v %+v", asked, d)
	}
	if l := strings.Join(TrailsLines(d), " | "); !strings.HasPrefix(l, "Run by the river at 18:00 (planned) | Hike: Trebević from Sarajevo (12.4 km") {
		t.Errorf("lines %q", l)
	}
}

func TestTrailsToday(t *testing.T) {
	var asked []string
	ts := trailsServer(t, &asked, false)
	defer ts.Close()
	tr := NewTrails(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := tr.Fetch(context.Background(), time.Now().In(time.UTC).Format("2006-01-02"), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	td := res.Payload.(TrailsDay).Today
	if td == nil {
		t.Fatalf("today: %v", asked)
	}
	if len(td.Out) != 1 || td.Out[0].Title != "Hike: Trebević" || td.Out[0].BackBy == "" || td.ToRate != 1 {
		t.Errorf("out %+v", td)
	}
	if td.Next == nil || td.Next.ID != 21 || td.Next.Days != 2 {
		t.Errorf("next %+v", td.Next)
	}
	if len(td.Recent) != 1 || td.Recent[0].Text != "5.4 km · 60 m up · 1 h 15 min" || td.Recent[0].Rating != 4 {
		t.Errorf("recent %+v", td.Recent)
	}
	day := time.Now().In(time.UTC).AddDate(0, 0, 2).Format("Mon")
	if got := TrailsNow(td); got != "out now on Hike: Trebević (back by 17:30); next: Run by the river in 2 days ("+day+" 08:00)" {
		t.Errorf("now %q", got)
	}
	if TrailsNow(nil) != "" || TrailsNow(&TrailsToday{}) != "" {
		t.Error("nothing on")
	}
}

// Trails' next outing is the one you are out on: the next planned one
// within the week instead.
func TestTrailsTodayNextNotTheOneOut(t *testing.T) {
	var asked []string
	ts := trailsServer(t, &asked, true)
	defer ts.Close()
	tr := NewTrails(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := tr.Fetch(context.Background(), time.Now().In(time.UTC).Format("2006-01-02"), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	td := res.Payload.(TrailsDay).Today
	if td == nil || td.Next == nil || td.Next.ID != 22 || td.Next.Days != 5 || td.Next.Time != "07:30" {
		t.Fatalf("next %+v (%v)", td, asked)
	}
}
