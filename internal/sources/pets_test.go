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

func TestPetsDay(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/day/2026-11-05" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": "2026-11-05",
			"records": []map[string]any{
				{"id": 11, "pet_id": 1, "pet_name": "Luna", "pet_species": "dog", "kind": "vet_visit", "title": "Limping on the front left leg", "notes": "rest a week", "cost": 4500, "currency": "BAM", "pro_name": "Dr. Hodžić", "date": "2026-11-05"},
				{"id": 12, "pet_id": 2, "pet_name": "Mici", "pet_species": "cat", "kind": "purchase", "title": "Food 10 kg", "cost": 6900, "currency": "BAM", "date": "2026-11-05"},
				{"id": 13, "pet_id": 1, "pet_name": "Luna", "pet_species": "dog", "kind": "vaccine", "title": "", "care_title": "Rabies", "valid_until": "2027-11-05", "date": "2026-11-05"},
			},
			"weights":   []map[string]any{{"id": 4, "pet_id": 2, "pet_name": "Mici", "weighed_on": "2026-11-05", "kg": 4.9, "change": -0.04}},
			"due":       []map[string]any{{"id": 9, "pet_id": 2, "pet_name": "Mici", "kind": "vaccine", "title": "Booster", "due_on": "2026-11-05", "status": "due"}},
			"birthdays": []map[string]any{{"pet_id": 1, "name": "Luna", "turns": 5}},
		})
	}))
	defer ts.Close()
	p := NewPets(func(context.Context) (*pocket.Client, error) { return pocket.New(ts.URL, "t", 5*time.Second), nil })
	res, err := p.Fetch(context.Background(), "2026-11-05", time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Luna turned 5; Luna at the vet: limping on the front left leg — Dr. Hodžić; Luna vaccinated: Rabies and 1 more"; res.Headline != 2 || res.Summary != want {
		t.Fatalf("got %v %q\nwant %q", res.Headline, res.Summary, want)
	}
	d := res.Payload.(PetsDay)
	if len(d.Entries) != 2 || d.Entries[0].Cost != 4500 || d.Entries[0].Pro != "Dr. Hodžić" || d.Entries[1].Title != "Rabies" || d.Entries[1].ValidUntil != "2027-11-05" {
		t.Fatalf("entries %+v (the purchase is left out, the care title fills in)", d.Entries)
	}
	if len(d.Weights) != 1 || d.Weights[0].Line != "Mici weighed 4.9 kg (−0.04)" {
		t.Fatalf("weights %+v", d.Weights)
	}
	if len(d.Birthdays) != 1 || d.Birthdays[0].PetID != 1 {
		t.Fatalf("birthdays %+v", d.Birthdays)
	}
}

func TestPetLines(t *testing.T) {
	up := 0.1
	for _, c := range []struct{ got, want string }{
		{petRecordLine("Luna", "grooming", "Bath and nails", "Salon Šapa"), "Luna groomed: Bath and nails — Salon Šapa"},
		{petRecordLine("Luna", "symptom", "", ""), "Luna: not feeling well"},
		{petRecordLine("Luna", "surgery", "Spaying", ""), "Luna had surgery: Spaying"},
		{petRecordLine("Luna", "vet_visit", "FIV test", ""), "Luna at the vet: FIV test"},
		{petRecordLine("Luna", "other", "", ""), "Luna: other"},
		{petWeightLine("Rex", 31.25, &up), "Rex weighed 31.25 kg (+0.1)"},
		{petWeightLine("Rex", 31, nil), "Rex weighed 31 kg"},
		{petBirthdayLine("Rex", 0), "Rex’s birthday"},
	} {
		if c.got != c.want {
			t.Errorf("got %q want %q", c.got, c.want)
		}
	}
}
