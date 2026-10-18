package server

import (
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-day/internal/sources"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-day/internal/store"
)

func chiParam(r *http.Request, name string) string { return chi.URLParam(r, name) }

// dayPage is everything the day screen shows.
type dayPage struct {
	Date      string                `json:"date"`
	Weekday   int                   `json:"weekday"`
	Today     bool                  `json:"today"`
	Future    bool                  `json:"future"`
	Entry     *store.Entry          `json:"entry"`
	Moments   []store.Moment        `json:"moments"`
	Pins      []store.Pin           `json:"pins"`
	Facts     map[string]store.Fact `json:"facts"`
	Sources   []string              `json:"sources"` // configured, in display order
	Memories  []store.Entry         `json:"memories"`
	PrevEntry string                `json:"prev_entry"`
	NextEntry string                `json:"next_entry"`
	Streak    int                   `json:"streak"`
	Prompt    string                `json:"prompt"`
	Links     map[string]string     `json:"links"` // source → public URL for "open in …"
}

func (s *Server) buildDay(r *http.Request, date string) (*dayPage, error) {
	ctx := r.Context()
	u := userFrom(ctx)
	today := store.Today()
	if date <= today {
		s.ensureDay(ctx, date)
	}
	st, _ := s.store.GetSettings(ctx)
	page := &dayPage{Date: date, Weekday: store.WeekdayISO(date), Today: date == today, Future: date > today, Moments: []store.Moment{}, Pins: []store.Pin{}, Facts: map[string]store.Fact{}, Sources: []string{}, Memories: []store.Entry{}, Prompt: st.Prompt, Links: map[string]string{}}
	if e, err := s.store.GetEntry(ctx, u.ID, date); err == nil {
		page.Entry = e
	}
	var err error
	if page.Moments, err = s.store.ListMoments(ctx, u.ID, date, date, 0); err != nil {
		return nil, err
	}
	if page.Pins, err = s.store.ListPins(ctx, u.ID, date, date); err != nil {
		return nil, err
	}
	if page.Facts, err = s.store.FactsForDay(ctx, date); err != nil {
		return nil, err
	}
	for _, n := range store.SourceNames {
		src := st.Sources[n]
		if src.Configured(n) && (n != "weather" || st.HomeLat != 0 || st.HomeLon != 0) {
			page.Sources = append(page.Sources, n)
			if src.PublicURL != "" {
				page.Links[n] = src.PublicURL
			} else if src.URL != "" && !strings.Contains(src.URL, "host.docker.internal") && !strings.Contains(src.URL, "localhost") {
				page.Links[n] = src.URL
			}
		}
	}
	page.Memories, _ = s.store.Memories(ctx, u.ID, date)
	page.PrevEntry, page.NextEntry = s.store.Neighbours(ctx, u.ID, date)
	page.Streak, _ = s.store.Streak(ctx, u.ID, today)
	return page, nil
}

func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	page, err := s.buildDay(r, store.Today())
	if err != nil {
		s.fail(w, err, "load today")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	page, err := s.buildDay(r, date)
	if err != nil {
		s.fail(w, err, "load day")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handlePutEntry writes the user's words for a day. Only the sent fields change.
func (s *Server) handlePutEntry(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if date > store.Today() {
		writeErr(w, http.StatusBadRequest, "you can only write about days that have happened")
		return
	}
	u := userFrom(r.Context())
	var in store.Entry
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := s.store.GetEntry(r.Context(), u.ID, date)
	if err != nil {
		cur = &store.Entry{UserID: u.ID, Date: date}
	}
	if keys["highlight"] {
		cur.Highlight = strings.TrimSpace(in.Highlight)
	}
	if keys["text"] {
		cur.Text = in.Text
	}
	if keys["mood"] {
		if in.Mood < 0 || in.Mood > 5 {
			writeErr(w, http.StatusBadRequest, "mood is 0 (unset) to 5")
			return
		}
		cur.Mood = in.Mood
	}
	if keys["tags"] {
		cur.Tags = store.NormalizeTags(in.Tags)
	}
	if keys["pinned"] {
		cur.Pinned = in.Pinned
	}
	if err := s.store.UpsertEntry(r.Context(), cur); err != nil {
		s.fail(w, err, "save entry")
		return
	}
	if cur.ID == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"entry": nil, "date": date})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entry": cur, "date": date})
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	u := userFrom(r.Context())
	if err := s.store.UpsertEntry(r.Context(), &store.Entry{UserID: u.ID, Date: date}); err != nil {
		s.fail(w, err, "delete entry")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleQuickEntry is the shortcut form: {"date"?, "highlight"?, "text"?, "mood"?, "tags"?}
// appends text to today's entry instead of replacing it.
func (s *Server) handleQuickEntry(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in struct {
		Date      string `json:"date"`
		Highlight string `json:"highlight"`
		Text      string `json:"text"`
		Mood      int    `json:"mood"`
		Tags      string `json:"tags"`
		Pinned    *bool  `json:"pinned"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.Date == "" {
		in.Date = store.Today()
	}
	if !validDate(in.Date) || in.Date > store.Today() {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD and not in the future")
		return
	}
	cur, err := s.store.GetEntry(r.Context(), u.ID, in.Date)
	if err != nil {
		cur = &store.Entry{UserID: u.ID, Date: in.Date}
	}
	if h := strings.TrimSpace(in.Highlight); h != "" {
		cur.Highlight = h
	}
	if t := strings.TrimSpace(in.Text); t != "" {
		if cur.Text != "" {
			cur.Text += "\n\n"
		}
		cur.Text += t
	}
	if in.Mood >= 1 && in.Mood <= 5 {
		cur.Mood = in.Mood
	}
	if in.Tags != "" {
		cur.Tags = store.NormalizeTags(cur.Tags + "," + in.Tags)
	}
	if in.Pinned != nil {
		cur.Pinned = *in.Pinned
	}
	if err := s.store.UpsertEntry(r.Context(), cur); err != nil {
		s.fail(w, err, "save entry")
		return
	}
	writeJSON(w, http.StatusOK, cur)
}

func (s *Server) handleListEntries(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	q := r.URL.Query()
	f := store.EntryFilter{UserID: u.ID, Query: q.Get("q"), Tag: q.Get("tag"), Mood: queryInt(r, "mood"), Pinned: queryBool(r, "pinned"), From: q.Get("from"), To: q.Get("to"), Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset"), Asc: q.Get("order") == "asc"}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 30
	}
	entries, total, err := s.store.ListEntries(r.Context(), f)
	if err != nil {
		s.fail(w, err, "list entries")
		return
	}
	// one-line summaries per day for the list
	sums := map[string]map[string]string{}
	if len(entries) > 0 {
		from, to := entries[0].Date, entries[0].Date
		for _, e := range entries {
			if e.Date < from {
				from = e.Date
			}
			if e.Date > to {
				to = e.Date
			}
		}
		facts, _ := s.store.FactsRange(r.Context(), from, to)
		for _, fc := range facts {
			if fc.Summary == "" || fc.Error != "" {
				continue
			}
			if sums[fc.Date] == nil {
				sums[fc.Date] = map[string]string{}
			}
			sums[fc.Date][fc.Source] = fc.Summary
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total, "summaries": sums, "limit": f.Limit, "offset": f.Offset})
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.Tags(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err, "tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = store.Today()
	}
	if !validDate(date) {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	m, err := s.store.Memories(r.Context(), userFrom(r.Context()).ID, date)
	if err != nil {
		s.fail(w, err, "memories")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// ---- calendar ----

type calendarDay struct {
	Date      string             `json:"date"`
	Entry     bool               `json:"entry"`
	Mood      int                `json:"mood"`
	Highlight string             `json:"highlight"`
	Pinned    bool               `json:"pinned"`
	Moments   int                `json:"moments"`
	Headlines map[string]float64 `json:"headlines"`
	Weather   string             `json:"weather,omitempty"`
	TMax      *float64           `json:"t_max,omitempty"`
	Cover     string             `json:"cover,omitempty"`   // Immich asset id for the cell background
	Summary   string             `json:"summary,omitempty"` // the day in one line, from the facts
}

// pickCover chooses the photo that represents a day: a favourite, else a
// camera photo from the middle of the day (screenshots are PNGs), else any image.
func pickCover(items []sources.Photo) string {
	var images, camera []sources.Photo
	for _, p := range items {
		if p.Type != "IMAGE" {
			continue
		}
		if p.Favorite {
			return p.ID
		}
		images = append(images, p)
		if !strings.HasSuffix(strings.ToLower(p.FileName), ".png") {
			camera = append(camera, p)
		}
	}
	if len(camera) > 0 {
		return camera[len(camera)/2].ID
	}
	if len(images) > 0 {
		return images[len(images)/2].ID
	}
	return ""
}

func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if m := r.URL.Query().Get("month"); len(m) == 7 {
		from = m + "-01"
		to = store.AddDays(store.AddDays(from, 31)[:7]+"-01", -1) // last day of month
	}
	if !validDate(from) || !validDate(to) || to < from {
		writeErr(w, http.StatusBadRequest, "pass month=YYYY-MM or from/to")
		return
	}
	if store.AddDays(from, 400) < to {
		writeErr(w, http.StatusBadRequest, "range too wide")
		return
	}
	entries, err := s.store.EntryDates(ctx, u.ID, from, to)
	if err != nil {
		s.fail(w, err, "calendar")
		return
	}
	moments := s.store.MomentCounts(ctx, u.ID, from, to)
	heads, _ := s.store.FactHeadlines(ctx, from, to)
	weather := map[string]string{}
	tmax := map[string]*float64{}
	covers := map[string]string{}
	sums := map[string]map[string]store.Fact{}
	if fs, err := s.store.FactsRange(ctx, from, to); err == nil {
		for _, f := range fs {
			if len(f.Payload) <= 4 {
				continue
			}
			if sums[f.Date] == nil {
				sums[f.Date] = map[string]store.Fact{}
			}
			sums[f.Date][f.Source] = f
			switch f.Source {
			case "weather":
				var d sources.Day
				if jsonUnmarshal(f.Payload, &d) == nil {
					weather[f.Date], tmax[f.Date] = d.Condition, d.TMax
				}
			case "immich":
				var p sources.Photos
				if jsonUnmarshal(f.Payload, &p) == nil {
					covers[f.Date] = pickCover(p.Items)
				}
			}
		}
	}
	days := []calendarDay{}
	for d := from; d <= to; d = store.AddDays(d, 1) {
		cd := calendarDay{Date: d, Moments: moments[d], Headlines: heads[d], Weather: weather[d], TMax: tmax[d], Cover: covers[d], Summary: summaryLine(sums[d], 3)}
		if cd.Headlines == nil {
			cd.Headlines = map[string]float64{}
		}
		if e, ok := entries[d]; ok {
			cd.Entry, cd.Mood, cd.Highlight, cd.Pinned = true, e.Mood, e.Highlight, e.Pinned
		}
		days = append(days, cd)
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "days": days})
}

// ---- moments ----

func (s *Server) handleListMoments(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if from == "" {
		from = store.AddDays(store.Today(), -30)
	}
	if to == "" {
		to = store.Today()
	}
	if !validDate(from) || !validDate(to) {
		writeErr(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD")
		return
	}
	ms, err := s.store.ListMoments(r.Context(), u.ID, from, to, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "moments")
		return
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *Server) handleCreateMoment(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var in store.Moment
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		writeErr(w, http.StatusBadRequest, "text is required")
		return
	}
	now := timeNow()
	if in.Date == "" {
		in.Date = now.Format("2006-01-02")
		if in.Time == "" {
			in.Time = now.Format("15:04")
		}
	}
	if !validDate(in.Date) || in.Date > store.Today() {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD and not in the future")
		return
	}
	if in.Time != "" && !validClock(in.Time) {
		writeErr(w, http.StatusBadRequest, "time must be HH:MM")
		return
	}
	in.UserID = u.ID
	if err := s.store.CreateMoment(r.Context(), &in); err != nil {
		s.fail(w, err, "create moment")
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

func (s *Server) handleUpdateMoment(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	m, err := s.store.GetMoment(r.Context(), u.ID, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "get moment")
		return
	}
	var in store.Moment
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if keys["text"] {
		if strings.TrimSpace(in.Text) == "" {
			writeErr(w, http.StatusBadRequest, "text is required")
			return
		}
		m.Text = in.Text
	}
	if keys["time"] {
		if in.Time != "" && !validClock(in.Time) {
			writeErr(w, http.StatusBadRequest, "time must be HH:MM")
			return
		}
		m.Time = in.Time
	}
	if keys["kind"] {
		m.Kind = in.Kind
	}
	if keys["date"] && validDate(in.Date) && in.Date <= store.Today() {
		m.Date = in.Date
	}
	if err := s.store.UpdateMoment(r.Context(), m); err != nil {
		s.fail(w, err, "update moment")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMoment(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteMoment(r.Context(), userFrom(r.Context()).ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "delete moment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validClock(t string) bool {
	if len(t) != 5 || t[2] != ':' {
		return false
	}
	h, m := t[:2], t[3:]
	return h >= "00" && h <= "23" && m >= "00" && m <= "59"
}

// ---- pins ----

func (s *Server) handleCreatePin(w http.ResponseWriter, r *http.Request) {
	date, ok := dateParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	u := userFrom(r.Context())
	var in store.Pin
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	in.Source = strings.TrimSpace(in.Source)
	in.Title = strings.TrimSpace(in.Title)
	if in.Source == "" || (in.Title == "" && in.Ref == "") {
		writeErr(w, http.StatusBadRequest, "source and a title or ref are required")
		return
	}
	in.UserID, in.Date = u.ID, date
	if err := s.store.CreatePin(r.Context(), &in); err != nil {
		s.fail(w, err, "pin")
		return
	}
	pins, _ := s.store.ListPins(r.Context(), u.ID, date, date)
	writeJSON(w, http.StatusCreated, map[string]any{"pin": in, "pins": pins})
}

func (s *Server) handleDeletePin(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePin(r.Context(), userFrom(r.Context()).ID, idParam(r, "id")); err != nil {
		s.fail(w, err, "unpin")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- search ----

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []store.Entry{}, "moments": []store.Moment{}})
		return
	}
	entries, _, err := s.store.ListEntries(r.Context(), store.EntryFilter{UserID: u.ID, Query: q, Limit: 20})
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	all, _ := s.store.ListMoments(r.Context(), u.ID, "0000-01-01", "9999-12-31", 100000)
	moments := []store.Moment{}
	lq := strings.ToLower(q)
	for i := len(all) - 1; i >= 0 && len(moments) < 20; i-- {
		if strings.Contains(strings.ToLower(all[i].Text), lq) {
			moments = append(moments, all[i])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "moments": moments})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sources": store.SourceNames, "moment_kinds": store.MomentKinds, "moods": []string{"", "rough", "meh", "okay", "good", "great"}})
}
