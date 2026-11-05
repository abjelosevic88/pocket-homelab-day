package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// Pocket Weather's day becomes Day's weather payload: Day's condition words
// (from the WMO code, else Pocket Weather's word), the place where the day
// was (lat/lon asked for), the note kept, and today's warnings for today only.
func TestPocketWeatherDay(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/day/" + today:
			gotQuery = r.URL.RawQuery
			_, _ = w.Write([]byte(`{"date":"` + today + `","source":"forecast","day":{"date":"` + today + `","code":null,"condition":"drizzle","t_max":8.04,"t_min":2.96,"precip":3.2,"wind":31.5,"sunshine_h":null,"daylight_h":9.8},"text":"Drizzle, 3 °C to 8 °C, 3 mm","note":"Umbrella day","location":null}`))
		case "/api/v1/warnings":
			_, _ = w.Write([]byte(`{"active":[{"id":1,"location_id":1,"kind":"wind","level":"warning","day":"` + today + `","starts_at":"` + today + `T14:00","ends_at":"` + today + `T22:00","title":"Strong wind","detail":"Gusts to 80 km/h","place":"Mostar"},{"id":2,"kind":"heat","level":"advisory","day":"2099-01-01","title":"Not today"}],"today":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	p := NewPocketWeather(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := p.FetchAt(context.Background(), today, 43.3438, 17.8078, "Mostar")
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "lat=43.3438&lon=17.8078" {
		t.Fatalf("query %q", gotQuery)
	}
	d := res.Payload.(*Day)
	if d.Code != -1 || d.Condition != "rain" || *d.TMax != 8 || *d.TMin != 3 || *d.PrecipMM != 3.2 || *d.WindMax != 31.5 || d.SunshineH != nil || d.Place != "Mostar" || d.Lat != 43.3438 || d.Note != "Umbrella day" || d.From != "pocket" || d.Diary {
		t.Fatalf("day %+v", d)
	}
	if len(d.Warnings) != 1 || d.Warnings[0].Title != "Strong wind" || d.Warnings[0].From != "14:00" || d.Warnings[0].Until != "22:00" {
		t.Fatalf("warnings %+v", d.Warnings)
	}
	if want := "Warning: Strong wind; Rain, 3–8 °C, 3 mm"; res.Summary != want {
		t.Fatalf("summary %q want %q", res.Summary, want)
	}
}

func TestPocketWeatherConditions(t *testing.T) {
	code := func(c int) *int { return &c }
	for _, c := range []struct {
		code *int
		cond string
		want string
	}{
		{code(0), "clear", "clear"},
		{code(1), "mostly-clear", "partly"},
		{code(48), "freezing", "fog"},
		{code(56), "freezing", "rain"},
		{code(95), "storm", "storm"},
		{nil, "mostly-clear", "partly"},
		{nil, "freezing", "rain"},
		{nil, "snow", "snow"},
	} {
		var a weatherDayAnswer
		a.Day.Code, a.Day.Condition = c.code, c.cond
		if d := weatherFromPocket(a, 0, 0, ""); d == nil || d.Condition != c.want {
			t.Errorf("%v %s: %+v, want %s", c.code, c.cond, d, c.want)
		}
	}
}
