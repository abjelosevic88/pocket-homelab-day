package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Fitness ----------------

// Fitness reads the day in Pocket Fitness: the workouts the person finished,
// what they trained (each exercise with its sets in words), the records they
// set and their own notes. Pocket Health's runs and rides that Pocket Fitness
// also lists are left to the health source, which has them already.
type Fitness struct{ Conn pocket.Conn }

// NewFitness creates a fetcher over a connection.
func NewFitness(conn pocket.Conn) *Fitness { return &Fitness{Conn: conn} }

// Name implements Fetcher.
func (f *Fitness) Name() string { return "fitness" }

// Test implements Fetcher.
func (f *Fitness) Test(ctx context.Context) (string, error) {
	cl, err := f.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Fitness " + v, nil
}

// FitnessExercise is one exercise of a workout with its sets in words.
type FitnessExercise struct {
	ID      int64  `json:"id,omitempty"`
	Name    string `json:"name"`
	Sets    string `json:"sets"` // "3 sets of 115 kg × 5", "2 sets of 62.5 kg × 8, 62.5 kg × 6"
	Records int    `json:"records,omitempty"`
}

// FitnessWorkout is a workout finished that day.
type FitnessWorkout struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Time      string            `json:"time,omitempty"` // HH:MM it started, local
	Minutes   int               `json:"minutes"`
	Sets      int               `json:"sets"`
	VolumeKg  float64           `json:"volume_kg,omitempty"`
	Records   int               `json:"records,omitempty"`
	Feel      int               `json:"feel,omitempty"` // 1–5
	Notes     string            `json:"notes,omitempty"`
	URL       string            `json:"url,omitempty"`
	Exercises []FitnessExercise `json:"exercises"`
	Line      string            `json:"line"` // "Pull day (72 min, 15 sets, 2 records)"
}

// FitnessDay is the day in Pocket Fitness: the workouts, oldest first.
type FitnessDay struct {
	Workouts []FitnessWorkout `json:"workouts"`
	Minutes  int              `json:"minutes"`
	Records  int              `json:"records"`
}

// Fetch implements Fetcher.
func (f *Fitness) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := f.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Workouts []struct {
			ID          int64   `json:"id"`
			Name        string  `json:"name"`
			StartedAt   string  `json:"started_at"`
			DurationMin int     `json:"duration_min"`
			Sets        int     `json:"sets"`
			VolumeKg    float64 `json:"volume_kg"`
			Records     int     `json:"records"`
			Feel        int     `json:"feel"`
			Notes       string  `json:"notes"`
			URL         string  `json:"url"`
			Exercises   []struct {
				Exercise   string `json:"exercise"`
				ExerciseID int64  `json:"exercise_id"`
				Sets       string `json:"sets"`
				Records    int    `json:"records"`
			} `json:"exercises"`
		} `json:"workouts"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := FitnessDay{Workouts: []FitnessWorkout{}}
	var parts []string
	for _, r := range page.Workouts {
		w := FitnessWorkout{ID: r.ID, Name: firstNonEmptyStr(strings.TrimSpace(r.Name), "A workout"), Time: clockIn(r.StartedAt, loc),
			Minutes: r.DurationMin, Sets: r.Sets, VolumeKg: Round1(r.VolumeKg), Records: r.Records, Feel: r.Feel,
			Notes: strings.TrimSpace(r.Notes), URL: strings.TrimSpace(r.URL), Exercises: []FitnessExercise{}}
		for _, x := range r.Exercises {
			sets := setsInWords(x.Sets)
			if sets == "" && strings.TrimSpace(x.Exercise) == "" {
				continue // only warm-ups, or nothing to say
			}
			w.Exercises = append(w.Exercises, FitnessExercise{ID: x.ExerciseID, Name: firstNonEmptyStr(strings.TrimSpace(x.Exercise), "An exercise"), Sets: sets, Records: x.Records})
		}
		w.Line = fitnessLine(w)
		d.Workouts = append(d.Workouts, w)
		d.Minutes += w.Minutes
		d.Records += w.Records
		parts = append(parts, w.Line)
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = strings.Join(parts[:3], "; ") + " and " + Plural(len(parts)-3, "more workout", "more workouts")
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Workouts)), Summary: sum}, nil
}

// fitnessLine: "Pull day (72 min, 15 sets, 2 records)".
func fitnessLine(w FitnessWorkout) string {
	var p []string
	if w.Minutes > 0 {
		p = append(p, fmt.Sprintf("%d min", w.Minutes))
	}
	if w.Sets > 0 {
		p = append(p, Plural(w.Sets, "set", "sets"))
	}
	if w.Records > 0 {
		p = append(p, Plural(w.Records, "record", "records"))
	}
	if len(p) == 0 {
		return w.Name
	}
	return w.Name + " (" + strings.Join(p, ", ") + ")"
}

// setsInWords folds Pocket Fitness's sets ("115 kg × 5, 115 kg × 5,
// 115 kg × 5") into fewer words: "3 sets of 115 kg × 5"; a change on the way
// reads "2 sets of 62.5 kg × 8, 62.5 kg × 6".
func setsInWords(s string) string {
	var runs []string
	var cur string
	n := 0
	flush := func() {
		switch {
		case n == 1:
			runs = append(runs, cur)
		case n > 1:
			runs = append(runs, fmt.Sprintf("%d sets of %s", n, cur))
		}
	}
	for _, x := range strings.Split(s, ",") {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if x == cur {
			n++
			continue
		}
		flush()
		cur, n = x, 1
	}
	flush()
	return strings.Join(runs, ", ")
}
