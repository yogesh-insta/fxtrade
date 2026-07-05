package config

const (
	StrategyModeFxSentiment     = "fx_sentiment"
	StrategyModeUniverseScanner = "universe_scanner"

	// Deprecated: use StrategyModeFxSentiment.
	StrategyModeRangeTrend = "range_trend"

	UniverseModeWatchlist = "watchlist"
	UniverseModePreset    = "preset"
	UniverseModeAccount   = "account"

	PresetVolatilePhase1 = "volatile_phase1"
	PresetExpanded       = "expanded"

	RuntimeModeAlwaysOn       = "always_on"
	RuntimeModeSessionDaemon  = "session_daemon"
	RuntimeModeScheduledJob   = "scheduled_job"
)

type ScannerConfig struct {
	Phase                    int                   `json:"phase"`
	UniverseMode             string                `json:"universe_mode"`
	UniversePreset           string                `json:"universe_preset"`
	Watchlist                []string              `json:"watchlist"`
	UniverseFilters          UniverseFiltersConfig `json:"universe_filters"`
	ScanConcurrency          int                   `json:"scan_concurrency"`
	CandleCacheSeconds       int                   `json:"candle_cache_seconds"`
	OpeningRangeCandles      int                   `json:"opening_range_candles"`
	OpeningRangeGranularity  string                `json:"opening_range_granularity"`
	PollSeconds              int                   `json:"poll_seconds"`
	AccountBalanceUSD        float64               `json:"account_balance_usd"`
	BalanceSource            string                `json:"balance_source"`
	RiskPerTradePct          float64               `json:"risk_per_trade_pct"`
	StopLossPipsFX           float64               `json:"stop_loss_pips_fx"`
	StopLossPointsCrypto     float64               `json:"stop_loss_points_crypto"`
	StopLossPointsIndex      float64               `json:"stop_loss_points_index"`
	TakeProfitRR             float64               `json:"take_profit_rr"`
	DailyLossCapPct          float64               `json:"daily_loss_cap_pct"`
	WeeklyProfitTargetPct    float64               `json:"weekly_profit_target_pct"`
	MinSetupScore            float64               `json:"min_setup_score"`
	MinRangeSpreadRatio      float64               `json:"min_range_spread_ratio"`
	RuntimeMode              string                `json:"runtime_mode"`
	JournalDir               string                `json:"journal_dir"`
	DBPath                   string                `json:"db_path"`
	AssetClasses             map[string]AssetClassConfig `json:"asset_classes"`
}

type UniverseFiltersConfig struct {
	Types                  []string  `json:"types"`
	ExcludeExotics         bool      `json:"exclude_exotics"`
	ExcludeQuoteCurrencies []string  `json:"exclude_quote_currencies"`
	MaxSpreadPipsFX        float64   `json:"max_spread_pips_fx"`
	MaxSpreadPointsIndex   float64   `json:"max_spread_points_index"`
	MinATRPercentile       float64   `json:"min_atr_percentile"`
}

type AssetClassConfig struct {
	Instruments       []string `json:"instruments,omitempty"`
	MaxSpreadPips     float64  `json:"max_spread_pips,omitempty"`
	MaxSpreadUSD      float64  `json:"max_spread_usd,omitempty"`
	MaxSpreadPoints   float64  `json:"max_spread_points,omitempty"`
	SessionUTC        string   `json:"session_utc"`
}

func DefaultScannerConfig() ScannerConfig {
	return ScannerConfig{
		Phase:                   1,
		UniverseMode:            UniverseModePreset,
		UniversePreset:          PresetVolatilePhase1,
		ScanConcurrency:         4,
		CandleCacheSeconds:      60,
		OpeningRangeCandles:     2,
		OpeningRangeGranularity: "M15",
		PollSeconds:             10,
		AccountBalanceUSD:       1000,
		BalanceSource:           "oanda_nav",
		RiskPerTradePct:         1.0,
		StopLossPipsFX:          10,
		StopLossPointsCrypto:    200,
		StopLossPointsIndex:     20,
		TakeProfitRR:            2.1,
		DailyLossCapPct:         1.0,
		WeeklyProfitTargetPct:   2.0,
		MinSetupScore:           0.65,
		MinRangeSpreadRatio:     3.0,
		RuntimeMode:             RuntimeModeAlwaysOn,
		JournalDir:              "logs/universe_scanner",
		DBPath:                  "data/universe_scanner/trades.db",
		UniverseFilters: UniverseFiltersConfig{
			Types:                  []string{"CURRENCY", "METAL", "CFD"},
			ExcludeExotics:         true,
			ExcludeQuoteCurrencies: []string{"THB", "HUF", "MXN", "ZAR", "TRY"},
			MaxSpreadPipsFX:        4.0,
			MaxSpreadPointsIndex:   3.0,
		},
		AssetClasses: map[string]AssetClassConfig{
			"FX":     {MaxSpreadPips: 2.5, SessionUTC: "08:00-17:00"},
			"JPY":    {MaxSpreadPips: 3.0, SessionUTC: "00:00-17:00"},
			"METAL":  {MaxSpreadPips: 50, SessionUTC: "13:00-21:00"},
			"CRYPTO": {Instruments: []string{"BTC_USD"}, MaxSpreadUSD: 80, SessionUTC: "24/7"},
			"INDEX":  {MaxSpreadPoints: 2.0, SessionUTC: "13:30-20:00"},
		},
	}
}

func (c *Config) ScannerEnabled() bool {
	return c.Strategy.Mode == StrategyModeUniverseScanner
}

func applyScannerDefaults(c *Config) {
	def := DefaultScannerConfig()
	if c.Scanner.Phase == 0 && c.Scanner.UniverseMode == "" && c.Scanner.UniversePreset == "" &&
		c.Scanner.ScanConcurrency == 0 && len(c.Scanner.Watchlist) == 0 {
		c.Scanner = def
		return
	}
	if c.Scanner.UniverseMode == "" {
		c.Scanner.UniverseMode = def.UniverseMode
	}
	if c.Scanner.UniversePreset == "" {
		c.Scanner.UniversePreset = def.UniversePreset
	}
	if c.Scanner.ScanConcurrency == 0 {
		c.Scanner.ScanConcurrency = def.ScanConcurrency
	}
	if c.Scanner.CandleCacheSeconds == 0 {
		c.Scanner.CandleCacheSeconds = def.CandleCacheSeconds
	}
	if c.Scanner.OpeningRangeCandles == 0 {
		c.Scanner.OpeningRangeCandles = def.OpeningRangeCandles
	}
	if c.Scanner.OpeningRangeGranularity == "" {
		c.Scanner.OpeningRangeGranularity = def.OpeningRangeGranularity
	}
	if c.Scanner.PollSeconds == 0 {
		c.Scanner.PollSeconds = def.PollSeconds
	}
	if c.Scanner.AccountBalanceUSD == 0 {
		c.Scanner.AccountBalanceUSD = def.AccountBalanceUSD
	}
	if c.Scanner.BalanceSource == "" {
		c.Scanner.BalanceSource = def.BalanceSource
	}
	if c.Scanner.RiskPerTradePct == 0 {
		c.Scanner.RiskPerTradePct = def.RiskPerTradePct
	}
	if c.Scanner.StopLossPipsFX == 0 {
		c.Scanner.StopLossPipsFX = def.StopLossPipsFX
	}
	if c.Scanner.StopLossPointsCrypto == 0 {
		c.Scanner.StopLossPointsCrypto = def.StopLossPointsCrypto
	}
	if c.Scanner.StopLossPointsIndex == 0 {
		c.Scanner.StopLossPointsIndex = def.StopLossPointsIndex
	}
	if c.Scanner.TakeProfitRR == 0 {
		c.Scanner.TakeProfitRR = def.TakeProfitRR
	}
	if c.Scanner.DailyLossCapPct == 0 {
		c.Scanner.DailyLossCapPct = def.DailyLossCapPct
	}
	if c.Scanner.WeeklyProfitTargetPct == 0 {
		c.Scanner.WeeklyProfitTargetPct = def.WeeklyProfitTargetPct
	}
	if c.Scanner.MinSetupScore == 0 {
		c.Scanner.MinSetupScore = def.MinSetupScore
	}
	if c.Scanner.MinRangeSpreadRatio == 0 {
		c.Scanner.MinRangeSpreadRatio = def.MinRangeSpreadRatio
	}
	if c.Scanner.RuntimeMode == "" {
		c.Scanner.RuntimeMode = def.RuntimeMode
	}
	if c.Scanner.JournalDir == "" {
		c.Scanner.JournalDir = def.JournalDir
	}
	if c.Scanner.DBPath == "" {
		c.Scanner.DBPath = def.DBPath
	}
	if len(c.Scanner.AssetClasses) == 0 {
		c.Scanner.AssetClasses = def.AssetClasses
	}
}
