package store

import (
	"context"
	"encoding/json"
)

// Story is an AI-written recap of a period.
type Story struct {
	UserID    int64  `json:"user_id"`
	Kind      string `json:"kind"`
	Period    string `json:"period"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
}

// GetStory returns a cached story (ErrNotFound when none).
func (s *Store) GetStory(ctx context.Context, userID int64, kind, period string) (*Story, error) {
	var st Story
	err := s.DB.QueryRowContext(ctx, `SELECT user_id, kind, period, title, text, model, created_at FROM ai_stories WHERE user_id = ? AND kind = ? AND period = ?`, userID, kind, period).
		Scan(&st.UserID, &st.Kind, &st.Period, &st.Title, &st.Text, &st.Model, &st.CreatedAt)
	if err != nil {
		return nil, notFound(err)
	}
	return &st, nil
}

// PutStory stores (or replaces) a story.
func (s *Store) PutStory(ctx context.Context, st *Story) error {
	st.CreatedAt = Now()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO ai_stories (user_id, kind, period, title, text, model, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, kind, period) DO UPDATE SET title = excluded.title, text = excluded.text, model = excluded.model, created_at = excluded.created_at`,
		st.UserID, st.Kind, st.Period, st.Title, st.Text, st.Model, st.CreatedAt)
	return err
}

// ListStories lists a user's stories, newest first.
func (s *Store) ListStories(ctx context.Context, userID int64, limit int) ([]Story, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT user_id, kind, period, title, text, model, created_at FROM ai_stories WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Story{}
	for rows.Next() {
		var st Story
		if err := rows.Scan(&st.UserID, &st.Kind, &st.Period, &st.Title, &st.Text, &st.Model, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeleteStory removes a story.
func (s *Store) DeleteStory(ctx context.Context, userID int64, kind, period string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM ai_stories WHERE user_id = ? AND kind = ? AND period = ?`, userID, kind, period)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Ask is one question to the journal.
type Ask struct {
	ID        int64    `json:"id"`
	UserID    int64    `json:"user_id"`
	Question  string   `json:"question"`
	Answer    string   `json:"answer"`
	Dates     []string `json:"dates"`
	Scope     string   `json:"scope"`
	Model     string   `json:"model"`
	CreatedAt string   `json:"created_at"`
}

// CreateAsk stores a question and its answer.
func (s *Store) CreateAsk(ctx context.Context, a *Ask) error {
	if a.Dates == nil {
		a.Dates = []string{}
	}
	d, _ := json.Marshal(a.Dates)
	res, err := s.DB.ExecContext(ctx, `INSERT INTO ai_asks (user_id, question, answer, dates, scope, model) VALUES (?, ?, ?, ?, ?, ?)`, a.UserID, a.Question, a.Answer, string(d), a.Scope, a.Model)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	a.CreatedAt = Now()
	return nil
}

// ListAsks returns the user's recent questions, newest first.
func (s *Store) ListAsks(ctx context.Context, userID int64, limit int) ([]Ask, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, question, answer, dates, scope, model, created_at FROM ai_asks WHERE user_id = ? ORDER BY id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ask{}
	for rows.Next() {
		var a Ask
		var d string
		if err := rows.Scan(&a.ID, &a.UserID, &a.Question, &a.Answer, &d, &a.Scope, &a.Model, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(d), &a.Dates)
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAsk removes one question (or all of the user's when id is 0).
func (s *Store) DeleteAsk(ctx context.Context, userID, id int64) error {
	if id == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM ai_asks WHERE user_id = ?`, userID)
		return err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM ai_asks WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LogUsage records the tokens one AI call used.
func (s *Store) LogUsage(ctx context.Context, userID int64, feature, model string, prompt, completion int) {
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO ai_usage (user_id, feature, model, prompt_tokens, completion_tokens) VALUES (?, ?, ?, ?, ?)`, userID, feature, model, prompt, completion)
}

// UsageRow is token use per feature in a window.
type UsageRow struct {
	Feature    string `json:"feature"`
	Model      string `json:"model"`
	Calls      int    `json:"calls"`
	Prompt     int    `json:"prompt_tokens"`
	Completion int    `json:"completion_tokens"`
}

// Usage sums a user's token use since an RFC3339 instant.
func (s *Store) Usage(ctx context.Context, userID int64, since string) ([]UsageRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT feature, model, COUNT(1), SUM(prompt_tokens), SUM(completion_tokens) FROM ai_usage WHERE user_id = ? AND created_at >= ? GROUP BY feature, model ORDER BY 2 DESC`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageRow{}
	for rows.Next() {
		var u UsageRow
		if err := rows.Scan(&u.Feature, &u.Model, &u.Calls, &u.Prompt, &u.Completion); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
