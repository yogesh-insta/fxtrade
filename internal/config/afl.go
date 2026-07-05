package config

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// AFLConfig holds AFLPulse value-betting scanner settings.
type AFLConfig struct {
	OddsAPIKey           string  `json:"odds_api_key"`
	MinEVThreshold       float64 `json:"min_ev_threshold"`
	Concurrency          int     `json:"concurrency"`
	MaxOddsAgeMinutes    int     `json:"max_odds_age_minutes"`
	ModelPath            string  `json:"model_path"`
	TotalsModelPath      string  `json:"totals_model_path"`
	StatsDir             string  `json:"stats_dir"`
	InjuriesFile         string  `json:"injuries_file"`
	AlertTopN            int     `json:"alert_top_n"`
	OverallTimeoutMin    int     `json:"overall_timeout_minutes"`
	OddsAPIBaseURL       string  `json:"odds_api_base_url"`
	OddsSportKey         string  `json:"odds_sport_key"`
	OddsRegions          string  `json:"odds_regions"`
	OddsMarkets          string  `json:"odds_markets"`
	PredictorType        string  `json:"predictor_type"` // "matrix" or "onnx"
	ONNXModelPath        string  `json:"onnx_model_path"`
	StatsRefreshOnRun    *bool   `json:"stats_refresh_on_run"`
	SquiggleUserAgent    string  `json:"squiggle_user_agent"`
	StatsSeasonYear      int     `json:"stats_season_year"`
	GeminiAPIKey              string  `json:"gemini_api_key"`
	GeminiModel               string  `json:"gemini_model"`
	LLMAnalyticsEnabled       *bool   `json:"llm_analytics_enabled"`
	LLMConcurrency            int     `json:"llm_concurrency"`
	PregameLeadMinutes        int     `json:"pregame_lead_minutes"`
	PregamePollWindowMinutes  int     `json:"pregame_poll_window_minutes"`
	PregameStatePath          string  `json:"pregame_state_path"`
	PregameLLMRequired        *bool   `json:"pregame_llm_required"`
	PregameLLMRetries         int     `json:"pregame_llm_retries"`
}

func DefaultAFLConfig() AFLConfig {
	return AFLConfig{
		MinEVThreshold:    0.05,
		Concurrency:       4,
		MaxOddsAgeMinutes: 30,
		ModelPath:         "data/afl/model_coefficients.json",
		TotalsModelPath:   "data/afl/totals_coefficients.json",
		StatsDir:          "data/afl",
		InjuriesFile:      "data/afl/injuries.json",
		AlertTopN:         5,
		OverallTimeoutMin: 5,
		OddsAPIBaseURL:    "https://api.the-odds-api.com/v4",
		OddsSportKey:      "aussierules_afl",
		OddsRegions:       "au",
		OddsMarkets:       "h2h,totals",
		PredictorType:     "matrix",
		StatsRefreshOnRun: defaultStatsRefreshOnRun(),
		GeminiModel:              "gemini-2.5-flash-lite",
		LLMConcurrency:           2,
		PregameLeadMinutes:       45,
		PregamePollWindowMinutes: 5,
		PregameLLMRetries:        2,
	}
}

func defaultStatsRefreshOnRun() *bool {
	v := true
	return &v
}

func applyAFLDefaults(a *AFLConfig) {
	def := DefaultAFLConfig()
	if a.MinEVThreshold == 0 {
		a.MinEVThreshold = def.MinEVThreshold
	}
	if a.Concurrency == 0 {
		a.Concurrency = def.Concurrency
	}
	if a.MaxOddsAgeMinutes == 0 {
		a.MaxOddsAgeMinutes = def.MaxOddsAgeMinutes
	}
	if a.ModelPath == "" {
		a.ModelPath = def.ModelPath
	}
	if a.TotalsModelPath == "" {
		a.TotalsModelPath = def.TotalsModelPath
	}
	if a.StatsDir == "" {
		a.StatsDir = def.StatsDir
	}
	if a.InjuriesFile == "" {
		a.InjuriesFile = def.InjuriesFile
	}
	if a.AlertTopN == 0 {
		a.AlertTopN = def.AlertTopN
	}
	if a.OverallTimeoutMin == 0 {
		a.OverallTimeoutMin = def.OverallTimeoutMin
	}
	if a.OddsAPIBaseURL == "" {
		a.OddsAPIBaseURL = def.OddsAPIBaseURL
	}
	if a.OddsSportKey == "" {
		a.OddsSportKey = def.OddsSportKey
	}
	if a.OddsRegions == "" {
		a.OddsRegions = def.OddsRegions
	}
	if a.OddsMarkets == "" {
		a.OddsMarkets = def.OddsMarkets
	}
	if a.PredictorType == "" {
		a.PredictorType = def.PredictorType
	}
	if a.SquiggleUserAgent == "" {
		a.SquiggleUserAgent = "AFLPulse/1.0 fxtrade"
	}
	if a.StatsRefreshOnRun == nil {
		a.StatsRefreshOnRun = def.StatsRefreshOnRun
	}
	if a.GeminiModel == "" {
		a.GeminiModel = def.GeminiModel
	}
	if a.LLMConcurrency == 0 {
		a.LLMConcurrency = def.LLMConcurrency
	}
	if a.PregameLeadMinutes == 0 {
		a.PregameLeadMinutes = def.PregameLeadMinutes
	}
	if a.PregamePollWindowMinutes == 0 {
		a.PregamePollWindowMinutes = def.PregamePollWindowMinutes
	}
	if a.PregameLLMRetries == 0 {
		a.PregameLLMRetries = def.PregameLLMRetries
	}
}

// AnalyticsEnabled reports whether weekly Gemini grounded analytics should run (opt-in).
func (a AFLConfig) AnalyticsEnabled() bool {
	if a.GeminiAPIKey == "" {
		return false
	}
	if a.LLMAnalyticsEnabled == nil {
		return false
	}
	return *a.LLMAnalyticsEnabled
}

// ResolvedPregameLead returns how long before kickoff pregame alerts fire.
func (a AFLConfig) ResolvedPregameLead() time.Duration {
	return time.Duration(a.PregameLeadMinutes) * time.Minute
}

// ResolvedPregamePollWindow returns the poll window width after the lead time.
func (a AFLConfig) ResolvedPregamePollWindow() time.Duration {
	return time.Duration(a.PregamePollWindowMinutes) * time.Minute
}

// ResolvedPregameStatePath returns the dedup JSON path for pregame emails.
func (a AFLConfig) ResolvedPregameStatePath() string {
	if p := strings.TrimSpace(a.PregameStatePath); p != "" {
		return p
	}
	base := strings.TrimSpace(a.StatsDir)
	if base == "" {
		base = "data/afl"
	}
	return filepath.Join(base, "pregame-sent.json")
}

// PregameLLMRequiredEnabled reports whether LLM is mandatory for pregame (always true by default).
func (a AFLConfig) PregameLLMRequiredEnabled() bool {
	if a.PregameLLMRequired == nil {
		return true
	}
	return *a.PregameLLMRequired
}

// ResolvedPregameLLMRetries returns retry count after the first attempt.
func (a AFLConfig) ResolvedPregameLLMRetries() int {
	if a.PregameLLMRetries < 0 {
		return 0
	}
	return a.PregameLLMRetries
}

// ResolvedGeminiModel returns the configured Gemini model or the cheapest search-capable default.
func (a AFLConfig) ResolvedGeminiModel() string {
	if a.GeminiModel == "" {
		return "gemini-2.5-flash-lite"
	}
	return a.GeminiModel
}

// StatsRefreshEnabled reports whether live stats should be fetched before each run.
func (a AFLConfig) StatsRefreshEnabled() bool {
	if a.StatsRefreshOnRun == nil {
		return true
	}
	return *a.StatsRefreshOnRun
}

func (a AFLConfig) Enabled() bool {
	return a.OddsAPIKey != ""
}

func (a AFLConfig) Validate() error {
	if a.OddsAPIKey == "" {
		return fmt.Errorf("afl.odds_api_key is required for AFLPulse")
	}
	if a.MinEVThreshold < 0 {
		return fmt.Errorf("afl.min_ev_threshold must be >= 0")
	}
	if a.Concurrency < 1 {
		return fmt.Errorf("afl.concurrency must be >= 1")
	}
	return nil
}
