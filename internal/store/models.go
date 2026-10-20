package store

import "encoding/json"

// User is an account that can sign in. The first user is the admin; admins
// manage other users and the box-wide settings (title). Everything else is
// personal: entries, moments, pins, stories, questions, the facts pulled
// from other apps (gathered with that person's own source tokens) and the
// settings (sources, AI key, notifications, home place, journal options).
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Entry is what the user wrote about one day.
type Entry struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Date      string `json:"date"`
	Highlight string `json:"highlight"`
	Text      string `json:"text"`
	Mood      int    `json:"mood"`
	Tags      string `json:"tags"`
	Pinned    bool   `json:"pinned"`
	Origin    string `json:"origin"` // user | habits
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	Moments int `json:"moments,omitempty"` // derived on lists
	Words   int `json:"words,omitempty"`
}

// Moment is one short line at a time of day.
type Moment struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Text      string `json:"text"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// MomentKinds the UI offers.
var MomentKinds = []string{"note", "win", "thanks", "idea", "event"}

// ValidKind reports whether k is a moment kind.
func ValidKind(k string) bool {
	for _, x := range MomentKinds {
		if x == k {
			return true
		}
	}
	return false
}

// Pin is something from a source the user kept on the day.
type Pin struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Date      string `json:"date"`
	Source    string `json:"source"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Image     string `json:"image"`
	CreatedAt string `json:"created_at"`
}

// Fact is one source's knowledge of one day.
type Fact struct {
	Date      string          `json:"date"`
	Source    string          `json:"source"`
	Payload   json.RawMessage `json:"payload"`
	Headline  float64         `json:"headline"`
	Summary   string          `json:"summary"`
	Error     string          `json:"error,omitempty"`
	FetchedAt string          `json:"fetched_at"`
}

// Source is the connection to one other app.
type Source struct {
	URL       string `json:"url"`
	Key       string `json:"key"`
	PublicURL string `json:"public_url"`
	Extra     string `json:"extra"`
	Enabled   bool   `json:"enabled"`
	// Mode is how a Pocket app source is reached: "core" (a Pocket Core app
	// link, per person), "direct" (URL + token) or "off"; "" = not chosen yet.
	Mode string `json:"mode"`
}

// NeedsKey reports whether a source cannot work without a token.
func NeedsKey(name string) bool {
	switch name {
	case "maloja", "weather":
		return false
	}
	return true
}

// Configured reports whether the source can be called.
func (s Source) Configured(name string) bool {
	if !s.Enabled {
		return false
	}
	if name == "weather" {
		return true
	}
	if s.URL == "" {
		return false
	}
	return !NeedsKey(name) || s.Key != ""
}

// Settings are one person's options plus the box-wide title (see BoxSettingKeys).
type Settings struct {
	Title         string  `json:"title"`
	DateFormat    string  `json:"date_format"`
	TimeFormat    string  `json:"time_format"`
	WeekStart     string  `json:"week_start"`
	HomeCurrency  string  `json:"home_currency"`
	DistanceUnit  string  `json:"distance_unit"` // km | mi
	HomeLat       float64 `json:"home_lat"`
	HomeLon       float64 `json:"home_lon"`
	HomePlace     string  `json:"home_place"`
	Prompt        string  `json:"prompt"` // the question above the text box
	BackfillDays  int     `json:"backfill_days"`
	NotifyURL     string  `json:"notify_url"`
	NotifyFormat  string  `json:"notify_format"`
	NotifyEvening bool    `json:"notify_evening"`
	EveningHour   int     `json:"evening_hour"`
	NotifyMorning bool    `json:"notify_morning"`
	MorningHour   int     `json:"morning_hour"`
	SetupComplete bool    `json:"setup_complete"`

	// AI (OpenAI-compatible). Empty key = every AI feature hidden.
	AIKey             string `json:"ai_key"`
	AIModel           string `json:"ai_model"`
	AIBaseURL         string `json:"ai_base_url"`
	AITranscribeModel string `json:"ai_transcribe_model"`
	AILanguage        string `json:"ai_language"`     // "" = same as the entries
	AIQuestion        bool   `json:"ai_question"`     // tonight's question on the day page + evening push
	AIWeeklyStory     bool   `json:"ai_weekly_story"` // Sunday evening: the week as a story, pushed
	AISendText        bool   `json:"ai_send_text"`    // include what you wrote, not only the facts

	Sources map[string]Source `json:"sources"`
}
