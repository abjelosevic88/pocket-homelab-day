package store

import (
	"context"
	"encoding/json"
	"strings"
)

const factCols = `date, source, payload, headline, summary, error, fetched_at`

func scanFact(row interface{ Scan(...any) error }) (*Fact, error) {
	var f Fact
	var payload string
	if err := row.Scan(&f.Date, &f.Source, &payload, &f.Headline, &f.Summary, &f.Error, &f.FetchedAt); err != nil {
		return nil, notFound(err)
	}
	if payload == "" {
		payload = "null"
	}
	f.Payload = json.RawMessage(payload)
	return &f, nil
}

// Facts are personal: each person's were gathered with their own source
// tokens, so every query here is scoped by user id.

// PutFact stores a successful fetch for a user.
func (s *Store) PutFact(ctx context.Context, userID int64, f *Fact) error {
	payload := string(f.Payload)
	if payload == "" {
		payload = "null"
	}
	f.FetchedAt = Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO facts (user_id, date, source, payload, headline, summary, error, fetched_at) VALUES (?, ?, ?, ?, ?, ?, '', ?)
		ON CONFLICT(user_id, date, source) DO UPDATE SET payload = excluded.payload, headline = excluded.headline, summary = excluded.summary, error = '', fetched_at = excluded.fetched_at`,
		userID, f.Date, f.Source, payload, f.Headline, f.Summary, f.FetchedAt)
	return err
}

// PutFactError records a failed fetch, keeping any earlier payload.
func (s *Store) PutFactError(ctx context.Context, userID int64, date, source, msg string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO facts (user_id, date, source, payload, headline, summary, error, fetched_at) VALUES (?, ?, ?, '', 0, '', ?, ?)
		ON CONFLICT(user_id, date, source) DO UPDATE SET error = excluded.error, fetched_at = excluded.fetched_at`, userID, date, source, msg, Now())
	return err
}

// FactsForDay returns source → fact for one date.
func (s *Store) FactsForDay(ctx context.Context, userID int64, date string) (map[string]Fact, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+factCols+` FROM facts WHERE user_id = ? AND date = ?`, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Fact{}
	for rows.Next() {
		f, err := scanFact(rows)
		if err != nil {
			return nil, err
		}
		out[f.Source] = *f
	}
	return out, rows.Err()
}

// FactsRange returns every fact between two dates, oldest first.
func (s *Store) FactsRange(ctx context.Context, userID int64, from, to string, sources ...string) ([]Fact, error) {
	q := `SELECT ` + factCols + ` FROM facts WHERE user_id = ? AND date BETWEEN ? AND ?`
	args := []any{userID, from, to}
	if len(sources) > 0 {
		q += ` AND source IN (?` + strings.Repeat(",?", len(sources)-1) + `)`
		for _, src := range sources {
			args = append(args, src)
		}
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY date, source`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Fact{}
	for rows.Next() {
		f, err := scanFact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, rows.Err()
}

// FactHeadlines returns date → source → headline for a range (cheap, no payloads).
func (s *Store) FactHeadlines(ctx context.Context, userID int64, from, to string) (map[string]map[string]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT date, source, headline FROM facts WHERE user_id = ? AND date BETWEEN ? AND ? AND error = '' AND payload <> ''`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]float64{}
	for rows.Next() {
		var d, src string
		var h float64
		if err := rows.Scan(&d, &src, &h); err != nil {
			return nil, err
		}
		if out[d] == nil {
			out[d] = map[string]float64{}
		}
		out[d][src] = h
	}
	return out, rows.Err()
}

// FactSources lists the sources that have a payload for a date.
func (s *Store) FactSources(ctx context.Context, userID int64, date string) map[string]bool {
	out := map[string]bool{}
	rows, err := s.DB.QueryContext(ctx, `SELECT source FROM facts WHERE user_id = ? AND date = ? AND payload <> '' AND error = ''`, userID, date)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		if rows.Scan(&src) == nil {
			out[src] = true
		}
	}
	return out
}

// DeleteFacts drops a user's cached facts for a date (so a refresh starts clean), or every date when date is "".
func (s *Store) DeleteFacts(ctx context.Context, userID int64, date string) error {
	if date == "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM facts WHERE user_id = ?`, userID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM facts WHERE user_id = ? AND date = ?`, userID, date)
	return err
}

// DeleteFactSource drops one source's cached facts for a date.
func (s *Store) DeleteFactSource(ctx context.Context, userID int64, date, source string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM facts WHERE user_id = ? AND date = ? AND source = ?`, userID, date, source)
	return err
}

// FactDates returns distinct dates with facts, newest first.
func (s *Store) FactDates(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT date FROM facts WHERE user_id = ? AND payload <> '' ORDER BY date DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}
