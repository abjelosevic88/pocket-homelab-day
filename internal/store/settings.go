package store

import (
	"context"
	"strconv"
	"strings"
)

// DefaultTitle is the app title until someone changes it.
const DefaultTitle = "Pocket Day"

// DefaultPrompt is the question above the text box.
const DefaultPrompt = "What made today today?"

// SourceNames lists every data source, in display order.
var SourceNames = []string{"dawarich", "immich", "maloja", "jellystat", "firefly", "habits", "bookshelf", "travel", "kitchen", "garage", "weather"}

func defaultSettings() Settings {
	st := Settings{Title: DefaultTitle, DateFormat: "auto", TimeFormat: "auto", WeekStart: "mon", HomeCurrency: "EUR", DistanceUnit: "km", Prompt: DefaultPrompt, BackfillDays: 90,
		NotifyFormat: "apprise", NotifyEvening: true, AIQuestion: true, AIWeeklyStory: true, AISendText: true, EveningHour: 21, NotifyMorning: true, MorningHour: 9, Sources: map[string]Source{}}
	for _, n := range SourceNames {
		st.Sources[n] = Source{Enabled: true}
	}
	return st
}

// GetSettings loads all settings with defaults applied.
func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	st := defaultSettings()
	rows, err := s.DB.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return st, err
		}
		if strings.HasPrefix(k, "src_") {
			parts := strings.SplitN(strings.TrimPrefix(k, "src_"), "_", 2)
			if len(parts) != 2 {
				continue
			}
			src, ok := st.Sources[parts[0]]
			if !ok {
				continue
			}
			switch parts[1] {
			case "url":
				src.URL = v
			case "key":
				src.Key = v
			case "public":
				src.PublicURL = v
			case "extra":
				src.Extra = v
			case "enabled":
				src.Enabled = v != "0"
			}
			st.Sources[parts[0]] = src
			continue
		}
		switch k {
		case "title":
			if v != "" {
				st.Title = v
			}
		case "date_format":
			if v != "" {
				st.DateFormat = v
			}
		case "time_format":
			if v != "" {
				st.TimeFormat = v
			}
		case "week_start":
			if v == "sun" {
				st.WeekStart = "sun"
			}
		case "home_currency":
			if v != "" {
				st.HomeCurrency = v
			}
		case "distance_unit":
			if v == "mi" {
				st.DistanceUnit = "mi"
			}
		case "home_lat":
			st.HomeLat, _ = strconv.ParseFloat(v, 64)
		case "home_lon":
			st.HomeLon, _ = strconv.ParseFloat(v, 64)
		case "home_place":
			st.HomePlace = v
		case "prompt":
			if v != "" {
				st.Prompt = v
			}
		case "backfill_days":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				st.BackfillDays = n
			}
		case "notify_url":
			st.NotifyURL = v
		case "notify_format":
			if v != "" {
				st.NotifyFormat = v
			}
		case "notify_evening":
			st.NotifyEvening = v != "0"
		case "evening_hour":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
				st.EveningHour = n
			}
		case "notify_morning":
			st.NotifyMorning = v != "0"
		case "morning_hour":
			if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
				st.MorningHour = n
			}
		case "setup_complete":
			st.SetupComplete = v == "1"
		case "ai_key":
			st.AIKey = v
		case "ai_model":
			st.AIModel = v
		case "ai_base_url":
			st.AIBaseURL = v
		case "ai_transcribe_model":
			st.AITranscribeModel = v
		case "ai_language":
			st.AILanguage = v
		case "ai_question":
			st.AIQuestion = v != "0"
		case "ai_weekly_story":
			st.AIWeeklyStory = v != "0"
		case "ai_send_text":
			st.AISendText = v != "0"
		}
	}
	return st, rows.Err()
}

// SaveSettings persists all settings.
func (s *Store) SaveSettings(ctx context.Context, st Settings) error {
	b := map[bool]string{true: "1", false: "0"}
	kv := map[string]string{
		"title":               st.Title,
		"date_format":         st.DateFormat,
		"time_format":         st.TimeFormat,
		"week_start":          st.WeekStart,
		"home_currency":       st.HomeCurrency,
		"distance_unit":       st.DistanceUnit,
		"home_lat":            strconv.FormatFloat(st.HomeLat, 'f', -1, 64),
		"home_lon":            strconv.FormatFloat(st.HomeLon, 'f', -1, 64),
		"home_place":          st.HomePlace,
		"prompt":              st.Prompt,
		"backfill_days":       strconv.Itoa(st.BackfillDays),
		"notify_url":          st.NotifyURL,
		"notify_format":       st.NotifyFormat,
		"notify_evening":      b[st.NotifyEvening],
		"evening_hour":        strconv.Itoa(st.EveningHour),
		"notify_morning":      b[st.NotifyMorning],
		"morning_hour":        strconv.Itoa(st.MorningHour),
		"setup_complete":      b[st.SetupComplete],
		"ai_key":              st.AIKey,
		"ai_model":            st.AIModel,
		"ai_base_url":         st.AIBaseURL,
		"ai_transcribe_model": st.AITranscribeModel,
		"ai_language":         st.AILanguage,
		"ai_question":         b[st.AIQuestion],
		"ai_weekly_story":     b[st.AIWeeklyStory],
		"ai_send_text":        b[st.AISendText],
	}
	for name, src := range st.Sources {
		kv["src_"+name+"_url"] = src.URL
		kv["src_"+name+"_key"] = src.Key
		kv["src_"+name+"_public"] = src.PublicURL
		kv["src_"+name+"_extra"] = src.Extra
		kv["src_"+name+"_enabled"] = b[src.Enabled]
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
