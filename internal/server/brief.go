package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

// The AI never sees raw payloads: every day is turned into a few plain lines
// here. "detail" adds names (stops, artists, shows, purchases, habits) for the
// day page features and the days an answer is built on; the overviews of a
// week, a month or a long search use the short form.

var moodWords = []string{"", "rough", "meh", "okay", "good", "great"}

// factLines renders a day's facts as "Label: …" lines in a fixed order.
func factLines(facts map[string]store.Fact, detail bool) []string {
	var out []string
	add := func(label, text string) {
		if strings.TrimSpace(text) != "" {
			out = append(out, label+": "+text)
		}
	}
	get := func(src string, v any) bool {
		f, ok := facts[src]
		if !ok || len(f.Payload) <= 4 {
			return false
		}
		return json.Unmarshal(f.Payload, v) == nil
	}
	var tr sources.Trips
	if get("travel", &tr) && len(tr.Trips) > 0 {
		t := tr.Trips[0]
		add("Trip", fmt.Sprintf("%s (%s), day %d of %d", t.Name, t.Destination, t.DayNo, t.Days))
	}
	var wd sources.Day
	if get("weather", &wd) && wd.Condition != "" {
		s := sources.ConditionLabel(wd.Condition)
		if wd.TMin != nil && wd.TMax != nil {
			s += fmt.Sprintf(", %.0f–%.0f °C", *wd.TMin, *wd.TMax)
		}
		if wd.PrecipMM != nil && *wd.PrecipMM >= 1 {
			s += fmt.Sprintf(", %.0f mm rain", *wd.PrecipMM)
		}
		if wd.Place != "" {
			s += " in " + wd.Place
		}
		add("Weather", s)
	}
	var wh sources.Where
	if get("dawarich", &wh) && wh.DistanceKm > 0 {
		s := fmt.Sprintf("%.1f km in total", wh.DistanceKm)
		var modes []string
		for m, v := range wh.Modes {
			if v >= 0.5 {
				modes = append(modes, fmt.Sprintf("%s %.1f km", m, v))
			}
		}
		sort.Strings(modes)
		if len(modes) > 0 {
			s += " — by mode: " + strings.Join(modes, ", ")
		}
		if wh.FirstMove != "" {
			s += fmt.Sprintf(", out %s–%s", wh.FirstMove, wh.LastMove)
		}
		if detail {
			var stops []string
			for _, e := range wh.Entries {
				if e.Kind == "visit" && e.Name != "" {
					stops = append(stops, fmt.Sprintf("%s %s (%d min)", e.Start, e.Name, e.Minutes))
				}
			}
			if len(stops) > 0 {
				s += "; stops: " + strings.Join(stops, ", ")
			}
		}
		add("Moved", s)
	}
	var ph sources.Photos
	if get("immich", &ph) && ph.Count > 0 {
		s := sources.Plural(ph.Count, "photo", "photos")
		if len(ph.Places) > 0 {
			s += " in " + strings.Join(ph.Places, ", ")
		}
		if ph.First != "" {
			s += fmt.Sprintf(" (%s–%s)", ph.First, ph.Last)
		}
		add("Photos", s)
	}
	var mu sources.Music
	if get("maloja", &mu) && mu.Listens > 0 {
		s := sources.Plural(mu.Listens, "song", "songs")
		var a []string
		for _, c := range mu.Artists {
			a = append(a, c.Name)
			if !detail && len(a) == 2 {
				break
			}
		}
		if len(a) > 0 {
			s += ", mostly " + strings.Join(a, ", ")
		}
		add("Music", s)
	}
	var wt sources.Watched
	if get("jellystat", &wt) && wt.Count > 0 {
		var it []string
		for _, p := range wt.Items {
			if p.Series != "" {
				it = append(it, p.Series+" – "+p.Title)
			} else {
				it = append(it, p.Title+" (film)")
			}
		}
		if !detail && len(it) > 3 {
			it = append(it[:3], "…")
		}
		add("Watched", strings.Join(it, "; ")+" · "+sources.HoursMinutes(wt.Minutes))
	}
	var rd sources.Reading
	if get("bookshelf", &rd) {
		var b []string
		for _, x := range rd.Finished {
			b = append(b, "finished “"+x.Title+"” by "+x.Authors)
		}
		for _, x := range rd.Started {
			b = append(b, "started “"+x.Title+"”")
		}
		for _, x := range rd.Sessions {
			what := fmt.Sprintf("%d pages", x.Pages)
			if x.Pages == 0 {
				what = fmt.Sprintf("%d%% of the book", x.Percent)
			}
			b = append(b, fmt.Sprintf("read “%s” %s–%s: %s in %s", x.Title, x.Time, x.End, what, sources.HoursMinutes(x.Minutes)))
		}
		if detail {
			for _, x := range rd.Reading {
				b = append(b, "reading “"+x.Title+"”")
			}
		}
		add("Books", strings.Join(b, "; "))
	}
	var ck sources.Cooked
	if get("kitchen", &ck) && len(ck.Cooks) > 0 {
		var d []string
		for _, c := range ck.Cooks {
			d = append(d, c.Title)
		}
		add("Cooked", strings.Join(d, ", "))
	}
	var mo sources.Money
	if get("firefly", &mo) && mo.Count > 0 {
		var tot []string
		for c, v := range mo.Totals {
			tot = append(tot, fmt.Sprintf("%.2f %s", v, c))
		}
		sort.Strings(tot)
		s := strings.Join(tot, " + ")
		if detail {
			var it []string
			for _, x := range mo.Items {
				it = append(it, fmt.Sprintf("%s %.2f", x.Description, x.Amount))
			}
			if len(it) > 0 {
				s += " (" + strings.Join(it, "; ") + ")"
			}
		}
		add("Spent", s)
	}
	var hd sources.HealthDay
	if get("health", &hd) {
		var p []string
		if hd.Steps != nil {
			p = append(p, fmt.Sprintf("%d steps", int(*hd.Steps)))
		}
		if hd.Sleep != nil {
			p = append(p, "slept "+sources.HoursMinutes(hd.Sleep.AsleepMin)+" ("+hd.Sleep.Bed+"–"+hd.Sleep.Wake+")")
		}
		for _, w := range hd.Workouts {
			x := fmt.Sprintf("%s %d min", strings.ReplaceAll(w.Kind, "_", " "), w.Minutes)
			if detail {
				if w.DistanceKm > 0 {
					x += fmt.Sprintf(" %.1f km", w.DistanceKm)
				}
				if w.AvgHR > 0 {
					x += fmt.Sprintf(" avg HR %d", w.AvgHR)
				}
			}
			p = append(p, x)
		}
		if hd.RestingHR != nil {
			p = append(p, fmt.Sprintf("resting HR %d", int(*hd.RestingHR)))
		}
		if hd.WeightKg != nil {
			p = append(p, fmt.Sprintf("%.1f kg", *hd.WeightKg))
		}
		if detail && len(hd.GoalsMet) > 0 {
			p = append(p, "goals met: "+strings.Join(hd.GoalsMet, ", "))
		}
		if len(p) > 0 {
			add("Health", strings.Join(p, ", "))
		}
	}
	var sd sources.StillDay
	if get("still", &sd) && sd.Sessions > 0 {
		var p []string
		for _, x := range sd.Practices {
			s := fmt.Sprintf("%s %.0f min", strings.ToLower(x.Title), x.Minutes)
			if detail && x.CalmBefore != nil && x.CalmAfter != nil {
				s += fmt.Sprintf(" (calm %d→%d)", *x.CalmBefore, *x.CalmAfter)
			}
			p = append(p, s)
		}
		if sd.Streak > 1 {
			p = append(p, fmt.Sprintf("day %d of a streak", sd.Streak))
		}
		for _, l := range sd.Lessons {
			p = append(p, "lesson \""+l.Title+"\"")
		}
		if detail && len(sd.Milestones) > 0 {
			p = append(p, "milestones: "+strings.Join(sd.Milestones, ", "))
		}
		add("Practice", strings.Join(p, ", "))
	}
	var hb sources.Checkin
	if get("habits", &hb) {
		var s []string
		if hb.Total > 0 {
			h := fmt.Sprintf("%d of %d habits", hb.Done, hb.Total)
			if detail {
				var done, missed []string
				for _, x := range hb.Habits {
					if !x.Scheduled {
						continue
					}
					switch x.Status {
					case 1:
						done = append(done, x.Name)
					case -1:
						missed = append(missed, x.Name)
					}
				}
				if len(done) > 0 {
					h += "; done: " + strings.Join(done, ", ")
				}
				if len(missed) > 0 {
					h += "; missed: " + strings.Join(missed, ", ")
				}
			}
			s = append(s, h)
		}
		if detail {
			for _, t := range hb.Trackers {
				if t.Value == nil || t.Defaulted {
					continue
				}
				switch {
				case t.Kind == "yesno":
					ans := "no"
					if *t.Value > 0 {
						ans = "yes"
					}
					s = append(s, t.Name+" "+ans)
				case t.Kind == "scale" && t.Max != nil:
					s = append(s, fmt.Sprintf("%s %v/%v", t.Name, *t.Value, *t.Max))
				default:
					s = append(s, fmt.Sprintf("%s %v%s", t.Name, *t.Value, unitSuffix(t.Unit)))
				}
			}
		}
		if hb.Journal != "" {
			s = append(s, "habit-journal line: “"+hb.Journal+"”")
		}
		add("Habits", strings.Join(s, "; "))
	}
	for _, name := range activityOrder() {
		var a sources.Activity
		if !get(name, &a) || a.Count == 0 {
			continue
		}
		m := sources.Activities[name]
		line := sources.Plural(a.Count, m.One, m.Many)
		if a.Minutes >= 1 {
			line += ", " + sources.HoursMinutes(int(a.Minutes+0.5))
		}
		if a.Amount > 0 {
			line += fmt.Sprintf(", %.2f %s", a.Amount, a.Cur)
		}
		n := 3
		if detail {
			n = 12
		}
		var it []string
		for i, x := range a.Items {
			if i >= n {
				it = append(it, "…")
				break
			}
			t := x.Title
			if x.Sub != "" {
				t += " (" + x.Sub + ")"
			}
			if detail && x.Time != "" {
				t = x.Time + " " + t
			}
			if x.Done {
				t += " ✓"
			}
			if x.Note != "" {
				t += " — " + x.Note
			}
			it = append(it, t)
		}
		if len(it) > 0 {
			line += ": " + strings.Join(it, "; ")
		}
		add(m.Label, line)
	}
	var car sources.Car
	if get("garage", &car) && len(car.Items) > 0 {
		var it []string
		for _, x := range car.Items {
			it = append(it, x.Vehicle+" "+x.Kind+" "+x.Title)
		}
		add("Car", strings.Join(it, "; "))
	}
	return out
}

func unitSuffix(u string) string {
	if u == "" {
		return ""
	}
	return " " + u
}

// dayBrief is one day for the AI: the user's words (when allowed), moments, pins and facts.
func (s *Server) dayBrief(ctx context.Context, u *store.User, date string, sendText, detail bool, maxText int) string {
	var b strings.Builder
	t, _ := store.ParseDate(date)
	fmt.Fprintf(&b, "## %s (%s)\n", date, t.Weekday())
	if e, err := s.store.GetEntry(ctx, u.ID, date); err == nil {
		if e.Mood > 0 {
			fmt.Fprintf(&b, "Mood: %s\n", moodWords[e.Mood])
		}
		if sendText {
			if e.Highlight != "" {
				fmt.Fprintf(&b, "Highlight: %s\n", e.Highlight)
			}
			if e.Text != "" {
				fmt.Fprintf(&b, "Wrote: %s\n", clip(e.Text, maxText))
			}
		}
		if e.Tags != "" {
			fmt.Fprintf(&b, "Tags: %s\n", e.Tags)
		}
		if e.Pinned {
			b.WriteString("Starred as a day worth remembering\n")
		}
	}
	if sendText {
		if ms, _ := s.store.ListMoments(ctx, u.ID, date, date, 0); len(ms) > 0 {
			var l []string
			for _, m := range ms {
				x := m.Text
				if m.Time != "" {
					x = m.Time + " " + x
				}
				if m.Kind != "note" {
					x += " (" + m.Kind + ")"
				}
				l = append(l, x)
			}
			fmt.Fprintf(&b, "Moments: %s\n", strings.Join(l, "; "))
		}
	}
	if ps, _ := s.store.ListPins(ctx, u.ID, date, date); len(ps) > 0 {
		var l []string
		for _, p := range ps {
			l = append(l, strings.TrimSpace(p.Title+" "+p.Subtitle))
		}
		fmt.Fprintf(&b, "Kept: %s\n", strings.Join(l, "; "))
	}
	facts, _ := s.store.FactsForDay(ctx, u.ID, date)
	for _, l := range factLines(facts, detail) {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// hasContent reports whether a brief says anything beyond its heading.
func hasContent(brief string) bool { return strings.Count(brief, "\n") > 1 }

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// languageRule tells the model which language to write in.
func (s *Server) languageRule(ctx context.Context, u *store.User, st store.Settings) string {
	if strings.TrimSpace(st.AILanguage) != "" {
		return "Write in " + st.AILanguage + "."
	}
	recent, _, _ := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, Limit: 5})
	var sample []string
	for _, e := range recent {
		if t := strings.TrimSpace(e.Highlight + " " + e.Text); t != "" {
			sample = append(sample, clip(t, 160))
		}
	}
	if len(sample) == 0 {
		return "Write in English."
	}
	return "Write in the same language (and script) the person uses in their own entries — for example: “" + strings.Join(sample, "” / “") + "”."
}

// activityOrder is the activity sources in a stable order.
func activityOrder() []string {
	var out []string
	for _, n := range store.SourceNames {
		if sources.IsActivity(n) {
			out = append(out, n)
		}
	}
	return out
}
