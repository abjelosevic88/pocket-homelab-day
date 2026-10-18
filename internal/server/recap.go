package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"
	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

type monthRecap struct {
	Month    string             `json:"month"`
	Entries  int                `json:"entries"`
	Moments  int                `json:"moments"`
	Mood     float64            `json:"mood"`
	Photos   int                `json:"photos"`
	Km       float64            `json:"km"`
	Listens  int                `json:"listens"`
	WatchMin int                `json:"watch_min"`
	Spend    map[string]float64 `json:"spend"`
	Books    int                `json:"books"`
	Cooked   int                `json:"cooked"`
	Habits   float64            `json:"habits_pct"`
	Days     int                `json:"days"` // days with any facts
	SunnyPct float64            `json:"sunny_pct"`
}

type activityYear struct {
	Count   int             `json:"count"`
	Minutes float64         `json:"minutes"`
	Done    int             `json:"done"`
	Amount  float64         `json:"amount"`
	Cur     string          `json:"currency"`
	Qty     float64         `json:"qty"`
	Days    int             `json:"days"`
	Top     []sources.Count `json:"top"`
	Titles  []sources.Count `json:"titles"` // most repeated item titles
	Months  [12]float64     `json:"months"` // headline per month
	groups  map[string]int
	titles  map[string]int
	amounts map[string]float64
}

type catSpend struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
	Count  int     `json:"count"`
}

type yearRecap struct {
	Year        int                      `json:"year"`
	Months      []monthRecap             `json:"months"`
	Entries     int                      `json:"entries"`
	Moments     int                      `json:"moments"`
	Words       int                      `json:"words"`
	Mood        float64                  `json:"mood"`
	Moods       [6]int                   `json:"moods"`
	Photos      int                      `json:"photos"`
	Videos      int                      `json:"videos"`
	Km          float64                  `json:"km"`
	Modes       map[string]float64       `json:"modes"`
	Listens     int                      `json:"listens"`
	Artists     []sources.Count          `json:"artists"`
	Tracks      []sources.Count          `json:"tracks"`
	WatchMin    int                      `json:"watch_min"`
	Shows       []sources.Count          `json:"shows"`
	Movies      int                      `json:"movies"`
	Spend       map[string]float64       `json:"spend"`
	Categories  []sources.Count          `json:"categories"`
	CatSpend    []catSpend               `json:"category_spend"` // in SpendCur, largest first
	Activity    map[string]*activityYear `json:"activity"`       // per activity source
	SpendCur    string                   `json:"spend_currency"`
	Books       []sources.Book           `json:"books"`
	Cooked      int                      `json:"cooked"`
	Dishes      []sources.Count          `json:"dishes"`
	Places      []sources.Count          `json:"places"`
	Visits      []sources.Count          `json:"visits"`
	Trips       []sources.Trip           `json:"trips"`
	Conditions  []sources.Count          `json:"conditions"`
	Tags        []store.TagCount         `json:"tags"`
	Pinned      []store.Entry            `json:"pinned"`
	Best        []store.Entry            `json:"best"`
	Longest     int                      `json:"longest_streak"`
	DaysFacts   int                      `json:"days_with_facts"`
	BusiestDay  string                   `json:"busiest_day"`
	FarthestDay string                   `json:"farthest_day"`
	WeekdayMood [8]float64               `json:"weekday_mood"`
}

func (s *Server) handleYears(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	rows, err := s.store.DB.QueryContext(r.Context(), `SELECT substr(date,1,4) y, COUNT(1) FROM entries WHERE user_id = ? GROUP BY y ORDER BY y DESC`, u.ID)
	if err != nil {
		s.fail(w, err, "years")
		return
	}
	defer rows.Close()
	type yr struct {
		Year    int `json:"year"`
		Entries int `json:"entries"`
	}
	out := []yr{}
	seen := map[int]bool{}
	for rows.Next() {
		var y string
		var n int
		if rows.Scan(&y, &n) == nil {
			yi, _ := strconv.Atoi(y)
			out = append(out, yr{yi, n})
			seen[yi] = true
		}
	}
	rows.Close()
	// years that only have facts
	frows, err := s.store.DB.QueryContext(r.Context(), `SELECT DISTINCT substr(date,1,4) FROM facts WHERE payload <> ''`)
	if err == nil {
		defer frows.Close()
		for frows.Next() {
			var y string
			if frows.Scan(&y) == nil {
				yi, _ := strconv.Atoi(y)
				if !seen[yi] {
					out = append(out, yr{yi, 0})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Year > out[j].Year })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleYear(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(chiParam(r, "year"))
	if err != nil || year < 1970 || year > 2200 {
		writeErr(w, http.StatusBadRequest, "year must be a number")
		return
	}
	rec, err := s.buildYear(r.Context(), userFrom(r.Context()), year)
	if err != nil {
		s.fail(w, err, "year")
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// buildYear computes the year in review for one user.
func (s *Server) buildYear(ctx context.Context, u *store.User, year int) (*yearRecap, error) {
	ys := strconv.Itoa(year)
	from, to := ys+"-01-01", ys+"-12-31"
	rec := yearRecap{Activity: map[string]*activityYear{}, Year: year, Months: make([]monthRecap, 12), Modes: map[string]float64{}, Spend: map[string]float64{}, Books: []sources.Book{}, Trips: []sources.Trip{}, Pinned: []store.Entry{}, Best: []store.Entry{}}
	for i := range rec.Months {
		rec.Months[i] = monthRecap{Month: ys + "-" + pad2(i+1), Spend: map[string]float64{}}
	}
	entries, _, err := s.store.ListEntries(ctx, store.EntryFilter{UserID: u.ID, From: from, To: to, Limit: 100000, Asc: true})
	if err != nil {
		return nil, err
	}
	moodSum, moodN := 0.0, 0
	monthMood := make([]float64, 12)
	monthMoodN := make([]int, 12)
	wdSum := [8]float64{}
	wdN := [8]int{}
	for _, e := range entries {
		mi := monthIndex(e.Date)
		rec.Entries++
		rec.Months[mi].Entries++
		rec.Months[mi].Moments += e.Moments
		rec.Moments += e.Moments
		rec.Words += e.Words
		if e.Mood > 0 {
			rec.Moods[e.Mood]++
			moodSum += float64(e.Mood)
			moodN++
			monthMood[mi] += float64(e.Mood)
			monthMoodN[mi]++
			wd := store.WeekdayISO(e.Date)
			wdSum[wd] += float64(e.Mood)
			wdN[wd]++
		}
		if e.Pinned {
			rec.Pinned = append(rec.Pinned, e)
		}
		if e.Mood == 5 && len(rec.Best) < 12 {
			rec.Best = append(rec.Best, e)
		}
	}
	if moodN > 0 {
		rec.Mood = round1(moodSum / float64(moodN))
	}
	for i := range rec.Months {
		if monthMoodN[i] > 0 {
			rec.Months[i].Mood = round1(monthMood[i] / float64(monthMoodN[i]))
		}
	}
	for wd := 1; wd <= 7; wd++ {
		if wdN[wd] > 0 {
			rec.WeekdayMood[wd] = round1(wdSum[wd] / float64(wdN[wd]))
		}
	}
	_, rec.Longest = s.store.Streak(ctx, u.ID, store.Today())
	tags, _ := s.store.Tags(ctx, u.ID)
	if len(tags) > 15 {
		tags = tags[:15]
	}
	rec.Tags = tags

	facts, err := s.store.FactsRange(ctx, from, to)
	if err != nil {
		return nil, err
	}
	artists, tracks, shows, cats, dishes, places, visits, conds := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	daysWithFacts := map[string]bool{}
	catAmt := map[string]map[string]*catSpend{} // currency → category
	habitSum := make([]float64, 12)
	habitN := make([]int, 12)
	sunny := make([]int, 12)
	weatherN := make([]int, 12)
	bestKm, bestPhotos := 0.0, 0
	seenTrip := map[int64]bool{}
	for _, f := range facts {
		if f.Error != "" && len(f.Payload) <= 4 {
			continue
		}
		mi := monthIndex(f.Date)
		daysWithFacts[f.Date] = true
		if sources.IsActivity(f.Source) {
			var a sources.Activity
			if json.Unmarshal(f.Payload, &a) == nil && a.Count > 0 {
				ay := rec.Activity[f.Source]
				if ay == nil {
					ay = &activityYear{groups: map[string]int{}, titles: map[string]int{}, amounts: map[string]float64{}}
					rec.Activity[f.Source] = ay
				}
				ay.Count += a.Count
				ay.Minutes += a.Minutes
				ay.Done += a.Done
				ay.Qty += a.Qty
				for _, it := range a.Items {
					if it.Amount != 0 {
						ay.amounts[it.Currency] += it.Amount
					}
				}
				ay.Days++
				ay.Months[mi] += f.Headline
				for _, it := range a.Items {
					g := it.Group
					if g == "" {
						g = it.Sub
					}
					if g != "" {
						ay.groups[g]++
					}
					if it.Title != "" {
						ay.titles[it.Title]++
					}
				}
			}
			continue
		}
		switch f.Source {
		case "immich":
			var p sources.Photos
			if json.Unmarshal(f.Payload, &p) == nil {
				rec.Photos += p.Count
				rec.Videos += p.Videos
				rec.Months[mi].Photos += p.Count
				for _, pl := range p.Places {
					places[pl]++
				}
				if p.Count > bestPhotos {
					bestPhotos, rec.BusiestDay = p.Count, f.Date
				}
			}
		case "dawarich":
			var wh sources.Where
			if json.Unmarshal(f.Payload, &wh) == nil {
				rec.Km += wh.DistanceKm
				rec.Months[mi].Km += wh.DistanceKm
				for k, v := range wh.Modes {
					rec.Modes[k] += v
				}
				for _, e := range wh.Entries {
					if e.Kind == "visit" && e.Name != "" && !strings.HasPrefix(strings.ToLower(e.Name), "unknown") {
						visits[e.Name]++
					}
				}
				if wh.DistanceKm > bestKm {
					bestKm, rec.FarthestDay = wh.DistanceKm, f.Date
				}
			}
		case "maloja":
			var m sources.Music
			if json.Unmarshal(f.Payload, &m) == nil {
				rec.Listens += m.Listens
				rec.Months[mi].Listens += m.Listens
				for _, l := range m.Items {
					artists[l.Artist]++
					tracks[l.Artist+" – "+l.Title]++
				}
			}
		case "jellystat":
			var wt sources.Watched
			if json.Unmarshal(f.Payload, &wt) == nil {
				rec.WatchMin += wt.Minutes
				rec.Months[mi].WatchMin += wt.Minutes
				for _, it := range wt.Items {
					if it.Series != "" {
						shows[it.Series]++
					} else {
						rec.Movies++
					}
				}
			}
		case "firefly":
			var mo sources.Money
			if json.Unmarshal(f.Payload, &mo) == nil {
				for cur, v := range mo.Totals {
					rec.Spend[cur] += v
					rec.Months[mi].Spend[cur] += v
				}
				for _, c := range mo.Categories {
					cats[c.Name] += c.Count
				}
				for _, it := range mo.Items {
					name := it.Category
					if name == "" {
						name = "uncategorised"
					}
					if catAmt[it.Currency] == nil {
						catAmt[it.Currency] = map[string]*catSpend{}
					}
					cs := catAmt[it.Currency][name]
					if cs == nil {
						cs = &catSpend{Name: name}
						catAmt[it.Currency][name] = cs
					}
					cs.Amount += it.Amount
					cs.Count++
				}
			}
		case "bookshelf":
			var rd sources.Reading
			if json.Unmarshal(f.Payload, &rd) == nil {
				for _, b := range rd.Finished {
					rec.Books = append(rec.Books, b)
					rec.Months[mi].Books++
				}
			}
		case "kitchen":
			var ck sources.Cooked
			if json.Unmarshal(f.Payload, &ck) == nil {
				rec.Cooked += len(ck.Cooks)
				rec.Months[mi].Cooked += len(ck.Cooks)
				for _, c := range ck.Cooks {
					dishes[c.Title]++
				}
			}
		case "habits":
			var c sources.Checkin
			if json.Unmarshal(f.Payload, &c) == nil && c.Total > 0 {
				habitSum[mi] += float64(c.Done) / float64(c.Total) * 100
				habitN[mi]++
			}
		case "travel":
			var t sources.Trips
			if json.Unmarshal(f.Payload, &t) == nil {
				for _, tr := range t.Trips {
					if !seenTrip[tr.ID] {
						seenTrip[tr.ID] = true
						rec.Trips = append(rec.Trips, tr)
					}
				}
			}
		case "weather":
			var d sources.Day
			if json.Unmarshal(f.Payload, &d) == nil && d.Condition != "" {
				conds[d.Condition]++
				weatherN[mi]++
				if d.Condition == "clear" || d.Condition == "partly" {
					sunny[mi]++
				}
			}
		}
	}
	for i := range rec.Months {
		if habitN[i] > 0 {
			rec.Months[i].Habits = round1(habitSum[i] / float64(habitN[i]))
		}
		if weatherN[i] > 0 {
			rec.Months[i].SunnyPct = round1(float64(sunny[i]) / float64(weatherN[i]) * 100)
		}
		rec.Months[i].Km = round1(rec.Months[i].Km)
		for cur, v := range rec.Months[i].Spend {
			rec.Months[i].Spend[cur] = sources.Round2(v)
		}
	}
	for d := range daysWithFacts {
		rec.Months[monthIndex(d)].Days++
	}
	rec.DaysFacts = len(daysWithFacts)
	rec.Km = round1(rec.Km)
	for k, v := range rec.Modes {
		rec.Modes[k] = round1(v)
	}
	for cur, v := range rec.Spend {
		rec.Spend[cur] = sources.Round2(v)
	}
	rec.Artists = sources.TopCounts(artists, 10)
	rec.Tracks = sources.TopCounts(tracks, 10)
	rec.Shows = sources.TopCounts(shows, 8)
	for _, ay := range rec.Activity {
		for cur, v := range ay.amounts {
			if ay.Cur == "" || v > ay.Amount {
				ay.Cur, ay.Amount = cur, v
			}
		}
		ay.Top = sources.TopCounts(ay.groups, 8)
		ay.Titles = sources.TopCounts(ay.titles, 5)
		ay.Minutes = round1(ay.Minutes)
		ay.Amount = sources.Round2(ay.Amount)
	}
	rec.Categories = sources.TopCounts(cats, 8)
	for cur, v := range rec.Spend {
		if rec.SpendCur == "" || v > rec.Spend[rec.SpendCur] {
			rec.SpendCur = cur
		}
	}
	rec.CatSpend = []catSpend{}
	for _, cs := range catAmt[rec.SpendCur] {
		cs.Amount = sources.Round2(cs.Amount)
		rec.CatSpend = append(rec.CatSpend, *cs)
	}
	sort.Slice(rec.CatSpend, func(i, j int) bool { return rec.CatSpend[i].Amount > rec.CatSpend[j].Amount })
	if len(rec.CatSpend) > 10 {
		rec.CatSpend = rec.CatSpend[:10]
	}
	rec.Dishes = sources.TopCounts(dishes, 8)
	rec.Places = sources.TopCounts(places, 10)
	rec.Visits = sources.TopCounts(visits, 10)
	rec.Conditions = sources.TopCounts(conds, 7)
	sort.SliceStable(rec.Trips, func(i, j int) bool { return rec.Trips[i].StartDate < rec.Trips[j].StartDate })
	return &rec, nil
}

func monthIndex(date string) int {
	if len(date) < 7 {
		return 0
	}
	m, _ := strconv.Atoi(date[5:7])
	if m < 1 || m > 12 {
		return 0
	}
	return m - 1
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func round1(f float64) float64 { return sources.Round1(f) }
