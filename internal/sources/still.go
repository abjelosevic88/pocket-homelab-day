package sources

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Still ----------------

// Still reads the day's breathing and meditation practice.
type Still struct{ Conn pocket.Conn }

// NewStill creates a fetcher over a connection.
func NewStill(conn pocket.Conn) *Still { return &Still{Conn: conn} }

// Name implements Fetcher.
func (s *Still) Name() string { return "still" }

// Test implements Fetcher.
func (s *Still) Test(ctx context.Context) (string, error) {
	cl, err := s.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Still " + v, nil
}

// StillPractice is one practice on the day.
type StillPractice struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	KindName   string  `json:"kind_name"`
	Title      string  `json:"title"`
	Time       string  `json:"time"`
	Minutes    float64 `json:"minutes"`
	CalmBefore *int    `json:"calm_before,omitempty"`
	CalmAfter  *int    `json:"calm_after,omitempty"`
	Lesson     string  `json:"lesson,omitempty"`
}

// StillLesson is a lesson finished that day.
type StillLesson struct {
	Program string `json:"program"`
	Stage   string `json:"stage"`
	Title   string `json:"title"`
}

// StillDay is the day in Pocket Still.
type StillDay struct {
	Minutes    float64         `json:"minutes"`
	Sessions   int             `json:"sessions"`
	Streak     int             `json:"streak"`
	GoalMin    int             `json:"goal_min"`
	GoalMet    bool            `json:"goal_met"`
	Practices  []StillPractice `json:"practices"`
	Lessons    []StillLesson   `json:"lessons"`
	Milestones []string        `json:"milestones"`
	Summary    string          `json:"summary"`
}

// Fetch implements Fetcher.
func (s *Still) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := s.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Minutes   float64 `json:"minutes"`
		Sessions  int     `json:"sessions"`
		Streak    int     `json:"streak"`
		GoalMin   int     `json:"goal_min"`
		GoalMet   bool    `json:"goal_met"`
		Practices []struct {
			ID         int64   `json:"id"`
			Kind       string  `json:"kind"`
			KindName   string  `json:"kind_name"`
			Title      string  `json:"title"`
			StartedAt  string  `json:"started_at"`
			Minutes    float64 `json:"minutes"`
			CalmBefore *int    `json:"calm_before"`
			CalmAfter  *int    `json:"calm_after"`
			Lesson     string  `json:"lesson"`
		} `json:"practices"`
		Lessons    []StillLesson `json:"lessons"`
		Milestones []string      `json:"milestones"`
		Summary    string        `json:"summary"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := StillDay{Minutes: page.Minutes, Sessions: page.Sessions, Streak: page.Streak, GoalMin: page.GoalMin, GoalMet: page.GoalMet,
		Practices: []StillPractice{}, Lessons: page.Lessons, Milestones: page.Milestones, Summary: page.Summary}
	if d.Lessons == nil {
		d.Lessons = []StillLesson{}
	}
	if d.Milestones == nil {
		d.Milestones = []string{}
	}
	for _, p := range page.Practices {
		t := ""
		if ts, ok := ParseAny(p.StartedAt); ok {
			t = LocalClock(ts, loc)
		}
		d.Practices = append(d.Practices, StillPractice{ID: p.ID, Kind: p.Kind, KindName: p.KindName, Title: p.Title, Time: t, Minutes: p.Minutes,
			CalmBefore: p.CalmBefore, CalmAfter: p.CalmAfter, Lesson: p.Lesson})
	}
	sum := d.Summary
	if sum == "" && d.Sessions > 0 {
		var names []string
		for _, p := range d.Practices {
			names = append(names, strings.ToLower(p.Title))
		}
		sum = fmt.Sprintf("%d min of practice: %s", int(math.Round(d.Minutes)), strings.Join(names, ", "))
	}
	return &Result{Payload: d, Headline: d.Minutes, Summary: sum}, nil
}
