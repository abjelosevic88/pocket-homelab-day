package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// seedDemo writes a month of journal with synthetic facts so the UI has
// something to show without any sibling app connected. Only runs on an empty
// journal.
func (s *Server) seedDemo(ctx context.Context) error {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	var u *store.User
	if n == 0 {
		// The same account AUTH_MODE=none resolves to, so the demo journal is the one you see.
		u = &store.User{Email: "admin@localhost", Name: "Demo", Role: "admin"}
		if err := s.store.CreateUser(ctx, u); err != nil {
			return err
		}
	} else {
		users, _ := s.store.ListUsers(ctx)
		u = &users[0]
	}
	if e, _, _ := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, Limit: 1}); len(e) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(7))
	today := store.Today()
	highlights := []string{"Long walk by the river", "Finished the deck for Monday", "Dinner with M. and the kids", "Fixed the garage door at last", "Rainy day, read on the sofa", "First coffee outside this year", "Quiet day, nothing to report", "Drove to the lake", "Board games until midnight", "Cleaned out the basement"}
	texts := []string{"Slept badly but the morning run fixed it. Spent the afternoon on the slides and actually enjoyed it.", "The kind of day that disappears. Emails, a call that ran long, leftovers.", "We took the long way back along the water. The light at six was unreal.", "Tried the new ramen place. Too salty, would go again.", "Mum called. Dad's test results are fine.", ""}
	tags := []string{"family", "work", "walk", "food", "music", "rest", "friends", "home"}
	artists := []string{"Boards of Canada", "Khruangbin", "Nils Frahm", "Radiohead", "Bonobo", "Tycho"}
	shows := []string{"Severance", "Slow Horses", "The Bear", "Shōgun"}
	places := []string{"Home", "Office", "Gym", "Kaufland", "Park", "Café Ruby", "Mum's"}
	conds := []string{"clear", "partly", "cloudy", "rain", "clear", "partly"}
	for i := 45; i >= 0; i-- {
		d := store.AddDays(today, -i)
		if i == 0 || rng.Intn(5) == 0 {
			// today is left for you to write; some days have facts but no words
			if i == 0 {
				_ = s.store.CreateMoment(ctx, &store.Moment{UserID: u.ID, Date: d, Time: "08:15", Text: "coffee on the balcony", Kind: "note"})
			}
		} else {
			e := &store.Entry{UserID: u.ID, Date: d, Highlight: highlights[rng.Intn(len(highlights))], Text: texts[rng.Intn(len(texts))], Mood: 2 + rng.Intn(4), Tags: tags[rng.Intn(len(tags))] + "," + tags[rng.Intn(len(tags))], Pinned: rng.Intn(9) == 0}
			if err := s.store.UpsertEntry(ctx, e); err != nil {
				return err
			}
			for j := 0; j < rng.Intn(3); j++ {
				_ = s.store.CreateMoment(ctx, &store.Moment{UserID: u.ID, Date: d, Time: fmt.Sprintf("%02d:%02d", 8+rng.Intn(12), rng.Intn(60)), Text: []string{"coffee with A.", "call with the bank", "walked the long loop", "kids' swimming", "pushed the release", "bought flowers"}[rng.Intn(6)], Kind: store.MomentKinds[rng.Intn(len(store.MomentKinds))]})
			}
		}
		// facts
		km := sources.Round1(float64(rng.Intn(60)) + rng.Float64())
		where := sources.Where{DistanceKm: km, MovingMinutes: int(km*3) + rng.Intn(40), StillMinutes: 600, Modes: map[string]float64{"walking": sources.Round1(km * 0.2), "driving": sources.Round1(km * 0.8)}, Journeys: 2 + rng.Intn(4), Entries: []sources.Visit{}, FirstMove: "07:40", LastMove: "19:10"}
		for j := 0; j < 2+rng.Intn(3); j++ {
			where.Entries = append(where.Entries, sources.Visit{Kind: "visit", Name: places[rng.Intn(len(places))], Start: fmt.Sprintf("%02d:00", 9+j*3), End: fmt.Sprintf("%02d:30", 10+j*3), Minutes: 90})
		}
		where.Places = len(where.Entries)
		put := func(src string, payload any, head float64, sum string) {
			b, _ := json.Marshal(payload)
			_ = s.store.PutFact(ctx, &store.Fact{Date: d, Source: src, Payload: b, Headline: head, Summary: sum})
		}
		put("dawarich", where, km, fmt.Sprintf("%.1f km · %s", km, sources.Plural(where.Places, "place", "places")))
		np := rng.Intn(14)
		ph := sources.Photos{Count: np, Items: []sources.Photo{}, Places: []string{"Banja Luka"}}
		for j := 0; j < np; j++ {
			ph.Items = append(ph.Items, sources.Photo{ID: fmt.Sprintf("demo-%s-%d", d, j), Type: "IMAGE", Time: fmt.Sprintf("%02d:%02d", 8+rng.Intn(12), rng.Intn(60))})
		}
		if np > 0 {
			put("immich", ph, float64(np), sources.Plural(np, "photo", "photos"))
		}
		nl := rng.Intn(40)
		m := sources.Music{Listens: nl, Minutes: nl * 4, Items: []sources.Listen{}, Artists: []sources.Count{}, Tracks: []sources.Count{}}
		ac := map[string]int{}
		for j := 0; j < nl; j++ {
			a := artists[rng.Intn(len(artists))]
			ac[a]++
			m.Items = append(m.Items, sources.Listen{Time: fmt.Sprintf("%02d:%02d", 7+j/3, rng.Intn(60)), Artist: a, Title: fmt.Sprintf("Track %d", 1+rng.Intn(12)), Album: "Album"})
		}
		m.Artists = sources.TopCounts(ac, 5)
		if nl > 0 {
			put("maloja", m, float64(nl), fmt.Sprintf("%d listens · mostly %s", nl, m.Artists[0].Name))
		}
		if rng.Intn(3) > 0 {
			sh := shows[rng.Intn(len(shows))]
			wt := sources.Watched{Count: 1, Episodes: 1, Minutes: 45 + rng.Intn(20), Items: []sources.Play{{Title: fmt.Sprintf("Episode %d", 1+rng.Intn(9)), Series: sh, Kind: "episode", Minutes: 50, Time: "21:10", Plays: 1}}}
			put("jellystat", wt, float64(wt.Minutes), "1 episode · "+sources.HoursMinutes(wt.Minutes))
		}
		amt := sources.Round2(float64(rng.Intn(9000)) / 100)
		mo := sources.Money{Count: 1 + rng.Intn(3), Totals: map[string]float64{"EUR": amt}, Main: "EUR", Items: []sources.Spend{{Description: "Groceries", Amount: amt, Currency: "EUR", Category: "food", Time: "17:20"}}, Categories: []sources.Count{{Name: "food", Count: 1}}}
		put("firefly", mo, amt, fmt.Sprintf("%.2f EUR in %s", amt, sources.Plural(mo.Count, "transaction", "transactions")))
		hb := sources.Checkin{Done: 2 + rng.Intn(3), Total: 4, Journal: "", Habits: []sources.HabitRow{{Name: "Run", Status: 1, Scheduled: true}, {Name: "Read", Status: 1, Scheduled: true}, {Name: "No sugar", Status: -1, Scheduled: true}, {Name: "Stretch", Status: 0, Scheduled: true}}, Trackers: []sources.TrackerRow{{Name: "Sleep", Unit: "h", Value: f64(6 + rng.Float64()*2.5)}}}
		put("habits", hb, float64(hb.Done)/4*100, fmt.Sprintf("%d of 4 habits", hb.Done))
		c := conds[rng.Intn(len(conds))]
		w := sources.Day{Code: 1, Condition: c, TMax: f64(12 + rng.Float64()*14), TMin: f64(4 + rng.Float64()*8), PrecipMM: f64(float64(rng.Intn(6))), Place: "Banja Luka"}
		put("weather", w, *w.TMax, fmt.Sprintf("%s, %.0f–%.0f °C", sources.ConditionLabel(c), *w.TMin, *w.TMax))
		if rng.Intn(3) == 0 {
			ck := sources.Cooked{Cooks: []sources.Cook{{Title: []string{"Shakshuka", "Pasta al limone", "Chicken curry", "Pancakes"}[rng.Intn(4)], Servings: 2, Rating: 4}}}
			put("kitchen", ck, 1, "cooked "+ck.Cooks[0].Title)
		}
	}
	s.log.Info("demo data seeded", "user", u.Email)
	return nil
}

func f64(v float64) *float64 {
	r := sources.Round1(v)
	return &r
}
