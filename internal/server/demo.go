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
		if err := s.seedOwner(ctx); err != nil {
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
			_ = s.store.PutFact(ctx, u.ID, &store.Fact{Date: d, Source: src, Payload: b, Headline: head, Summary: sum})
		}
		put("dawarich", where, km, fmt.Sprintf("%.1f km · %s", km, sources.Plural(where.Places, "place", "places")))
		np := rng.Intn(14)
		ph := sources.Photos{Count: np, Items: []sources.Photo{}, Places: []string{"Lisbon"}}
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
			m.Items = append(m.Items, sources.Listen{Time: fmt.Sprintf("%02d:%02d", 7+j/3, rng.Intn(60)), Artist: a, Title: demoSongs[a][rng.Intn(len(demoSongs[a]))], Album: ""})
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
		hb := demoCheckin(rng, 45-i)
		put("habits", hb, float64(hb.Done)/float64(hb.Total)*100, fmt.Sprintf("%d of %d habits", hb.Done, hb.Total))
		c := conds[rng.Intn(len(conds))]
		w := sources.Day{Code: 1, Condition: c, TMax: f64(12 + rng.Float64()*14), TMin: f64(4 + rng.Float64()*8), PrecipMM: f64(float64(rng.Intn(6))), Place: "Lisbon"}
		put("weather", w, *w.TMax, fmt.Sprintf("%s, %.0f–%.0f °C", sources.ConditionLabel(c), *w.TMin, *w.TMax))
		for src, a := range demoActivities(rng, d, i) {
			if a.Count == 0 {
				continue
			}
			head := float64(a.Count)
			switch sources.Activities[src].Unit {
			case "min":
				head = a.Minutes
			case "qty":
				head = a.Qty
			}
			m := sources.Activities[src]
			sum := sources.ActivitySummary(a, m.One, m.Many)
			if m.Unit == "qty" {
				sum = fmt.Sprintf("%d commits in %s", int(a.Qty), sources.Plural(a.Count, "repository", "repositories"))
			}
			put(src, a, head, sum)
		}
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

// demoCheckin is a plausible Pocket Habits day: five habits with streaks,
// two 1–5 scales, a yes/no where "no" is good, and a number.
func demoCheckin(rng *rand.Rand, n int) sources.Checkin {
	names := []string{"Run", "Read 20 pages", "No sugar", "Stretch", "In bed by 23:00"}
	c := sources.Checkin{Total: len(names), Habits: []sources.HabitRow{}, Trackers: []sources.TrackerRow{}}
	for i, name := range names {
		st := []int{1, 1, 1, -1, 0}[rng.Intn(5)]
		if i == 1 { // reading almost always happens
			st = 1
		}
		if st == 1 {
			c.Done++
		}
		c.Habits = append(c.Habits, sources.HabitRow{ID: int64(i + 1), Name: name, Status: st, Scheduled: true, Streak: (n * (i + 2)) % 9})
	}
	one, five, zero := 1.0, 5.0, 0.0
	yes, no := true, false
	scale := func(id int64, name string) sources.TrackerRow {
		v, p := float64(2+rng.Intn(4)), float64(2+rng.Intn(4))
		return sources.TrackerRow{ID: id, Name: name, Kind: "scale", Value: &v, Previous: &p, Min: &one, Max: &five, HigherIsBetter: &yes}
	}
	c.Trackers = append(c.Trackers, scale(1, "Sleep quality"), scale(2, "Energy"))
	hv := float64(rng.Intn(5) / 4) // mostly no headache
	c.Trackers = append(c.Trackers, sources.TrackerRow{ID: 3, Name: "Headache?", Kind: "yesno", Value: &hv, Min: &zero, Max: &one, HigherIsBetter: &no})
	water := float64(4 + rng.Intn(5))
	pw := water - 1
	c.Trackers = append(c.Trackers, sources.TrackerRow{ID: 4, Name: "Water", Kind: "number", Unit: "glasses", Value: &water, Previous: &pw, HigherIsBetter: &yes})
	return c
}

var demoSongs = map[string][]string{
	"Boards of Canada": {"Roygbiv", "Dayvan Cowboy", "Olson", "Turquoise Hexagon Sun"},
	"Khruangbin":       {"Maria También", "White Gloves", "People Everywhere", "August 10"},
	"Nils Frahm":       {"Says", "Ambre", "Familiar", "All Melody"},
	"Radiohead":        {"Weird Fishes", "Reckoner", "Daydreaming", "No Surprises"},
	"Bonobo":           {"Kerala", "Cirrus", "Kiara", "First Fires"},
	"Tycho":            {"Awake", "Dive", "A Walk", "Epoch"},
}

// demoActivities invents a plausible day for the activity sources.
func demoActivities(rng *rand.Rand, date string, daysAgo int) map[string]sources.Activity {
	out := map[string]sources.Activity{}
	clock := func(h int) string { return fmt.Sprintf("%02d:%02d", h, rng.Intn(60)) }
	// podcasts on most days
	if rng.Intn(4) > 0 {
		shows := []struct{ show, ep string }{{"Hard Fork", "The AI agents are coming for your calendar"}, {"99% Invisible", "The Smell of Concrete After Rain"}, {"Radiolab", "The Clock That Kept Lisbon"}, {"Decoder", "Why every app wants to be a super app"}, {"Song Exploder", "Khruangbin – Maria También"}}
		a := sources.Activity{}
		perm := rng.Perm(len(shows))
		for k := 0; k < 1+rng.Intn(3); k++ {
			x := shows[perm[k]] // a different episode each time
			mins := float64(12 + rng.Intn(50))
			a.Items = append(a.Items, sources.ActivityItem{Time: clock(7 + k*5), Title: x.ep, Sub: x.show, Minutes: mins, Done: rng.Intn(2) == 0, Ref: fmt.Sprintf("%s-%d", date, k)})
		}
		a.Finish()
		out["pinepods"] = a
	}
	// an audiobook in the evening, now and then
	if rng.Intn(3) == 0 {
		a := sources.Activity{Items: []sources.ActivityItem{{Time: clock(21), Title: "Project Hail Mary", Sub: "Andy Weir", Minutes: float64(20 + rng.Intn(60)), Ref: "phm"}}}
		a.Finish()
		out["audiobookshelf"] = a
	}
	// commits on weekdays
	if t, _ := store.ParseDate(date); t.Weekday() != 0 && t.Weekday() != 6 && rng.Intn(4) > 0 {
		repos := []struct{ repo, msg string }{{"pocket-day", "Calendar: colour the days by podcasts"}, {"homelab", "Back up the new app"}, {"dotfiles", "Tidy the shell prompt"}, {"garden-planner", "Frost dates for Lisbon"}}
		a := sources.Activity{}
		perm := rng.Perm(len(repos))
		for k := 0; k < 1+rng.Intn(3); k++ {
			r := repos[perm[k]]
			n := 1 + rng.Intn(9)
			a.Items = append(a.Items, sources.ActivityItem{Time: clock(9 + k*3), End: clock(12 + k*3), Title: r.repo, Sub: sources.Plural(n, "commit", "commits") + " · " + r.msg, Group: r.repo, Qty: float64(n), Ref: r.repo + fmt.Sprint(k)})
		}
		a.Finish()
		out["forgejo"] = a
	}
	// tasks done
	if rng.Intn(3) > 0 {
		tasks := []struct{ t, list string }{{"Buy milk and eggs", "Shopping"}, {"Call the dentist", "Personal"}, {"Renew the car insurance", "Personal"}, {"Water the balcony plants", "Home"}, {"Pick up the parcel", "Personal"}, {"Book the train to Porto", "Travel"}}
		a := sources.Activity{}
		tperm := rng.Perm(len(tasks))
		for k := 0; k < 1+rng.Intn(3); k++ {
			x := tasks[tperm[k]]
			a.Items = append(a.Items, sources.ActivityItem{Time: clock(10 + k*3), Title: x.t, Sub: x.list, Done: true})
		}
		a.Finish()
		out["nextcloud"] = a
	}
	// requests, a few times a week
	if rng.Intn(3) == 0 {
		titles := []string{"Severance", "Dune: Part Two", "The Bear", "Arrival", "Slow Horses", "Past Lives"}
		x := titles[rng.Intn(len(titles))]
		a := sources.Activity{Items: []sources.ActivityItem{{Time: clock(19), Title: x, Sub: "requested film", Group: "film"}}}
		if rng.Intn(2) == 0 {
			a.Items = append(a.Items, sources.ActivityItem{Time: clock(22), Title: x, Sub: "now available", Group: "film", Done: true})
		}
		a.Finish()
		out["jellyseerr"] = a
	}
	// a document once a week
	if daysAgo%7 == 3 {
		a := sources.Activity{Items: []sources.ActivityItem{{Time: clock(18), Title: "Electricity bill", Sub: "EDP · Invoice", Group: "Invoice"}}}
		a.Finish()
		out["documents"] = a
	}
	return out
}
