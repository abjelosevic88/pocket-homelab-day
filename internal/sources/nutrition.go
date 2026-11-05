package sources

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Nutrition ----------------

// Nutrition reads the day in Pocket Nutrition: what the person ate, meal by
// meal, against their calorie budget and macro goals, the water they drank and
// their note for the day.
type Nutrition struct{ Conn pocket.Conn }

// NewNutrition creates a fetcher over a connection.
func NewNutrition(conn pocket.Conn) *Nutrition { return &Nutrition{Conn: conn} }

// Name implements Fetcher.
func (n *Nutrition) Name() string { return "nutrition" }

// Test implements Fetcher.
func (n *Nutrition) Test(ctx context.Context) (string, error) {
	cl, err := n.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Nutrition " + v, nil
}

// NutritionMeal is one meal of the day with its foods in plain words.
type NutritionMeal struct {
	Meal  string   `json:"meal"`  // breakfast, lunch, dinner, snacks
	Label string   `json:"label"` // "Breakfast"
	Time  string   `json:"time,omitempty"`
	Kcal  int      `json:"kcal"`
	Foods []string `json:"foods"` // "oats", "banana", "coffee ×2"
	Line  string   `json:"line"`  // "Breakfast · oats, banana, coffee — 520 kcal"
}

// NutritionDay is the day in Pocket Nutrition. Budget is the calorie goal
// plus what was burned, when the person counts that; goals are 0 when not set.
type NutritionDay struct {
	Kcal        int             `json:"kcal"`
	Budget      int             `json:"budget,omitempty"`
	Burned      int             `json:"burned,omitempty"`
	Protein     float64         `json:"protein"`
	Carbs       float64         `json:"carbs"`
	Fat         float64         `json:"fat"`
	ProteinGoal int             `json:"protein_goal,omitempty"`
	CarbsGoal   int             `json:"carbs_goal,omitempty"`
	FatGoal     int             `json:"fat_goal,omitempty"`
	WaterMl     int             `json:"water_ml"`
	WaterGoal   int             `json:"water_goal,omitempty"`
	Entries     int             `json:"entries"`
	Meals       []NutritionMeal `json:"meals"`
	Note        string          `json:"note,omitempty"`
	Line        string          `json:"line"` // "1,850 kcal of 2,300 (protein 112 g, carbs 190 g, fat 64 g)"
}

// Fetch implements Fetcher.
func (n *Nutrition) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := n.Conn(ctx)
	if err != nil {
		return nil, err
	}
	type numbers struct {
		Kcal    float64 `json:"kcal"`
		Protein float64 `json:"protein"`
		Carbs   float64 `json:"carbs"`
		Fat     float64 `json:"fat"`
	}
	var page struct {
		Meals []struct {
			Meal    string  `json:"meal"`
			Label   string  `json:"label"`
			Totals  numbers `json:"totals"`
			Entries []struct {
				ID        int64   `json:"id"`
				Name      string  `json:"name"`
				Kcal      float64 `json:"kcal"`
				CreatedAt string  `json:"created_at"`
			} `json:"entries"`
		} `json:"meals"`
		Totals  numbers `json:"totals"`
		Entries int     `json:"entries"`
		WaterMl int     `json:"water_ml"`
		Goals   struct {
			Kcal    int `json:"kcal"`
			Protein int `json:"protein"`
			Carbs   int `json:"carbs"`
			Fat     int `json:"fat"`
			Water   int `json:"water"`
			Burned  int `json:"burned"`
			Budget  int `json:"budget"`
		} `json:"goals"`
		Note string `json:"note"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	g := page.Goals
	d := NutritionDay{Kcal: roundInt(page.Totals.Kcal), Budget: g.Budget, Burned: g.Burned,
		Protein: Round1(page.Totals.Protein), Carbs: Round1(page.Totals.Carbs), Fat: Round1(page.Totals.Fat),
		ProteinGoal: g.Protein, CarbsGoal: g.Carbs, FatGoal: g.Fat, WaterMl: page.WaterMl, WaterGoal: g.Water,
		Entries: page.Entries, Meals: []NutritionMeal{}, Note: strings.TrimSpace(page.Note)}
	if d.Budget == 0 {
		d.Budget = g.Kcal
	}
	count := 0
	for _, m := range page.Meals {
		if len(m.Entries) == 0 {
			continue
		}
		meal := NutritionMeal{Meal: m.Meal, Label: firstNonEmptyStr(strings.TrimSpace(m.Label), mealLabel(m.Meal)), Kcal: roundInt(m.Totals.Kcal), Foods: []string{}}
		// the meal is on the timeline at its first entry, when that was logged on the day itself (not written in later)
		var first time.Time
		times := map[string]int{}
		var order []string
		for _, e := range m.Entries {
			count++
			if t, ok := parseAt(e.CreatedAt); ok && t.In(loc).Format("2006-01-02") == date && (first.IsZero() || t.Before(first)) {
				first = t
			}
			name := lowerFirst(strings.TrimSpace(e.Name))
			if name == "" {
				name = "something"
			}
			if times[name] == 0 {
				order = append(order, name)
			}
			times[name]++
		}
		for _, name := range order {
			if times[name] > 1 {
				name = fmt.Sprintf("%s ×%d", name, times[name])
			}
			meal.Foods = append(meal.Foods, name)
		}
		if !first.IsZero() {
			meal.Time = first.In(loc).Format("15:04")
		}
		meal.Line = meal.Label + " · " + strings.Join(meal.Foods, ", ")
		if meal.Kcal > 0 {
			meal.Line += " — " + Thousands(meal.Kcal) + " kcal"
		}
		d.Meals = append(d.Meals, meal)
	}
	if d.Entries == 0 {
		d.Entries = count
	}
	if d.Entries == 0 {
		return &Result{Payload: d, Headline: 0, Summary: ""}, nil
	}
	d.Line = nutritionLine(d)
	return &Result{Payload: d, Headline: float64(d.Kcal), Summary: d.Line}, nil
}

// nutritionLine: "1,850 kcal of 2,300 (protein 112 g, carbs 190 g, fat 64 g)".
func nutritionLine(d NutritionDay) string {
	s := Thousands(d.Kcal) + " kcal"
	if d.Budget > 0 {
		s += " of " + Thousands(d.Budget)
	}
	return s + fmt.Sprintf(" (protein %d g, carbs %d g, fat %d g)", roundInt(d.Protein), roundInt(d.Carbs), roundInt(d.Fat))
}

// WaterWords is an amount of water: "750 ml", "1.5 l".
func WaterWords(ml int) string {
	if ml >= 1000 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(ml)/1000), ".0") + " l"
	}
	return fmt.Sprintf("%d ml", ml)
}

// Thousands writes a whole number with thousands separators: 1,850.
func Thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func roundInt(f float64) int { return int(math.Round(f)) }

func mealLabel(m string) string {
	switch m {
	case "breakfast":
		return "Breakfast"
	case "lunch":
		return "Lunch"
	case "dinner":
		return "Dinner"
	}
	return "Snacks"
}

// parseAt reads an RFC 3339 time.
func parseAt(at string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
