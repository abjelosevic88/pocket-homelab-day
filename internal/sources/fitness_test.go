package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestFitnessDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-04" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-04",
			"workouts": []map[string]any{
				{"id": 96, "name": "Pull day", "started_at": "2026-11-04T16:55:00Z", "duration_min": 72, "sets": 15, "volume_kg": 5072.5, "records": 2, "feel": 4,
					"notes": "Grip gave out on the last row set.", "url": "https://fitness.example.org/workouts/96",
					"exercises": []map[string]any{
						{"exercise": "Deadlift (Barbell)", "exercise_id": 10, "records": 3, "sets": "115 kg × 5, 115 kg × 5, 115 kg × 5"},
						{"exercise": "Pull Up", "exercise_id": 11, "records": 0, "sets": "11 reps, 11 reps, 11 reps"},
						{"exercise": "Bent Over Row (Barbell)", "exercise_id": 14, "records": 0, "sets": "62.5 kg × 8, 62.5 kg × 8, 62.5 kg × 6"},
						{"exercise": "Running", "exercise_id": 40, "records": 1, "sets": "5 km in 26:10"},
						{"exercise": "Bench Press (Barbell)", "exercise_id": 1, "records": 0, "sets": ""},
					}},
				{"id": 97, "name": "", "started_at": "2026-11-04T20:00:00Z", "duration_min": 20, "sets": 4, "volume_kg": 0, "records": 0, "feel": 0, "notes": "", "url": "",
					"exercises": []map[string]any{{"exercise": "Plank", "exercise_id": 30, "records": 0, "sets": "1:00, 1:00, 1:30, 1:00"}}},
			},
			// Pocket Health's own run: the health source has it, so it is not said twice
			"other":   []map[string]any{{"id": 7, "name": "Morning run", "kind": "running", "minutes": 31, "distance_m": 5200, "health": true}},
			"planned": map[string]any{"weekday": 3, "routine_id": 2, "routine": "Pull day", "time": "18:00"},
			"text":    "Pull day (72 min, 15 sets); A workout (20 min, 4 sets); Morning run (31 min).",
		})
	}))
	defer ts.Close()
	f := NewFitness(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	res, err := f.Fetch(context.Background(), "2026-11-04", loc)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Pull day (72 min, 15 sets, 2 records); A workout (20 min, 4 sets)"; res.Headline != 2 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(FitnessDay)
	if len(d.Workouts) != 2 || d.Minutes != 92 || d.Records != 2 {
		t.Fatalf("day %+v", d)
	}
	w := d.Workouts[0]
	if w.Time != "17:55" || w.Notes != "Grip gave out on the last row set." || w.URL != "https://fitness.example.org/workouts/96" || w.Feel != 4 || w.VolumeKg != 5072.5 {
		t.Fatalf("workout %+v", w)
	}
	if len(w.Exercises) != 5 {
		t.Fatalf("exercises %+v", w.Exercises)
	}
	for i, want := range []FitnessExercise{
		{ID: 10, Name: "Deadlift (Barbell)", Sets: "3 sets of 115 kg × 5", Records: 3},
		{ID: 11, Name: "Pull Up", Sets: "3 sets of 11 reps"},
		{ID: 14, Name: "Bent Over Row (Barbell)", Sets: "2 sets of 62.5 kg × 8, 62.5 kg × 6"},
		{ID: 40, Name: "Running", Sets: "5 km in 26:10", Records: 1},
		{ID: 1, Name: "Bench Press (Barbell)", Sets: ""},
	} {
		if w.Exercises[i] != want {
			t.Errorf("exercise %d: got %+v want %+v", i, w.Exercises[i], want)
		}
	}
	if p := d.Workouts[1]; p.Name != "A workout" || p.Time != "21:00" || p.Exercises[0].Sets != "2 sets of 1:00, 1:30, 1:00" || p.Line != "A workout (20 min, 4 sets)" {
		t.Fatalf("second workout %+v", p)
	}
}

func TestFitnessEmptyDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"date":"2026-11-05","workouts":[],"other":[],"planned":null,"text":"No workout."}`))
	}))
	defer ts.Close()
	f := NewFitness(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := f.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Payload.(FitnessDay); res.Headline != 0 || res.Summary != "" || d.Workouts == nil || len(d.Workouts) != 0 {
		t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
	}
}

func TestSetsInWords(t *testing.T) {
	for in, want := range map[string]string{
		"80 kg × 8, 80 kg × 8":             "2 sets of 80 kg × 8",
		"80 kg × 8":                        "80 kg × 8",
		"60 kg × 10, 70 kg × 8, 70 kg × 8": "60 kg × 10, 2 sets of 70 kg × 8",
		"12 reps, 10 reps":                 "12 reps, 10 reps",
		"5.2 km in 28:10":                  "5.2 km in 28:10",
		"":                                 "",
		" 1:00 ,1:00,  ":                   "2 sets of 1:00",
		"20 kg × 5, 20 kg × 5, 25 kg × 5, 20 kg × 5": "2 sets of 20 kg × 5, 25 kg × 5, 20 kg × 5",
	} {
		if got := setsInWords(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
