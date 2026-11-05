package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

func TestNutritionDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-04" {
			http.NotFound(w, r)
			return
		}
		e := func(id int, name string, kcal float64, at string) map[string]any {
			return map[string]any{"id": id, "name": name, "brand": "", "qty": 1, "serving": "1 serving", "amount": 1, "unit": "serving", "kcal": kcal, "protein": 1, "carbs": 1, "fat": 1, "note": "", "created_at": at}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-04",
			"meals": []map[string]any{
				{"meal": "breakfast", "label": "Breakfast", "totals": map[string]any{"kcal": 520.4, "protein": 18, "carbs": 80, "fat": 12},
					// logged out of order: the first one logged sets the time
					"entries": []map[string]any{e(1, "Oats", 380, "2026-11-04T06:45:10.123456Z"), e(2, "Banana", 105, "2026-11-04T06:40:00Z"), e(3, "Coffee", 17.7, "2026-11-04T06:46:00Z"), e(4, "Coffee", 17.7, "2026-11-04T09:00:00Z")}},
				{"meal": "lunch", "label": "Lunch", "totals": map[string]any{"kcal": 0}, "entries": []map[string]any{}},
				// written in the next morning: no time, so not on the timeline
				{"meal": "dinner", "label": "Dinner", "totals": map[string]any{"kcal": 1329.6}, "entries": []map[string]any{e(5, "Ćevapi", 1129.6, "2026-11-05T07:30:00Z"), e(6, "BBQ sauce", 200, "2026-11-05T07:31:00Z")}},
				{"meal": "snacks", "label": "Snacks", "totals": map[string]any{"kcal": 0}, "entries": []map[string]any{}},
			},
			"totals":    map[string]any{"kcal": 1850, "protein": 112.34, "carbs": 190.06, "fat": 63.6, "fiber": nil},
			"entries":   6,
			"water_ml":  1500,
			"goals":     map[string]any{"kcal": 2000, "protein": 140, "carbs": 250, "fat": 75, "fiber": 30, "water": 2000, "burned": 300, "budget": 2300},
			"remaining": map[string]any{"kcal": 450, "protein": 28, "carbs": 60, "fat": 11, "water": 500},
			"note":      "  Ate late, slept badly.  ",
			"text":      "1,850 kcal of 2,300; protein 112.3 g, carbs 190.1 g, fat 63.6 g, 1.5 l of water. Breakfast: oats, banana, coffee, coffee. Dinner: ćevapi, bBQ sauce.",
		})
	}))
	defer ts.Close()
	n := NewNutrition(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	loc, _ := time.LoadLocation("Europe/Sarajevo")
	res, err := n.Fetch(context.Background(), "2026-11-04", loc)
	if err != nil {
		t.Fatal(err)
	}
	if want := "1,850 kcal of 2,300 (protein 112 g, carbs 190 g, fat 64 g)"; res.Headline != 1850 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(NutritionDay)
	if d.Kcal != 1850 || d.Budget != 2300 || d.Burned != 300 || d.Protein != 112.3 || d.Carbs != 190.1 || d.Fat != 63.6 || d.ProteinGoal != 140 || d.WaterMl != 1500 || d.WaterGoal != 2000 || d.Entries != 6 || d.Note != "Ate late, slept badly." {
		t.Fatalf("day %+v", d)
	}
	want := []NutritionMeal{
		{Meal: "breakfast", Label: "Breakfast", Time: "07:40", Kcal: 520, Foods: []string{"oats", "banana", "coffee ×2"}, Line: "Breakfast · oats, banana, coffee ×2 — 520 kcal"},
		{Meal: "dinner", Label: "Dinner", Kcal: 1330, Foods: []string{"ćevapi", "BBQ sauce"}, Line: "Dinner · ćevapi, BBQ sauce — 1,330 kcal"},
	}
	if !reflect.DeepEqual(d.Meals, want) {
		t.Fatalf("meals\n got %+v\nwant %+v", d.Meals, want)
	}
}

func TestNutritionNoGoalAndEmptyDays(t *testing.T) {
	body := `{"date":"2026-11-05","meals":[{"meal":"snacks","label":"","totals":{"kcal":95},"entries":[{"id":9,"name":"Apple","kcal":95,"created_at":"2026-11-05T15:00:00Z"}]}],"totals":{"kcal":95,"protein":0.5,"carbs":25,"fat":0.3},"entries":1,"water_ml":0,"goals":{"kcal":0,"protein":0,"carbs":0,"fat":0,"fiber":0,"water":0,"burned":0,"budget":0},"note":"","text":"95 kcal; protein 0.5 g, carbs 25 g, fat 0.3 g. Snacks: apple."}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer ts.Close()
	n := NewNutrition(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := n.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	d := res.Payload.(NutritionDay)
	if res.Summary != "95 kcal (protein 1 g, carbs 25 g, fat 0 g)" || len(d.Meals) != 1 || d.Meals[0].Line != "Snacks · apple — 95 kcal" || d.Meals[0].Time != "15:00" {
		t.Fatalf("no goals: %q %+v", res.Summary, d)
	}

	// only water, then nothing: no summary, nothing on the calendar
	for _, b := range []string{
		`{"date":"2026-11-06","meals":[{"meal":"breakfast","label":"Breakfast","totals":{"kcal":0},"entries":[]}],"totals":{"kcal":0},"entries":0,"water_ml":750,"goals":{"kcal":2300,"water":2000,"budget":2300},"note":"","text":"Nothing logged; 750 ml of water."}`,
		`{"date":"2026-11-06","meals":[],"totals":{"kcal":0},"entries":0,"water_ml":0,"goals":{},"note":"","text":"Nothing logged."}`,
	} {
		body = b
		res, err := n.Fetch(context.Background(), "2026-11-06", time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		if d := res.Payload.(NutritionDay); res.Headline != 0 || res.Summary != "" || d.Meals == nil || len(d.Meals) != 0 {
			t.Fatalf("empty day: %v %q %+v", res.Headline, res.Summary, d)
		}
	}
}

func TestWaterWordsAndThousands(t *testing.T) {
	for in, want := range map[int]string{250: "250 ml", 1000: "1 l", 1500: "1.5 l", 2300: "2.3 l"} {
		if got := WaterWords(in); got != want {
			t.Errorf("WaterWords(%d) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[int]string{0: "0", 950: "950", 1850: "1,850", 12345: "12,345", 1234567: "1,234,567", -2300: "-2,300"} {
		if got := Thousands(in); got != want {
			t.Errorf("Thousands(%d) = %q, want %q", in, got, want)
		}
	}
}
