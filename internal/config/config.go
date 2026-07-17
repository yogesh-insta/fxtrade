package config

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	EnvPractice     = "practice"
	EnvLive         = "live"
	DefaultHaltFile = ".halt"
)

type Config struct {
	OANDA         OANDAConfig         `json:"oanda"`
	Instruments   []string            `json:"instruments"`
	Email         EmailConfig         `json:"email"`
	Notifications NotificationsConfig `json:"notifications"`
	Scanner       ScannerConfig       `json:"scanner"`
	BtcCfd        BtcCfdConfig        `json:"btc_cfd"`
	Bots          BotsConfig          `json:"bots"`
	Risk          RiskConfig          `json:"risk"`
	Finnhub       FinnhubConfig       `json:"finnhub"`
	LLM           LLMConfig           `json:"llm"`
	Sentiment     SentimentConfig     `json:"sentiment"`
	Strategy      StrategyConfig      `json:"strategy"`
	RangeMode     RangeModeConfig     `json:"range_mode"`
	TrendMode     TrendModeConfig     `json:"trend_mode"`
	LLMGate       LLMGateConfig       `json:"llm_gate"`
	StockScan     StockScanConfig     `json:"stock_scan"`
	AFL           AFLConfig           `json:"afl"`
	State         StateConfig         `json:"state"`
}

type OANDAConfig struct {
	AccountID          string  `json:"account_id"`
	Token              string  `json:"token"`
	Environment        string  `json:"environment"`
	InitialCapitalAUD  float64 `json:"initial_capital_aud,omitempty"`
	InitialCapitalNote string  `json:"initial_capital_note,omitempty"`
}

type EmailConfig struct {
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           int    `json:"smtp_port"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	AlertTo            string `json:"alert_to"`
	MinIntervalMinutes int    `json:"min_interval_minutes"`
}

func (e EmailConfig) Enabled() bool {
	return e.SMTPHost != "" && e.Username != "" && e.Password != "" && e.AlertTo != ""
}

type FinnhubConfig struct {
	APIKey string `json:"api_key"`
}

type LLMConfig struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

type SentimentConfig struct {
	IntervalMinutes     int    `json:"interval_minutes"`
	MaxHeadlines        int    `json:"max_headlines"`
	HeadlineMaxAgeHours int    `json:"headline_max_age_hours"`
	AuditDir            string `json:"audit_dir"`
}

type RiskConfig struct {
	// AllocatedCapitalUSD caps position sizing and loss limits when > 0 (per-bot slice of account).
	AllocatedCapitalUSD     float64 `json:"allocated_capital_usd"`
	RiskPerTradePctBase     float64 `json:"risk_per_trade_pct_base"`
	RiskPerTradePctHighConf float64 `json:"risk_per_trade_pct_high_conf"`
	HighConfThreshold       float64 `json:"high_conf_threshold"`
	MaxDailyLossPct         float64 `json:"max_daily_loss_pct"`
	MaxWeeklyLossPct        float64 `json:"max_weekly_loss_pct"`
	MaxTradesPerMonth       int     `json:"max_trades_per_month"`
	MaxOpenPositions        int     `json:"max_open_positions"`
	CooldownAfterLossDays   int     `json:"cooldown_after_loss_days"`
	MaxSpreadPips           float64 `json:"max_spread_pips"`
	HaltFile                string  `json:"halt_file"`
}

func DefaultRiskConfig() RiskConfig {
	return RiskConfig{
		RiskPerTradePctBase:     0.5,
		RiskPerTradePctHighConf: 1.0,
		HighConfThreshold:       0.75,
		MaxDailyLossPct:         2.0,
		MaxWeeklyLossPct:        5.0,
		MaxTradesPerMonth:       4,
		MaxOpenPositions:        1,
		CooldownAfterLossDays:   3,
		MaxSpreadPips:           2.5,
		HaltFile:                DefaultHaltFile,
	}
}

type StateConfig struct {
	File string `json:"file"`
}

func DefaultStateConfig() StateConfig {
	return StateConfig{File: "data/state.json"}
}

func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Provider: "groq",
		BaseURL:  "https://api.groq.com/openai/v1",
		Model:    "llama-3.3-70b-versatile",
	}
}

// MinSentimentIntervalMinutes is the floor for Groq/LLM sentiment cycles.
const MinSentimentIntervalMinutes = 30

func DefaultSentimentConfig() SentimentConfig {
	return SentimentConfig{
		IntervalMinutes:     30,
		MaxHeadlines:        12,
		HeadlineMaxAgeHours: 48,
		AuditDir:            "logs/sentiment",
	}
}

type StockScanConfig struct {
	Concurrency         int      `json:"concurrency"`
	RequestTimeoutSec   int      `json:"request_timeout_seconds"`
	RateLimitMS         int      `json:"rate_limit_ms"`
	OverallTimeoutMin   int      `json:"overall_timeout_minutes"`
	SMAPeriod           int      `json:"sma_period"`
	RSIPeriod           int      `json:"rsi_period"`
	RSIMin              float64  `json:"rsi_min"`
	RSIMax              float64  `json:"rsi_max"`
	StopLossPct         float64  `json:"stop_loss_pct"`
	TargetPct           float64  `json:"target_pct"`
	SentimentCandidates int      `json:"sentiment_candidates"`
	RSSFeeds            []string `json:"rss_feeds"`
}

func DefaultStockScanConfig() StockScanConfig {
	return StockScanConfig{
		Concurrency:         4,
		RequestTimeoutSec:   15,
		RateLimitMS:         300,
		OverallTimeoutMin:   15, // Nifty 200 + Nifty 500-rest (~500 Yahoo chart calls)
		SMAPeriod:           50,
		RSIPeriod:           14,
		RSIMin:              30,
		RSIMax:              45,
		StopLossPct:         0.02,
		TargetPct:           0.03,
		SentimentCandidates: 3,
	}
}

func (c *Config) SentimentEnabled() bool {
	return c.Finnhub.APIKey != "" && c.LLM.APIKey != ""
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.OANDA.Environment == "" {
		c.OANDA.Environment = EnvPractice
	}
	if len(c.Instruments) == 0 && !c.ScannerEnabled() {
		c.Instruments = []string{"AUD_USD"}
	}
	if c.Email.SMTPPort == 0 && c.Email.SMTPHost != "" {
		c.Email.SMTPPort = 587
	}

	defRisk := DefaultRiskConfig()
	if c.Risk == (RiskConfig{}) {
		c.Risk = defRisk
	} else {
		if c.Risk.RiskPerTradePctBase == 0 {
			c.Risk.RiskPerTradePctBase = defRisk.RiskPerTradePctBase
		}
		if c.Risk.RiskPerTradePctHighConf == 0 {
			c.Risk.RiskPerTradePctHighConf = defRisk.RiskPerTradePctHighConf
		}
		if c.Risk.HighConfThreshold == 0 {
			c.Risk.HighConfThreshold = defRisk.HighConfThreshold
		}
		if c.Risk.MaxDailyLossPct == 0 {
			c.Risk.MaxDailyLossPct = defRisk.MaxDailyLossPct
		}
		if c.Risk.MaxWeeklyLossPct == 0 {
			c.Risk.MaxWeeklyLossPct = defRisk.MaxWeeklyLossPct
		}
		if c.Risk.MaxTradesPerMonth == 0 {
			c.Risk.MaxTradesPerMonth = defRisk.MaxTradesPerMonth
		}
		if c.Risk.MaxOpenPositions == 0 {
			c.Risk.MaxOpenPositions = defRisk.MaxOpenPositions
		}
		if c.Risk.CooldownAfterLossDays == 0 {
			c.Risk.CooldownAfterLossDays = defRisk.CooldownAfterLossDays
		}
		if c.Risk.MaxSpreadPips == 0 {
			c.Risk.MaxSpreadPips = defRisk.MaxSpreadPips
		}
		if c.Risk.HaltFile == "" {
			c.Risk.HaltFile = defRisk.HaltFile
		}
	}

	defLLM := DefaultLLMConfig()
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = defLLM.BaseURL
	}
	if c.LLM.Model == "" {
		c.LLM.Model = defLLM.Model
	}
	if c.LLM.Provider == "" {
		c.LLM.Provider = defLLM.Provider
	}

	defSent := DefaultSentimentConfig()
	if c.Sentiment.IntervalMinutes == 0 {
		c.Sentiment.IntervalMinutes = defSent.IntervalMinutes
	}
	if c.Sentiment.IntervalMinutes < MinSentimentIntervalMinutes {
		c.Sentiment.IntervalMinutes = MinSentimentIntervalMinutes
	}
	if c.Sentiment.MaxHeadlines == 0 {
		c.Sentiment.MaxHeadlines = defSent.MaxHeadlines
	}
	if c.Sentiment.HeadlineMaxAgeHours == 0 {
		c.Sentiment.HeadlineMaxAgeHours = defSent.HeadlineMaxAgeHours
	}
	if c.Sentiment.AuditDir == "" {
		c.Sentiment.AuditDir = defSent.AuditDir
	}

	defStockScan := DefaultStockScanConfig()
	applyStockScanDefaults(&c.StockScan, defStockScan)
	applyAFLDefaults(&c.AFL)

	applyStrategyDefaults(c)
	applyScannerDefaults(c)
	applyBtcCfdDefaults(c)
	applyNotificationsDefaults(c)
	applyStateDefaults(c)

	// Legacy single-mode shortcut: when only universe_scanner is active via strategy.mode.
	if c.ScannerEnabled() && len(c.Bots.Enabled) == 0 {
		c.Strategy.Enabled = true
		if len(c.Scanner.Watchlist) > 0 {
			c.Instruments = append([]string(nil), c.Scanner.Watchlist...)
		}
	}
}

func applyStateDefaults(c *Config) {
	if c.State.File == "" {
		c.State.File = "data/state.json"
	}
}

func applyStockScanDefaults(s *StockScanConfig, def StockScanConfig) {
	if s.Concurrency == 0 {
		s.Concurrency = def.Concurrency
	}
	if s.RequestTimeoutSec == 0 {
		s.RequestTimeoutSec = def.RequestTimeoutSec
	}
	if s.RateLimitMS == 0 {
		s.RateLimitMS = def.RateLimitMS
	}
	if s.OverallTimeoutMin == 0 {
		s.OverallTimeoutMin = def.OverallTimeoutMin
	}
	if s.SMAPeriod == 0 {
		s.SMAPeriod = def.SMAPeriod
	}
	if s.RSIPeriod == 0 {
		s.RSIPeriod = def.RSIPeriod
	}
	if s.RSIMin == 0 {
		s.RSIMin = def.RSIMin
	}
	if s.RSIMax == 0 {
		s.RSIMax = def.RSIMax
	}
	if s.StopLossPct == 0 {
		s.StopLossPct = def.StopLossPct
	}
	if s.TargetPct == 0 {
		s.TargetPct = def.TargetPct
	}
	if s.SentimentCandidates == 0 {
		s.SentimentCandidates = def.SentimentCandidates
	}
}

func (c *Config) Validate() error {
	if c.OANDA.AccountID == "" {
		return fmt.Errorf("oanda.account_id is required")
	}
	if c.OANDA.Token == "" {
		return fmt.Errorf("oanda.token is required")
	}
	if c.OANDA.Environment != EnvPractice && c.OANDA.Environment != EnvLive {
		return fmt.Errorf("oanda.environment must be %q or %q", EnvPractice, EnvLive)
	}
	if !c.ScannerEnabled() {
		for _, inst := range c.Instruments {
			if inst == "" {
				return fmt.Errorf("instruments must not contain empty strings")
			}
		}
	}
	return c.ValidateBots()
}

func (o OANDAConfig) RESTBaseURL() string {
	if o.Environment == EnvLive {
		return "https://api-fxtrade.oanda.com"
	}
	return "https://api-fxpractice.oanda.com"
}

func (o OANDAConfig) StreamBaseURL() string {
	if o.Environment == EnvLive {
		return "https://stream-fxtrade.oanda.com"
	}
	return "https://stream-fxpractice.oanda.com"
}
