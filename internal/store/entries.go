package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const entryCols = `id, user_id, date, highlight, text, mood, tags, pinned, created_at, updated_at`

func scanEntry(row interface{ Scan(...any) error }) (*Entry, error) {
	var e Entry
	var pinned int
	if err := row.Scan(&e.ID, &e.UserID, &e.Date, &e.Highlight, &e.Text, &e.Mood, &e.Tags, &pinned, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, notFound(err)
	}
	e.Pinned = pinned == 1
	e.Words = wordCount(e.Text) + wordCount(e.Highlight)
	return &e, nil
}

func wordCount(s string) int {
	return len(strings.Fields(s))
}

// GetEntry returns the user's entry for a date (ErrNotFound when none).
func (s *Store) GetEntry(ctx context.Context, userID int64, date string) (*Entry, error) {
	return scanEntry(s.DB.QueryRowContext(ctx, `SELECT `+entryCols+` FROM entries WHERE user_id = ? AND date = ?`, userID, date))
}

// UpsertEntry writes the entry for (user, date). An entry with nothing in it is deleted.
func (s *Store) UpsertEntry(ctx context.Context, e *Entry) error {
	if strings.TrimSpace(e.Highlight) == "" && strings.TrimSpace(e.Text) == "" && e.Mood == 0 && strings.TrimSpace(e.Tags) == "" && !e.Pinned {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM entries WHERE user_id = ? AND date = ?`, e.UserID, e.Date)
		e.ID = 0
		return err
	}
	e.Tags = NormalizeTags(e.Tags)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO entries (user_id, date, highlight, text, mood, tags, pinned, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET highlight = excluded.highlight, text = excluded.text, mood = excluded.mood, tags = excluded.tags, pinned = excluded.pinned, updated_at = excluded.updated_at`,
		e.UserID, e.Date, strings.TrimSpace(e.Highlight), strings.TrimRight(e.Text, " \n\t"), e.Mood, e.Tags, boolInt(e.Pinned), Now())
	if err != nil {
		return err
	}
	got, err := s.GetEntry(ctx, e.UserID, e.Date)
	if err != nil {
		return err
	}
	*e = *got
	return nil
}

// NormalizeTags lower-cases, trims and de-duplicates a comma list.
func NormalizeTags(t string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(t, ",") {
		p = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(p), "#")))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

// EntryFilter narrows a list.
type EntryFilter struct {
	UserID int64
	Query  string
	Tag    string
	Mood   int
	Pinned bool
	From   string
	To     string
	Limit  int
	Offset int
	Asc    bool
}

// ListEntries lists entries newest first.
func (s *Store) ListEntries(ctx context.Context, f EntryFilter) ([]Entry, int, error) {
	where := []string{"e.user_id = ?"}
	args := []any{f.UserID}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, `(e.text LIKE ? OR e.highlight LIKE ? OR e.tags LIKE ? OR EXISTS (SELECT 1 FROM moments m WHERE m.user_id = e.user_id AND m.date = e.date AND m.text LIKE ?))`)
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	if f.Tag != "" {
		where = append(where, `(',' || e.tags || ',') LIKE ?`)
		args = append(args, "%,"+strings.ToLower(f.Tag)+",%")
	}
	if f.Mood > 0 {
		where = append(where, "e.mood = ?")
		args = append(args, f.Mood)
	}
	if f.Pinned {
		where = append(where, "e.pinned = 1")
	}
	if f.From != "" {
		where = append(where, "e.date >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, "e.date <= ?")
		args = append(args, f.To)
	}
	w := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM entries e WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "DESC"
	if f.Asc {
		order = "ASC"
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+prefixed("e", entryCols)+`, (SELECT COUNT(1) FROM moments m WHERE m.user_id = e.user_id AND m.date = e.date)
		FROM entries e WHERE `+w+` ORDER BY e.date `+order+` LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var pinned int
		if err := rows.Scan(&e.ID, &e.UserID, &e.Date, &e.Highlight, &e.Text, &e.Mood, &e.Tags, &pinned, &e.CreatedAt, &e.UpdatedAt, &e.Moments); err != nil {
			return nil, 0, err
		}
		e.Pinned = pinned == 1
		e.Words = wordCount(e.Text) + wordCount(e.Highlight)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func prefixed(p, cols string) string {
	parts := strings.Split(cols, ", ")
	for i := range parts {
		parts[i] = p + "." + parts[i]
	}
	return strings.Join(parts, ", ")
}

// EntryDates returns the dates with an entry in a range (for the calendar).
func (s *Store) EntryDates(ctx context.Context, userID int64, from, to string) (map[string]Entry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+entryCols+` FROM entries WHERE user_id = ? AND date BETWEEN ? AND ?`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out[e.Date] = *e
	}
	return out, rows.Err()
}

// Memories returns the user's entries on the same month-day in other years.
func (s *Store) Memories(ctx context.Context, userID int64, date string) ([]Entry, error) {
	if len(date) != 10 {
		return nil, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+entryCols+` FROM entries WHERE user_id = ? AND substr(date, 6) = ? AND date <> ? ORDER BY date DESC`, userID, date[5:], date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// Neighbours returns the nearest dates with an entry before and after a date.
func (s *Store) Neighbours(ctx context.Context, userID int64, date string) (prev, next string) {
	_ = s.DB.QueryRowContext(ctx, `SELECT date FROM entries WHERE user_id = ? AND date < ? ORDER BY date DESC LIMIT 1`, userID, date).Scan(&prev)
	_ = s.DB.QueryRowContext(ctx, `SELECT date FROM entries WHERE user_id = ? AND date > ? ORDER BY date ASC LIMIT 1`, userID, date).Scan(&next)
	return
}

// Streak counts consecutive days with an entry ending today or yesterday.
func (s *Store) Streak(ctx context.Context, userID int64, today string) (current, longest int) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date FROM entries WHERE user_id = ? ORDER BY date DESC`, userID)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			dates = append(dates, d)
		}
	}
	if len(dates) == 0 {
		return 0, 0
	}
	// current: walk back from today (or yesterday if today isn't written yet)
	start := today
	if dates[0] != today {
		start = AddDays(today, -1)
	}
	set := map[string]bool{}
	for _, d := range dates {
		set[d] = true
	}
	for d := start; set[d]; d = AddDays(d, -1) {
		current++
	}
	run := 0
	var prev string
	for i := len(dates) - 1; i >= 0; i-- { // ascending
		d := dates[i]
		if prev != "" && AddDays(prev, 1) == d {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
		prev = d
	}
	return current, longest
}

// EntryStats are the numbers on the home and dashboard.
type EntryStats struct {
	Total      int     `json:"total"`
	ThisMonth  int     `json:"this_month"`
	ThisYear   int     `json:"this_year"`
	MoodMonth  float64 `json:"mood_month"`
	Words      int     `json:"words"`
	FirstDate  string  `json:"first_date"`
	Pinned     int     `json:"pinned"`
	MomentsAll int     `json:"moments"`
}

// Stats computes EntryStats for a user.
func (s *Store) Stats(ctx context.Context, userID int64, today string) EntryStats {
	var st EntryStats
	month := today[:7]
	year := today[:4]
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(MIN(date), ''), COALESCE(SUM(pinned), 0) FROM entries WHERE user_id = ?`, userID).Scan(&st.Total, &st.FirstDate, &st.Pinned)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1), COALESCE(AVG(CASE WHEN mood > 0 THEN mood END), 0) FROM entries WHERE user_id = ? AND substr(date, 1, 7) = ?`, userID, month).Scan(&st.ThisMonth, &st.MoodMonth)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM entries WHERE user_id = ? AND substr(date, 1, 4) = ?`, userID, year).Scan(&st.ThisYear)
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM moments WHERE user_id = ?`, userID).Scan(&st.MomentsAll)
	rows, err := s.DB.QueryContext(ctx, `SELECT text, highlight FROM entries WHERE user_id = ?`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t, h string
			if rows.Scan(&t, &h) == nil {
				st.Words += wordCount(t) + wordCount(h)
			}
		}
	}
	return st
}

// Tags returns tag → count for a user.
func (s *Store) Tags(ctx context.Context, userID int64) ([]TagCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tags FROM entries WHERE user_id = ? AND tags <> ''`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			for _, p := range strings.Split(t, ",") {
				if p != "" {
					counts[p]++
				}
			}
		}
	}
	out := []TagCount{}
	for k, v := range counts {
		out = append(out, TagCount{Tag: k, Count: v})
	}
	sortTags(out)
	return out, rows.Err()
}

// TagCount is a tag with how often it was used.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

func sortTags(t []TagCount) {
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && (t[j].Count > t[j-1].Count || (t[j].Count == t[j-1].Count && t[j].Tag < t[j-1].Tag)); j-- {
			t[j], t[j-1] = t[j-1], t[j]
		}
	}
}

// ---- moments ----

const momentCols = `id, user_id, date, time, text, kind, created_at`

func scanMoment(row interface{ Scan(...any) error }) (*Moment, error) {
	var m Moment
	if err := row.Scan(&m.ID, &m.UserID, &m.Date, &m.Time, &m.Text, &m.Kind, &m.CreatedAt); err != nil {
		return nil, notFound(err)
	}
	return &m, nil
}

// CreateMoment inserts a moment.
func (s *Store) CreateMoment(ctx context.Context, m *Moment) error {
	if !ValidKind(m.Kind) {
		m.Kind = "note"
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO moments (user_id, date, time, text, kind) VALUES (?, ?, ?, ?, ?)`, m.UserID, m.Date, m.Time, strings.TrimSpace(m.Text), m.Kind)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	got, err := s.GetMoment(ctx, m.UserID, m.ID)
	if err != nil {
		return err
	}
	*m = *got
	return nil
}

// GetMoment returns one moment.
func (s *Store) GetMoment(ctx context.Context, userID, id int64) (*Moment, error) {
	return scanMoment(s.DB.QueryRowContext(ctx, `SELECT `+momentCols+` FROM moments WHERE id = ? AND user_id = ?`, id, userID))
}

// UpdateMoment saves text/time/kind/date.
func (s *Store) UpdateMoment(ctx context.Context, m *Moment) error {
	if !ValidKind(m.Kind) {
		m.Kind = "note"
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE moments SET date = ?, time = ?, text = ?, kind = ? WHERE id = ? AND user_id = ?`, m.Date, m.Time, strings.TrimSpace(m.Text), m.Kind, m.ID, m.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMoment removes a moment.
func (s *Store) DeleteMoment(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM moments WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMoments lists the user's moments between two dates, in time order.
func (s *Store) ListMoments(ctx context.Context, userID int64, from, to string, limit int) ([]Moment, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+momentCols+` FROM moments WHERE user_id = ? AND date BETWEEN ? AND ? ORDER BY date, time, id LIMIT ?`, userID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Moment{}
	for rows.Next() {
		m, err := scanMoment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// MomentCounts returns date → number of moments in a range.
func (s *Store) MomentCounts(ctx context.Context, userID int64, from, to string) map[string]int {
	out := map[string]int{}
	rows, err := s.DB.QueryContext(ctx, `SELECT date, COUNT(1) FROM moments WHERE user_id = ? AND date BETWEEN ? AND ? GROUP BY date`, userID, from, to)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var n int
		if rows.Scan(&d, &n) == nil {
			out[d] = n
		}
	}
	return out
}

// ---- pins ----

const pinCols = `id, user_id, date, source, ref, title, subtitle, image, created_at`

// CreatePin keeps something on a day (idempotent per source+ref).
func (s *Store) CreatePin(ctx context.Context, p *Pin) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO pins (user_id, date, source, ref, title, subtitle, image) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date, source, ref) DO UPDATE SET title = excluded.title, subtitle = excluded.subtitle, image = excluded.image`,
		p.UserID, p.Date, p.Source, p.Ref, p.Title, p.Subtitle, p.Image)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	if p.ID == 0 {
		_ = s.DB.QueryRowContext(ctx, `SELECT id FROM pins WHERE user_id = ? AND date = ? AND source = ? AND ref = ?`, p.UserID, p.Date, p.Source, p.Ref).Scan(&p.ID)
	}
	return nil
}

// DeletePin removes a pin.
func (s *Store) DeletePin(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM pins WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPins lists the user's pins in a date range.
func (s *Store) ListPins(ctx context.Context, userID int64, from, to string) ([]Pin, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+pinCols+` FROM pins WHERE user_id = ? AND date BETWEEN ? AND ? ORDER BY date, id`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pin{}
	for rows.Next() {
		var p Pin
		if err := rows.Scan(&p.ID, &p.UserID, &p.Date, &p.Source, &p.Ref, &p.Title, &p.Subtitle, &p.Image, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AllEntries returns every entry of a user (export).
func (s *Store) AllEntries(ctx context.Context, userID int64) ([]Entry, error) {
	e, _, err := s.ListEntries(ctx, EntryFilter{UserID: userID, Limit: 1000000, Asc: true})
	return e, err
}

// ---- weekday helper ----

// WeekdayISO returns 1 (Monday) .. 7 (Sunday) for a date.
func WeekdayISO(date string) int {
	t, err := ParseDate(date)
	if err != nil {
		return 0
	}
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return wd
}

// nullStr helps scanning optional text.
func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

var _ = nullStr
var _ = fmt.Sprintf
