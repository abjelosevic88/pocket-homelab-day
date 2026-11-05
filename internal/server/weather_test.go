package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// fakeOpenMeteo points the direct weather at a fake Open-Meteo for the test
// and counts its calls.
func fakeOpenMeteo(t *testing.T) func() int {
	t.Helper()
	var mu sync.Mutex
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		mu.Unlock()
		d := r.URL.Query().Get("start_date")
		_, _ = rw.Write([]byte(`{"daily":{"time":["` + d + `"],"weather_code":[61],"temperature_2m_max":[18.2],"temperature_2m_min":[9.6],"precipitation_sum":[4.4],"sunshine_duration":[7200],"daylight_duration":[43200],"wind_speed_10m_max":[21]}}`))
	}))
	t.Cleanup(srv.Close)
	f, a := sources.OpenMeteoForecast, sources.OpenMeteoArchive
	sources.OpenMeteoForecast, sources.OpenMeteoArchive = srv.URL, srv.URL
	t.Cleanup(func() { sources.OpenMeteoForecast, sources.OpenMeteoArchive = f, a })
	return func() int { mu.Lock(); defer mu.Unlock(); return n }
}

func setHome(t *testing.T, e *linkEnv, u *store.User) {
	t.Helper()
	st, _ := e.st.GetSettings(context.Background(), u.ID)
	st.HomeLat, st.HomeLon, st.HomePlace = 43.86, 18.41, "Sarajevo"
	if err := e.st.SaveSettings(context.Background(), u.ID, st); err != nil {
		t.Fatal(err)
	}
}

func weatherFact(t *testing.T, e *linkEnv, u *store.User) (sources.Day, store.Fact) {
	t.Helper()
	f, ok := e.facts(u, day0)["weather"]
	if !ok {
		t.Fatal("no weather fact")
	}
	var d sources.Day
	if f.Error == "" {
		if err := json.Unmarshal(f.Payload, &d); err != nil {
			t.Fatal(err)
		}
	}
	return d, f
}

// With the Day → Pocket Weather link on, the weather comes from Pocket
// Weather (its home place, the diary note); with the link off, refused at
// fetch time, or no place in Pocket Weather, it comes straight from
// Open-Meteo for the home location in Day; a real error from Pocket Weather
// is the day's error.
func TestWeatherFromPocketWeather(t *testing.T) {
	e := newLinkEnv(t, true)
	ctx := context.Background()
	meteo := fakeOpenMeteo(t)
	var calls []string
	status := http.StatusOK
	wx := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		calls = append(calls, r.Header.Get("Authorization")+" "+r.URL.Path)
		code := status
		e.mu.Unlock()
		if code != http.StatusOK {
			rw.WriteHeader(code)
			_, _ = rw.Write([]byte(`{"error":"diary locked"}`))
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/day/") {
			http.NotFound(rw, r)
			return
		}
		_, _ = rw.Write([]byte(`{"date":"` + day0 + `","source":"diary","day":{"date":"` + day0 + `","code":2,"condition":"partly","text":"Partly cloudy","t_max":21.36,"t_min":11.04,"precip":0.2,"wind":14,"sunshine_h":9.1,"daylight_h":13.2},"text":"Partly cloudy, 11 °C to 21 °C","note":"Picnic weather at Vrelo Bosne","location":{"id":1,"name":"Ilidža","country":"BA","lat":43.83,"lon":18.31}}`))
	}))
	t.Cleanup(wx.Close)
	e.mu.Lock()
	e.targets[weatherApp.slug] = wx
	e.mu.Unlock()

	// No home in Day yet: Pocket Weather alone makes the source work.
	w := e.settings(e.ownerTok).Sources["weather"]
	if w.Weather == nil || w.Weather.Via != modeCore || w.Weather.Home || !w.Configured {
		t.Fatalf("weather state %+v %+v", w, w.Weather)
	}
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	d, f := weatherFact(t, e, e.owner)
	if d.From != "pocket" || d.Condition != "partly" || *d.TMax != 21.4 || d.Place != "Ilidža" || d.Lat != 43.83 || d.Note != "Picnic weather at Vrelo Bosne" || !d.Diary || *d.WindMax != 14 {
		t.Fatalf("pocket weather day %+v", d)
	}
	if f.Summary != "Partly cloudy, 11–21 °C" || f.Headline != 21.4 {
		t.Fatalf("fact %+v", f)
	}
	if len(calls) != 1 || calls[0] != "Bearer pcx_weather_weather:read /api/v1/day/"+day0 || meteo() != 0 {
		t.Fatalf("calls %v, open-meteo %d", calls, meteo())
	}
	e.mu.Lock()
	calls = nil
	e.mu.Unlock()
	if _, body := e.do(e.ownerTok, "POST", "/integrations/test", map[string]any{"which": "weather"}); !strings.Contains(body, "Pocket Weather answers") || !strings.Contains(body, "through Pocket Core") || len(calls) != 1 {
		t.Errorf("weather test %s %v", body, calls)
	}

	// A home in Day: Pocket Weather still answers, Open-Meteo stays behind it.
	setHome(t, e, e.owner)
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if d, _ := weatherFact(t, e, e.owner); d.From != "pocket" || meteo() != 0 {
		t.Fatalf("pocket weather with a home %+v, open-meteo %d", d, meteo())
	}

	// The link is refused when the day is fetched: Open-Meteo answers.
	e.mu.Lock()
	e.tokOff[weatherApp.slug+" "+weatherApp.scope] = true
	e.mu.Unlock()
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if d, f := weatherFact(t, e, e.owner); d.From != "" || d.Condition != "rain" || d.Place != "Sarajevo" || f.Summary != "Rain, 10–18 °C, 4 mm" || meteo() != 1 {
		t.Fatalf("fallback %+v %+v, open-meteo %d", d, f, meteo())
	}
	e.mu.Lock()
	delete(e.tokOff, weatherApp.slug+" "+weatherApp.scope)
	e.mu.Unlock()

	// A real error from Pocket Weather is the day's error, not Open-Meteo's weather.
	e.mu.Lock()
	status = http.StatusInternalServerError
	e.mu.Unlock()
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if _, f := weatherFact(t, e, e.owner); !strings.Contains(f.Error, "diary locked") || meteo() != 1 {
		t.Fatalf("error fact %+v, open-meteo %d", f, meteo())
	}

	// No place in Pocket Weather yet (404): Open-Meteo answers.
	e.mu.Lock()
	status = http.StatusNotFound
	e.mu.Unlock()
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if d, f := weatherFact(t, e, e.owner); d.From != "" || f.Error != "" || meteo() != 2 {
		t.Fatalf("404 fallback %+v %+v, open-meteo %d", d, f, meteo())
	}
	e.mu.Lock()
	status = http.StatusOK
	e.mu.Unlock()

	// The link switched off in Core: straight from Open-Meteo, no Pocket Weather calls.
	e.mu.Lock()
	e.off[weatherApp.slug+" "+weatherApp.scope] = "off_admin"
	n := len(calls)
	e.mu.Unlock()
	w = e.settings(e.ownerTok).Sources["weather"]
	if w.Weather.Via != "" || !w.Weather.Home || !strings.Contains(w.Weather.Detail, "Day → Pocket Weather link is switched off in Pocket Core") || !w.Configured {
		t.Fatalf("weather off %+v", w.Weather)
	}
	e.s.pullDay(ctx, e.owner.ID, day0, true)
	if d, _ := weatherFact(t, e, e.owner); d.From != "" || len(calls) != n || meteo() != 3 {
		t.Fatalf("direct %+v, %d pocket calls, open-meteo %d", d, len(calls)-n, meteo())
	}
}

// Without Pocket Core the weather is Open-Meteo's for the home location, as always.
func TestWeatherWithoutCore(t *testing.T) {
	e := newLinkEnv(t, false)
	meteo := fakeOpenMeteo(t)
	if w := e.settings(e.ownerTok).Sources["weather"]; w.Configured || w.Weather == nil || w.Weather.Via != "" {
		t.Fatalf("no home, no Pocket Core: %+v", w)
	}
	setHome(t, e, e.owner)
	if w := e.settings(e.ownerTok).Sources["weather"]; !w.Configured || !w.Weather.Home {
		t.Fatalf("home set: %+v", w)
	}
	e.s.pullDay(context.Background(), e.owner.ID, day0, true)
	if d, f := weatherFact(t, e, e.owner); d.From != "" || d.Place != "Sarajevo" || f.Summary != "Rain, 10–18 °C, 4 mm" || meteo() != 1 {
		t.Fatalf("direct %+v %+v", d, f)
	}
}
