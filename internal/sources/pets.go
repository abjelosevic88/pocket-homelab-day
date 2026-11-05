package sources

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-day/internal/pocket"
)

// ---------------- Pocket Pets ----------------

// Pets reads what happened with the pets that day: vet visits, vaccines,
// treatments, grooming, the weighings and the birthdays.
type Pets struct{ Conn pocket.Conn }

// NewPets creates a fetcher over a connection.
func NewPets(conn pocket.Conn) *Pets { return &Pets{Conn: conn} }

// Name implements Fetcher.
func (p *Pets) Name() string { return "pets" }

// Test implements Fetcher.
func (p *Pets) Test(ctx context.Context) (string, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return "", err
	}
	v, err := cl.Health(ctx)
	if err != nil {
		return "", err
	}
	return "Pocket Pets " + v, nil
}

// PetsEntry is one entry of the health log: a vet visit, a vaccine, grooming.
type PetsEntry struct {
	ID         int64  `json:"id"`
	PetID      int64  `json:"pet_id"`
	Pet        string `json:"pet"`
	Species    string `json:"species,omitempty"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Notes      string `json:"notes,omitempty"`
	Pro        string `json:"pro,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
	Cost       int64  `json:"cost"` // minor units
	Currency   string `json:"currency"`
	Line       string `json:"line"` // "Luna at the vet: limping — Dr. Hodžić"
}

// PetsWeight is a weighing that day.
type PetsWeight struct {
	ID     int64    `json:"id"`
	PetID  int64    `json:"pet_id"`
	Pet    string   `json:"pet"`
	Kg     float64  `json:"kg"`
	Change *float64 `json:"change,omitempty"` // kg since the weighing before
	Line   string   `json:"line"`
}

// PetsBirthday is a pet's birthday.
type PetsBirthday struct {
	PetID int64  `json:"pet_id"`
	Pet   string `json:"pet"`
	Turns int    `json:"turns"`
	Line  string `json:"line"`
}

// PetsDay is the day in Pocket Pets. Routine purchases (a bag of food) and
// care that was only due are left out: the journal is what happened.
type PetsDay struct {
	Entries   []PetsEntry    `json:"entries"`
	Weights   []PetsWeight   `json:"weights"`
	Birthdays []PetsBirthday `json:"birthdays"`
}

// Fetch implements Fetcher.
func (p *Pets) Fetch(ctx context.Context, date string, loc *time.Location) (*Result, error) {
	cl, err := p.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var page struct {
		Records []struct {
			ID         int64  `json:"id"`
			PetID      int64  `json:"pet_id"`
			PetName    string `json:"pet_name"`
			PetSpecies string `json:"pet_species"`
			Kind       string `json:"kind"`
			Title      string `json:"title"`
			Notes      string `json:"notes"`
			ValidUntil string `json:"valid_until"`
			Cost       int64  `json:"cost"`
			Currency   string `json:"currency"`
			ProName    string `json:"pro_name"`
			CareTitle  string `json:"care_title"`
		} `json:"records"`
		Weights []struct {
			ID      int64    `json:"id"`
			PetID   int64    `json:"pet_id"`
			PetName string   `json:"pet_name"`
			Kg      float64  `json:"kg"`
			Change  *float64 `json:"change"`
		} `json:"weights"`
		Birthdays []struct {
			PetID int64  `json:"pet_id"`
			Name  string `json:"name"`
			Turns int    `json:"turns"`
		} `json:"birthdays"`
	}
	if err := cl.Get(ctx, "/api/v1/day/"+date, nil, &page); err != nil {
		return nil, err
	}
	d := PetsDay{Entries: []PetsEntry{}, Weights: []PetsWeight{}, Birthdays: []PetsBirthday{}}
	var parts []string
	for _, b := range page.Birthdays {
		pb := PetsBirthday{PetID: b.PetID, Pet: b.Name, Turns: b.Turns, Line: petBirthdayLine(b.Name, b.Turns)}
		d.Birthdays = append(d.Birthdays, pb)
		parts = append(parts, pb.Line)
	}
	for _, r := range page.Records {
		if r.Kind == "purchase" {
			continue
		}
		title := strings.TrimSpace(r.Title)
		if title == "" {
			title = strings.TrimSpace(r.CareTitle)
		}
		e := PetsEntry{ID: r.ID, PetID: r.PetID, Pet: r.PetName, Species: r.PetSpecies, Kind: r.Kind, Title: title, Notes: r.Notes, Pro: r.ProName,
			ValidUntil: r.ValidUntil, Cost: r.Cost, Currency: r.Currency, Line: petRecordLine(r.PetName, r.Kind, title, r.ProName)}
		d.Entries = append(d.Entries, e)
		parts = append(parts, e.Line)
	}
	for _, w := range page.Weights {
		pw := PetsWeight{ID: w.ID, PetID: w.PetID, Pet: w.PetName, Kg: w.Kg, Change: w.Change, Line: petWeightLine(w.PetName, w.Kg, w.Change)}
		d.Weights = append(d.Weights, pw)
		parts = append(parts, pw.Line)
	}
	sum := ""
	if len(parts) > 0 {
		sum = strings.Join(parts, "; ")
		if len(parts) > 3 {
			sum = fmt.Sprintf("%s and %d more", strings.Join(parts[:3], "; "), len(parts)-3)
		}
	}
	return &Result{Payload: d, Headline: float64(len(d.Entries)), Summary: sum}, nil
}

// petRecordLine: "Luna at the vet: limping on the front left leg — Dr. Hodžić".
func petRecordLine(pet, kind, title, pro string) string {
	if pet == "" {
		pet = "The pet"
	}
	var lead string
	switch kind {
	case "vet_visit":
		lead, title = pet+" at the vet", lowerFirst(title)
	case "vaccine":
		lead = pet + " vaccinated"
	case "treatment":
		lead = pet + " treated"
	case "medication":
		lead = pet + "’s medication"
	case "grooming":
		lead = pet + " groomed"
	case "symptom":
		lead, title = pet, lowerFirst(title)
		if title == "" {
			title = "not feeling well"
		}
	case "test":
		lead = pet + " tested"
	case "surgery":
		lead = pet + " had surgery"
	default:
		lead = pet
		if title == "" {
			title = strings.ReplaceAll(kind, "_", " ")
		}
	}
	s := lead
	if title != "" {
		s += ": " + title
	}
	if pro != "" {
		s += " — " + pro
	}
	return s
}

// petWeightLine: "Mici weighed 4.9 kg (−0.04)".
func petWeightLine(pet string, kg float64, change *float64) string {
	s := pet + " weighed " + kgString(kg) + " kg"
	if change != nil && math.Abs(*change) >= 0.005 {
		sign := "+"
		if *change < 0 {
			sign = "−"
		}
		s += " (" + sign + kgString(math.Abs(*change)) + ")"
	}
	return s
}

// petBirthdayLine: "Luna turned 5".
func petBirthdayLine(pet string, turns int) string {
	if turns <= 0 {
		return pet + "’s birthday"
	}
	return fmt.Sprintf("%s turned %d", pet, turns)
}

// kgString writes a weight with at most two decimals and no trailing zeros.
func kgString(kg float64) string {
	return strconv.FormatFloat(math.Round(kg*100)/100, 'f', -1, 64)
}

// lowerFirst lowercases the first letter of a sentence-cased description
// ("Limping on…" → "limping on…") but leaves names and acronyms alone.
func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 || !unicode.IsUpper(r) {
		return s
	}
	word := s
	if i := strings.IndexByte(s, ' '); i > 0 {
		word = s[:i]
	}
	if strings.ContainsAny(word, ".") {
		return s
	}
	for _, c := range word[n:] {
		if unicode.IsUpper(c) {
			return s
		}
	}
	return string(unicode.ToLower(r)) + s[n:]
}
