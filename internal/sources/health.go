package sources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Health ----------------

// Health reads the day's numbers: steps, sleep, workouts, heart, weight, goals met.
type Health struct{ Conn pocket.Conn }

// NewHealth creates a fetcher over a connection.
func NewHealth(conn pocket.Conn) *Health { return &Health{Conn: conn} }

// Name implements Fetcher.
func (h *Health) Name() string { return "health" }

// Test implements Fetcher.
func (h *Health) Test(ctx context.Context) (string, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Health " + v, nil
}

// HealthWorkout is one session on the day.
type HealthWorkout struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"`
	Name       string  `json:"name"`
	Time       string  `json:"time"`
	Minutes    int     `json:"minutes"`
	DistanceKm float64 `json:"distance_km,omitempty"`
	Kcal       int     `json:"kcal,omitempty"`
	AvgHR      int     `json:"avg_hr,omitempty"`
}

// HealthSleep is the night that ended on this morning.
type HealthSleep struct {
	AsleepMin int    `json:"asleep_min"`
	InBedMin  int    `json:"in_bed_min"`
	DeepMin   int    `json:"deep_min,omitempty"`
	RemMin    int    `json:"rem_min,omitempty"`
	Bed       string `json:"bed"`
	Wake      string `json:"wake"`
}

// HealthMetric is one of the day's numbers.
type HealthMetric struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Value  float64  `json:"value"`
	Value2 *float64 `json:"value2,omitempty"`
	Unit   string   `json:"unit"`
}

// HealthDay is the day in Pocket Health.
type HealthDay struct {
	Steps       *float64        `json:"steps,omitempty"`
	DistanceKm  *float64        `json:"distance_km,omitempty"`
	ActiveKcal  *float64        `json:"active_kcal,omitempty"`
	ExerciseMin *float64        `json:"exercise_min,omitempty"`
	RestingHR   *float64        `json:"resting_hr,omitempty"`
	HRV         *float64        `json:"hrv,omitempty"`
	WeightKg    *float64        `json:"weight_kg,omitempty"`
	Sleep       *HealthSleep    `json:"sleep,omitempty"`
	Workouts    []HealthWorkout `json:"workouts"`
	GoalsMet    []string        `json:"goals_met"`
	Metrics     []HealthMetric  `json:"metrics"`
	Summary     string          `json:"summary"`
}

// Fetch implements Fetcher.
func (h *Health) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := h.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Metrics map[string]struct {
			Value  float64  `json:"value"`
			Value2 *float64 `json:"value2"`
			Unit   string   `json:"unit"`
			Name   string   `json:"name"`
		} `json:"metrics"`
		Workouts []struct {
			ID         int64   `json:"id"`
			Kind       string  `json:"kind"`
			Name       string  `json:"name"`
			StartAt    string  `json:"start_at"`
			DurationS  int     `json:"duration_s"`
			DistanceM  float64 `json:"distance_m"`
			EnergyKcal float64 `json:"energy_kcal"`
			AvgHR      float64 `json:"avg_hr"`
		} `json:"workouts"`
		Sleep []struct {
			StartAt string `json:"start_at"`
			EndAt   string `json:"end_at"`
			AsleepS int    `json:"asleep_s"`
			InBedS  int    `json:"in_bed_s"`
			DeepS   int    `json:"deep_s"`
			RemS    int    `json:"rem_s"`
		} `json:"sleep"`
		GoalsMet []string `json:"goals_met"`
		Summary  string   `json:"summary"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := HealthDay{Workouts: []HealthWorkout{}, GoalsMet: page.GoalsMet, Metrics: []HealthMetric{}, Summary: page.Summary}
	if d.GoalsMet == nil {
		d.GoalsMet = []string{}
	}
	get := func(k string) *float64 {
		if m, ok := page.Metrics[k]; ok {
			v := m.Value
			return &v
		}
		return nil
	}
	d.Steps, d.DistanceKm, d.ActiveKcal, d.ExerciseMin, d.RestingHR, d.HRV, d.WeightKg = get("steps"), get("distance"), get("active_energy"), get("exercise_min"), get("resting_hr"), get("hrv"), get("weight")
	for k, m := range page.Metrics {
		if strings.HasPrefix(k, "sleep_") {
			continue
		}
		d.Metrics = append(d.Metrics, HealthMetric{Key: k, Name: m.Name, Value: m.Value, Value2: m.Value2, Unit: m.Unit})
	}
	for _, w := range page.Workouts {
		t := ""
		if ts, ok := ParseAny(w.StartAt); ok {
			t = LocalClock(ts, loc)
		}
		d.Workouts = append(d.Workouts, HealthWorkout{ID: w.ID, Kind: w.Kind, Name: w.Name, Time: t, Minutes: w.DurationS / 60, DistanceKm: Round1(w.DistanceM / 1000), Kcal: int(w.EnergyKcal), AvgHR: int(w.AvgHR)})
	}
	if len(page.Sleep) > 0 {
		best := page.Sleep[0]
		for _, x := range page.Sleep {
			if x.AsleepS > best.AsleepS {
				best = x
			}
		}
		sl := &HealthSleep{AsleepMin: best.AsleepS / 60, InBedMin: best.InBedS / 60, DeepMin: best.DeepS / 60, RemMin: best.RemS / 60}
		if ts, ok := ParseAny(best.StartAt); ok {
			sl.Bed = LocalClock(ts, loc)
		}
		if ts, ok := ParseAny(best.EndAt); ok {
			sl.Wake = LocalClock(ts, loc)
		}
		d.Sleep = sl
	}
	headline := 0.0
	if d.Steps != nil {
		headline = *d.Steps
	}
	sum := page.Summary
	if sum == "" {
		var parts []string
		if d.Steps != nil {
			parts = append(parts, fmt.Sprintf("%d steps", int(*d.Steps)))
		}
		if d.Sleep != nil {
			parts = append(parts, "slept "+HoursMinutes(d.Sleep.AsleepMin))
		}
		for _, w := range d.Workouts {
			parts = append(parts, fmt.Sprintf("%s %d min", strings.ReplaceAll(w.Kind, "_", " "), w.Minutes))
		}
		sum = strings.Join(parts, " · ")
	}
	return &Result{Payload: d, Headline: headline, Summary: sum}, nil
}
