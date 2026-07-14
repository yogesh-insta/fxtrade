package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultGeminiModel mirrors afl.gemini_model in .credentials.example.
const DefaultGeminiModel = "gemini-2.5-flash-lite"

// DefaultRedditUserAgent must be unique per Reddit's API rules; include your Reddit username.
const DefaultRedditUserAgent = "fxtrade:btc-sentiment:1.0 (by /u/YOUR_REDDIT_USERNAME)"

// Config holds all runtime settings from credentials file + environment.
type Config struct {
	GeminiAPIKey       string
	GeminiModel        string
	CryptoPanicAPIKey  string
	RedditClientID     string
	RedditClientSecret string
	RedditUserAgent    string
	RedditSubreddits   []string
	CacheTTL           time.Duration
	Window             time.Duration
	MinItemsThreshold  int
	DBPath             string
	Port               string
	CredentialsPath    string
	RedditConfigured   bool
}

type credentialsFile struct {
	AFL *struct {
		GeminiAPIKey string `json:"gemini_api_key"`
		GeminiModel  string `json:"gemini_model"`
	} `json:"afl"`
	BTCSentiment *btcSentimentFile `json:"btc_sentiment"`
}

type btcSentimentFile struct {
	GeminiAPIKey       string `json:"gemini_api_key"`
	GeminiModel        string `json:"gemini_model"`
	CryptoPanicAPIKey  string `json:"cryptopanic_api_key"`
	RedditClientID     string `json:"reddit_client_id"`
	RedditClientSecret string `json:"reddit_client_secret"`
	RedditUserAgent    string `json:"reddit_user_agent"`
	RedditSubreddits   string `json:"reddit_subreddits"` // comma-separated
}

// Load merges .credentials (optional) then environment overrides.
// Gemini falls back to afl.gemini_* when btc_sentiment.gemini_* is empty.
func Load() (Config, error) {
	cfg := Config{
		GeminiModel:     DefaultGeminiModel,
		RedditUserAgent: DefaultRedditUserAgent,
		DBPath:          envOr("DB_PATH", "/tmp/btc-sentiment.db"),
		Port:            envOr("PORT", "8080"),
		RedditSubreddits: []string{"Bitcoin", "CryptoCurrency"},
	}

	if path := findCredentialsPath(); path != "" {
		cfg.CredentialsPath = path
		if err := applyCredentialsFile(&cfg, path); err != nil {
			return Config{}, err
		}
	}

	// Environment overrides credentials file.
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.GeminiAPIKey = v
	}
	if v := os.Getenv("GEMINI_MODEL"); v != "" {
		cfg.GeminiModel = v
	}
	if v := os.Getenv("CRYPTOPANIC_API_KEY"); v != "" {
		cfg.CryptoPanicAPIKey = v
	}
	if v := os.Getenv("REDDIT_CLIENT_ID"); v != "" {
		cfg.RedditClientID = v
	}
	if v := os.Getenv("REDDIT_CLIENT_SECRET"); v != "" {
		cfg.RedditClientSecret = v
	}
	if v := os.Getenv("REDDIT_USER_AGENT"); v != "" {
		cfg.RedditUserAgent = v
	}
	if v := strings.TrimSpace(os.Getenv("REDDIT_SUBREDDITS")); v != "" {
		cfg.RedditSubreddits = splitCSV(v)
	}

	cacheHours, err := envInt("CACHE_TTL_HOURS", 4)
	if err != nil {
		return Config{}, err
	}
	cfg.CacheTTL = time.Duration(cacheHours) * time.Hour

	windowHours, err := envInt("WINDOW_HOURS", 4)
	if err != nil {
		return Config{}, err
	}
	cfg.Window = time.Duration(windowHours) * time.Hour

	cfg.MinItemsThreshold, err = envInt("MIN_ITEMS_THRESHOLD", 3)
	if err != nil {
		return Config{}, err
	}

	if cfg.GeminiAPIKey == "" {
		return Config{}, fmt.Errorf("GEMINI_API_KEY is required (btc_sentiment.gemini_api_key or afl.gemini_api_key in .credentials)")
	}
	if cfg.GeminiModel == "" {
		cfg.GeminiModel = DefaultGeminiModel
	}
	cfg.RedditConfigured = strings.TrimSpace(cfg.RedditClientID) != "" && strings.TrimSpace(cfg.RedditClientSecret) != ""

	return cfg, nil
}

func applyCredentialsFile(cfg *Config, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read credentials %s: %w", path, err)
	}
	var file credentialsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return fmt.Errorf("parse credentials %s: %w", path, err)
	}
	if file.AFL != nil {
		if cfg.GeminiAPIKey == "" && file.AFL.GeminiAPIKey != "" {
			cfg.GeminiAPIKey = file.AFL.GeminiAPIKey
		}
		if file.AFL.GeminiModel != "" {
			cfg.GeminiModel = file.AFL.GeminiModel
		}
	}
	if b := file.BTCSentiment; b != nil {
		if b.GeminiAPIKey != "" {
			cfg.GeminiAPIKey = b.GeminiAPIKey
		}
		if b.GeminiModel != "" {
			cfg.GeminiModel = b.GeminiModel
		}
		if b.CryptoPanicAPIKey != "" {
			cfg.CryptoPanicAPIKey = b.CryptoPanicAPIKey
		}
		if b.RedditClientID != "" {
			cfg.RedditClientID = b.RedditClientID
		}
		if b.RedditClientSecret != "" {
			cfg.RedditClientSecret = b.RedditClientSecret
		}
		if b.RedditUserAgent != "" {
			cfg.RedditUserAgent = b.RedditUserAgent
		}
		if s := strings.TrimSpace(b.RedditSubreddits); s != "" {
			cfg.RedditSubreddits = splitCSV(s)
		}
	}
	return nil
}

func findCredentialsPath() string {
	if p := os.Getenv("CREDENTIALS_PATH"); p != "" {
		return p
	}
	candidates := []string{
		".credentials",
		filepath.Join("..", ".credentials"),
		filepath.Join("..", "..", ".credentials"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func splitCSV(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid int %q: %w", key, v, err)
	}
	return n, nil
}
