package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestLeagueDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-10" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2026-11-10","games":[
			{"id":31,"title":"Tennis · Tuesday Tennis","group":"Tuesday Tennis","emoji":"🎾","time":"20:00","venue":"TK Koševo","status":"planned","my_answer":"in","in":4,"url":"https://league.example.org/games/31"},
			{"id":35,"title":"Padel · Padel crew","group":"Padel crew","emoji":"🎾","time":"18:00","venue":"Padel Centar Stup","status":"planned","my_answer":"maybe","in":3,"url":"https://league.example.org/games/35"},
			{"id":36,"title":"Futsal / 5-a-side · Friday 5-a-side","group":"Friday 5-a-side","emoji":"⚽","time":"21:00","venue":"","status":"planned","my_answer":"out","in":9,"url":"https://league.example.org/games/36"},
			{"id":37,"title":"Cup final","group":"Friday 5-a-side","emoji":"⚽","time":"10:00","venue":"","status":"cancelled","my_answer":"in","in":2,"url":"https://league.example.org/games/37"}],
			"matches":[{"id":40,"group":"Tuesday Tennis","emoji":"🎾","text":"Aleksandar beat Marko Petrović 6–4 6–3","outcome":"W","url":"https://league.example.org/games/31"}]}`))
	}))
	defer ts.Close()
	l := NewLeague(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := l.Fetch(context.Background(), "2026-11-10", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	want := "Won in Tuesday Tennis: Aleksandar beat Marko Petrović 6–4 6–3; Padel crew at 18:00, Padel Centar Stup (maybe); Cup final (Friday 5-a-side) was called off"
	if res.Summary != want || res.Headline != 1 {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(LeagueDay)
	if len(d.Games) != 4 || len(d.Matches) != 1 || d.Won != 1 || d.Matches[0].Time != "20:00" {
		t.Fatalf("day %+v", d)
	}
}

func TestLeagueSummary(t *testing.T) {
	g := func(group, answer, tm string) LeagueGame {
		return LeagueGame{Title: "Tennis · " + group, Group: group, Time: tm, MyAnswer: answer, Status: "planned"}
	}
	for _, c := range []struct {
		d    LeagueDay
		want string
		head int
	}{
		{LeagueDay{}, "", 0},
		{LeagueDay{Games: []LeagueGame{g("Tuesday Tennis", "in", "20:00")}}, "Tuesday Tennis at 20:00 (you're in)", 1},
		{LeagueDay{Games: []LeagueGame{g("Tuesday Tennis", "", "20:00"), g("Padel crew", "out", "")}}, "", 0},
		{LeagueDay{Matches: []LeagueMatch{{Group: "Padel crew", Outcome: "W"}, {Group: "Padel crew", Outcome: "L"}, {Group: "Tuesday Tennis", Outcome: "W"}}, Won: 2, Lost: 1}, "3 results in Padel crew, Tuesday Tennis: 2 won, 1 lost", 3},
		{LeagueDay{Matches: []LeagueMatch{{Group: "Friday 5-a-side", Text: "Team Ana and Team Marko drew 3–3", Outcome: "D"}}, Drawn: 1}, "Drew in Friday 5-a-side: Team Ana and Team Marko drew 3–3", 1},
	} {
		if got := leagueSummary(c.d); got != c.want || leagueHeadline(c.d) != c.head {
			t.Errorf("%+v: got %q (%d) want %q (%d)", c.d, got, leagueHeadline(c.d), c.want, c.head)
		}
	}
}

func TestLeagueEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-03","games":[],"matches":[]}`))
	}))
	defer ts.Close()
	l := NewLeague(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := l.Fetch(context.Background(), "2026-11-03", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(LeagueDay); res.Headline != 0 || res.Summary != "" || d.Games == nil || d.Matches == nil {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}
