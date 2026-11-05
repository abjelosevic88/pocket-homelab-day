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

func TestPlacesDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-05" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-05",
			"visits": []map[string]any{
				{"id": 1, "place_id": 1, "date": "2026-11-05", "time": "19:30", "rating": 5, "with_whom": "Ana", "spent": 6800, "currency": "BAM", "place_name": "Kibe Mahala", "place_kind": "restaurant", "city": "Sarajevo", "notes": "the lamb"},
				{"id": 2, "place_id": 2, "date": "2026-11-05", "time": "09:10", "minutes": 48, "place_name": "Kafeterija", "place_kind": "cafe", "city": "Sarajevo"},
			},
			"planned": []map[string]any{
				{"id": 3, "place_id": 3, "date": "2026-11-05", "time": "13:00", "planned": true, "with_whom": "Marko", "place_name": "Ćevabdžinica Željo", "place_kind": "restaurant"},
			},
			"added": []map[string]any{
				{"id": 1, "name": "Kibe Mahala", "kind": "restaurant", "city": "Sarajevo", "status": "been"},
				{"id": 5, "name": "Zlatna Ribica", "kind": "bar", "city": "Sarajevo", "status": "want"},
			},
			"spent": []map[string]any{{"currency": "BAM", "total": 6800, "visits": 1}},
		})
	}))
	defer ts.Close()
	p := NewPlaces(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := p.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Dinner at Kibe Mahala with Ana, ★5, 68.00 BAM; Coffee at Kafeterija (48 min); Planned: lunch at Ćevabdžinica Željo with Marko and 1 more"; res.Headline != 2 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(PlacesDay)
	if len(d.Visits) != 2 || d.Visits[0].Time != "19:30" || d.Visits[0].Notes != "the lamb" || d.Visits[0].PlaceID != 1 || d.Visits[1].Minutes != 48 {
		t.Fatalf("visits %+v", d.Visits)
	}
	if len(d.Planned) != 1 || d.Planned[0].Line != "Planned: lunch at Ćevabdžinica Željo with Marko" {
		t.Fatalf("planned %+v", d.Planned)
	}
	if len(d.Saved) != 2 || d.Saved[1].Line != "Saved to try: Zlatna Ribica" || !d.Saved[1].Want || d.Saved[0].Line != "Saved: Kibe Mahala" {
		t.Fatalf("saved %+v", d.Saved)
	}
	if len(d.Spent) != 1 || d.Spent[0].Total != 6800 {
		t.Fatalf("spent %+v", d.Spent)
	}
}

func TestPlacesLines(t *testing.T) {
	for _, c := range []struct {
		v    PlacesVisit
		want string
	}{
		{PlacesVisit{Place: "Zemaljski muzej", Kind: "museum", Minutes: 90, Rating: 4.5}, "Visited Zemaljski muzej (1 h 30 min), ★4.5"},
		{PlacesVisit{Place: "Pekara Edin", Kind: "bakery", Time: "07:40", Spent: 350, Currency: "BAM"}, "Stopped by Pekara Edin, 3.50 BAM"},
		{PlacesVisit{Place: "Hodžić", Kind: "restaurant"}, "Ate at Hodžić"},
		{PlacesVisit{Place: "Hodžić", Kind: "street_food", Time: "12:15", With: "Ana, Marko"}, "Lunch at Hodžić with Ana, Marko"},
		{PlacesVisit{Place: "Zlatna Ribica", Kind: "bar", Time: "22:00", Minutes: 120}, "Drinks at Zlatna Ribica (2 h)"},
		{PlacesVisit{Place: "Hotel Europe", Kind: "hotel", Spent: 12000, Currency: "EUR"}, "Stayed at Hotel Europe, €120.00"},
		{PlacesVisit{Place: "Escape room", Kind: "activity"}, "Went to Escape room"},
	} {
		if got := placeVisitLine(c.v); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
	if got := placePlannedLine(PlacesVisit{Place: "Vrelo Bosne", Kind: "nature"}); got != "Planned: Vrelo Bosne" {
		t.Errorf("planned: %q", got)
	}
}

// Places saved that day are one part of the summary, the ones visited the
// same day left out.
func TestPlacesSavedSummary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"visits":  []map[string]any{},
			"planned": []map[string]any{},
			"added": []map[string]any{
				{"id": 1, "name": "Kibe Mahala", "status": "want"}, {"id": 2, "name": "Kafeterija", "status": "been"},
				{"id": 3, "name": "Zlatna Ribica", "status": "want"}, {"id": 4, "name": "Vrelo Bosne", "status": "want"},
			},
			"spent": []map[string]any{},
		})
	}))
	defer ts.Close()
	p := NewPlaces(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := p.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Saved to Pocket Places: Kibe Mahala, Kafeterija, Zlatna Ribica and 1 more"; res.Headline != 0 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
}
