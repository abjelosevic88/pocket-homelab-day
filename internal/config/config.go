// Package config loads runtime configuration from environment variables.
//
// Every option has a sensible default so that `docker run` with a single
// volume works out of the box. See docs/configuration.md for the full list.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
type Config struct {
	Host           string
	Port           int
	BaseURL        string
	DataDir        string
	DBPath         string
	FilesDir       string
	LogLevel       string
	LogFormat      string // text | json
	TrustProxy     bool
	MetricsEnabled bool

	AuthMode             string // local | proxy | none
	AuthProxyHeader      string
	AuthProxyEmailHeader string
	SessionTTL           time.Duration
	SecureCookies        bool
	AdminEmail           string
	AdminPassword        string
	AdminName            string

	// Sign in with an OpenID Connect provider. On a box with Pocket Core the
	// app receives its client when it registers, so none of these are needed;
	// explicit values win when set.
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCName         string // button label
	OIDCAutoCreate   bool   // unknown verified emails get a member account

	DefaultCurrency string
	HomeLat         float64
	HomeLon         float64
	HomePlace       string
	BackfillDays    int

	// Integration defaults; the values saved under Settings → Integrations win.
	NotifyURL    string
	NotifyFormat string // ntfy | apprise | json
	Sources      map[string]SourceEnv

	OpenAIKey     string
	OpenAIModel   string
	OpenAIBaseURL string

	// MoneyWebhookSecret enables POST /api/v1/hooks/money (Pocket Money's signing secret).
	MoneyWebhookSecret string

	// Outbound lookups identify as this.
	UserAgent string

	// Pocket Core: when CORE_URL is set the app registers itself on start,
	// publishes events and accepts the tokens Core hands to paired phones.
	CoreURL             string
	CoreRegistrationKey string
	CoreAppURL          string // how Core reaches this app; defaults to BASE_URL

	DemoData bool

	// Warnings are logged once the logger exists (settings that are ignored).
	Warnings []string
}

// SourceEnv is one integration seeded from the environment.
type SourceEnv struct {
	URL       string
	Key       string
	PublicURL string
	Extra     string
}

// SourceNames lists every data source, in display order.
var SourceNames = []string{"dawarich", "immich", "maloja", "jellystat", "firefly", "habits", "bookshelf", "travel", "kitchen", "garage", "podcasts", "pinepods", "audiobookshelf", "jellyseerr", "forgejo", "nextcloud", "documents", "invoicing", "split", "inventory", "projects", "weather"}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(env(key, "")) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	if v := env(key, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := env(key, ""); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := env(key, ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// Load reads configuration from the environment.
func Load() (*Config, error) {
	dataDir := env("DATA_DIR", "./data")
	c := &Config{
		Host:           env("HOST", "0.0.0.0"),
		Port:           envInt("PORT", 8080),
		BaseURL:        strings.TrimRight(env("BASE_URL", ""), "/"),
		DataDir:        dataDir,
		DBPath:         env("DB_PATH", dataDir+"/pocket-day.db"),
		FilesDir:       env("FILES_DIR", dataDir+"/files"),
		LogLevel:       strings.ToLower(env("LOG_LEVEL", "info")),
		LogFormat:      strings.ToLower(env("LOG_FORMAT", "text")),
		TrustProxy:     envBool("TRUST_PROXY", true),
		MetricsEnabled: envBool("METRICS_ENABLED", true),

		AuthMode:             strings.ToLower(env("AUTH_MODE", "local")),
		AuthProxyHeader:      env("AUTH_PROXY_HEADER", "Remote-User"),
		AuthProxyEmailHeader: env("AUTH_PROXY_EMAIL_HEADER", "Remote-Email"),
		SessionTTL:           envDuration("SESSION_TTL", 720*time.Hour),
		SecureCookies:        envBool("SECURE_COOKIES", false),
		AdminEmail:           strings.ToLower(env("ADMIN_EMAIL", "")),
		AdminPassword:        env("ADMIN_PASSWORD", ""),
		AdminName:            env("ADMIN_NAME", "Admin"),

		OIDCIssuer:       strings.TrimRight(env("OIDC_ISSUER", ""), "/"),
		OIDCClientID:     env("OIDC_CLIENT_ID", ""),
		OIDCClientSecret: env("OIDC_CLIENT_SECRET", ""),
		OIDCName:         env("OIDC_NAME", "single sign-on"),
		OIDCAutoCreate:   envBool("OIDC_AUTO_CREATE", true),

		DefaultCurrency: strings.ToUpper(env("DEFAULT_CURRENCY", "EUR")),
		HomeLat:         envFloat("HOME_LAT", 0),
		HomeLon:         envFloat("HOME_LON", 0),
		HomePlace:       env("HOME_PLACE", ""),
		BackfillDays:    envInt("BACKFILL_DAYS", 90),

		NotifyURL:    env("NOTIFY_URL", ""),
		NotifyFormat: strings.ToLower(env("NOTIFY_FORMAT", "")),
		Sources:      map[string]SourceEnv{},

		OpenAIKey:          env("OPENAI_API_KEY", ""),
		OpenAIModel:        env("OPENAI_MODEL", ""),
		OpenAIBaseURL:      strings.TrimRight(env("OPENAI_BASE_URL", ""), "/"),
		MoneyWebhookSecret: env("MONEY_WEBHOOK_SECRET", ""),

		UserAgent: env("USER_AGENT", "PocketDay/1.0 (+https://github.com/abjelosevic88/pocket-homelab-day)"),

		CoreURL:             strings.TrimRight(env("CORE_URL", ""), "/"),
		CoreRegistrationKey: env("CORE_REGISTRATION_KEY", ""),
		CoreAppURL:          strings.TrimRight(env("CORE_APP_URL", ""), "/"),

		DemoData: envBool("DEMO_DATA", false),
	}
	if c.CoreAppURL == "" {
		c.CoreAppURL = c.BaseURL
	}
	for _, name := range SourceNames {
		up := strings.ToUpper(name)
		se := SourceEnv{
			URL:       strings.TrimRight(env(up+"_URL", ""), "/"),
			Key:       env(up+"_TOKEN", env(up+"_API_KEY", env(up+"_KEY", ""))),
			PublicURL: strings.TrimRight(env(up+"_PUBLIC_URL", ""), "/"),
			Extra:     env(up+"_EXTRA", ""),
		}
		if name == "jellystat" && se.Extra == "" {
			se.Extra = env("JELLYSTAT_USER", "")
		}
		if se.URL != "" || se.Key != "" || se.PublicURL != "" || se.Extra != "" {
			c.Sources[name] = se
		}
	}
	// Paperless-ngx was replaced by Pocket Documents (the "documents" source).
	if env("PAPERLESS_URL", "") != "" {
		c.Warnings = append(c.Warnings, "PAPERLESS_URL is set but ignored: Pocket Day reads documents from Pocket Documents now (Settings → Sources, through Pocket Core or DOCUMENTS_URL + DOCUMENTS_TOKEN); remove the PAPERLESS_* variables")
	}
	switch c.AuthMode {
	case "local", "proxy", "none":
	default:
		return nil, fmt.Errorf("AUTH_MODE must be one of local, proxy, none (got %q)", c.AuthMode)
	}
	if c.OIDCIssuer != "" && (c.OIDCClientID == "" || c.BaseURL == "") {
		return nil, fmt.Errorf("OIDC_ISSUER needs OIDC_CLIENT_ID and BASE_URL (the callback is BASE_URL/api/v1/auth/oidc/callback)")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return nil, fmt.Errorf("PORT out of range: %d", c.Port)
	}
	if c.BackfillDays < 0 {
		c.BackfillDays = 0
	}
	return c, nil
}

// Addr returns host:port.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }
