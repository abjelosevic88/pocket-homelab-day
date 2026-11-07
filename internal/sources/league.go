package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket League ----------------

// League reads the day in Pocket League: the games planned that day in your
// groups with your answer (in, maybe, out or not yet) and how many are in,
// and the results you played that day — won, drawn or lost — each linking to
// the game in Pocket League. The groups' chat stays in Pocket League.
type League struct{ Conn pocket.Conn }

// NewLeague creates a fetcher over a connection.
func NewLeague(conn pocket.Conn) *League { return &League{Conn: conn} }

// Name implements Fetcher.
func (l *League) Name() string { return "league" }

// Test implements Fetcher.
func (l *League) Test(ctx context.Context) (string, error) {
	cl, err := l.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket League " + v, nil
}

// LeagueGame is a game planned that day in one of your groups.
type LeagueGame struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Group    string `json:"group"`
	Emoji    string `json:"emoji,omitempty"`
	Time     string `json:"time,omitempty"` // HH:MM
	Venue    string `json:"venue,omitempty"`
	Status   string `json:"status"`              // planned | voting | cancelled
	MyAnswer string `json:"my_answer,omitempty"` // in | maybe | out | "" (not answered)
	In       int    `json:"in"`                  // how many are in
	URL      string `json:"url,omitempty"`       // the game in Pocket League
}

// LeagueMatch is a result you played that day.
type LeagueMatch struct {
	ID      int64  `json:"id"`
	Group   string `json:"group"`
	Emoji   string `json:"emoji,omitempty"`
	Text    string `json:"text"`              // "Aleksandar beat Marko 6–4 6–3"
	Outcome string `json:"outcome,omitempty"` // W | D | L
	URL     string `json:"url,omitempty"`     // its game, else the group's results
	Time    string `json:"time,omitempty"`    // the time of its game that day, when it has one
}

// LeagueDay is the day in Pocket League.
type LeagueDay struct {
	Date    string        `json:"date"`
	Games   []LeagueGame  `json:"games"`
	Matches []LeagueMatch `json:"matches"`
	Won     int           `json:"won"`
	Drawn   int           `json:"drawn"`
	Lost    int           `json:"lost"`
}

// Fetch implements Fetcher.
func (l *League) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := l.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Games   []LeagueGame  `json:"games"`
		Matches []LeagueMatch `json:"matches"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := LeagueDay{Date: date, Games: []LeagueGame{}, Matches: []LeagueMatch{}}
	at := map[string]string{} // a game's link → its time, to put its result there
	for _, g := range page.Games {
		g.Title = clipWords(strings.TrimSpace(g.Title), 120)
		g.Group = strings.TrimSpace(g.Group)
		g.Title = firstNonEmptyStr(g.Title, g.Group, "A game")
		g.Venue = strings.TrimSpace(g.Venue)
		g.Status = strings.TrimSpace(g.Status)
		g.MyAnswer = strings.TrimSpace(g.MyAnswer)
		g.URL = strings.TrimSpace(g.URL)
		if len(g.Time) > 5 {
			g.Time = g.Time[:5]
		}
		if g.URL != "" && g.Time != "" {
			at[g.URL] = g.Time
		}
		d.Games = append(d.Games, g)
	}
	for _, m := range page.Matches {
		m.Group = firstNonEmptyStr(strings.TrimSpace(m.Group), "a group")
		m.Text = clipWords(strings.TrimSpace(m.Text), 160)
		m.Outcome = strings.ToUpper(strings.TrimSpace(m.Outcome))
		m.URL = strings.TrimSpace(m.URL)
		m.Time = at[m.URL]
		switch m.Outcome {
		case "W":
			d.Won++
		case "D":
			d.Drawn++
		case "L":
			d.Lost++
		}
		d.Matches = append(d.Matches, m)
	}
	return &Result{Payload: d, Headline: float64(leagueHeadline(d)), Summary: leagueSummary(d)}, nil
}

// leagueHeadline: the results played, and the games you were in that have no result.
func leagueHeadline(d LeagueDay) int {
	n := len(d.Matches)
	for _, g := range leagueOpen(d) {
		if g.MyAnswer == "in" {
			n++
		}
	}
	return n
}

// leagueOpen is the day's games without a result, that were not cancelled.
func leagueOpen(d LeagueDay) []LeagueGame {
	played := map[string]bool{}
	for _, m := range d.Matches {
		if m.URL != "" {
			played[m.URL] = true
		}
	}
	var out []LeagueGame
	for _, g := range d.Games {
		if g.Status != "cancelled" && (g.URL == "" || !played[g.URL]) {
			out = append(out, g)
		}
	}
	return out
}

// LeagueGameName is a game by its group: "Tuesday Tennis", or "Cup final (Tuesday Tennis)".
func LeagueGameName(g LeagueGame) string {
	if g.Group == "" || strings.Contains(g.Title, g.Group) {
		if g.Group != "" {
			return g.Group
		}
		return g.Title
	}
	return g.Title + " (" + g.Group + ")"
}

// LeagueAnswer says your answer to a game: "you're in", "maybe", "you're out", "not answered".
func LeagueAnswer(a string) string {
	switch a {
	case "in":
		return "you're in"
	case "maybe":
		return "maybe"
	case "out":
		return "you're out"
	}
	return "not answered"
}

func leagueOutcome(o string) string {
	switch o {
	case "W":
		return "won"
	case "D":
		return "drew"
	case "L":
		return "lost"
	}
	return "played"
}

// leagueSummary: "Won in Tuesday Tennis: Aleksandar beat Marko 6–4 6–3; Friday
// 5-a-side at 19:00, Dvorana Mojmilo (you're in)". The results you played and
// the games you were in or might be in; the games you said no to stay in the card.
func leagueSummary(d LeagueDay) string {
	var parts []string
	switch n := len(d.Matches); {
	case n == 1:
		m := d.Matches[0]
		s := cap1(leagueOutcome(m.Outcome)) + " in " + m.Group
		if m.Text != "" {
			s += ": " + m.Text
		}
		parts = append(parts, s)
	case n > 1:
		var groups []string
		for _, m := range d.Matches {
			groups = append(groups, m.Group)
		}
		var rec []string
		if d.Won > 0 {
			rec = append(rec, fmt.Sprintf("%d won", d.Won))
		}
		if d.Drawn > 0 {
			rec = append(rec, fmt.Sprintf("%d drawn", d.Drawn))
		}
		if d.Lost > 0 {
			rec = append(rec, fmt.Sprintf("%d lost", d.Lost))
		}
		s := fmt.Sprintf("%d results in %s", n, crmList(groups))
		if len(rec) > 0 {
			s += ": " + strings.Join(rec, ", ")
		}
		parts = append(parts, s)
	}
	var games []string
	for _, g := range leagueOpen(d) {
		if g.MyAnswer != "in" && g.MyAnswer != "maybe" {
			continue
		}
		s := LeagueGameName(g)
		if g.Time != "" {
			s += " at " + g.Time
		}
		if g.Venue != "" {
			s += ", " + g.Venue
		}
		games = append(games, s+" ("+LeagueAnswer(g.MyAnswer)+")")
	}
	if len(games) > 3 {
		games = append(games[:3], fmt.Sprintf("%d more games", len(games)-3))
	}
	parts = append(parts, games...)
	for _, g := range d.Games {
		if g.Status == "cancelled" && g.MyAnswer == "in" {
			parts = append(parts, LeagueGameName(g)+" was called off")
		}
	}
	return cap1(strings.Join(parts, "; "))
}

// cap1 capitalises the first letter.
func cap1(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
