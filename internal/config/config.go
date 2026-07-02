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
	OANDA     OANDAConfig     `json:"oanda"`
	Email     EmailConfig     `json:"email"`
	Risk      RiskConfig      `json:"risk"`
	Finnhub   FinnhubConfig   `json:"finnhub"`
	LLM       LLMConfig       `json:"llm"`
	Sentiment SentimentConfig `json:"sentiment"`
	Strategy  StrategyConfig  `json:"strategy"`
	RangeMode RangeModeConfig `json:"range_mode"`
	TrendMode TrendModeConfig `json:"trend_mode"`
	LLMGate   LLMGateConfig   `json:"llm_gate"`
}

type OANDAConfig struct {
	AccountID   string `json:"account_id"`
	Token       string `json:"token"`
	Environment string `json:"environment"`
}

type EmailConfig struct {
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	AlertTo  string `json:"alert_to"`
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

func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Provider: "groq",
		BaseURL:  "https://api.groq.com/openai/v1",
		Model:    "llama-3.3-70b-versatile",
	}
}

func DefaultSentimentConfig() SentimentConfig {
	return SentimentConfig{
		IntervalMinutes:     30,
		MaxHeadlines:        25,
		HeadlineMaxAgeHours: 48,
		AuditDir:            "logs/sentiment",
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
	if c.Sentiment.MaxHeadlines == 0 {
		c.Sentiment.MaxHeadlines = defSent.MaxHeadlines
	}
	if c.Sentiment.HeadlineMaxAgeHours == 0 {
		c.Sentiment.HeadlineMaxAgeHours = defSent.HeadlineMaxAgeHours
	}
	if c.Sentiment.AuditDir == "" {
		c.Sentiment.AuditDir = defSent.AuditDir
	}

	applyStrategyDefaults(c)
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
	return nil
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
