package config

type StrategyConfig struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode"`
	CycleMinutes int    `json:"cycle_minutes"`
	PollSeconds  int    `json:"poll_seconds"`
	JournalDir   string `json:"journal_dir"`
	DBPath       string `json:"db_path"`
}

type RangeModeConfig struct {
	RangeLookbackWeeks       int     `json:"range_lookback_weeks"`
	MinRangeWidthPips        float64 `json:"min_range_width_pips"`
	MinBoundaryTouches       int     `json:"min_boundary_touches"`
	ZoneATRMultiplier        float64 `json:"zone_atr_multiplier"`
	StopATRBeyondBoundary    float64 `json:"stop_atr_beyond_boundary"`
	TP1Fraction              float64 `json:"tp1_fraction"`
	PendingLimitsEnabled     bool    `json:"pending_limits_enabled"`
	PendingLimitTimeInForce  string  `json:"pending_limit_time_in_force"`
	CancelOppositeOnFill     bool    `json:"cancel_opposite_limit_on_fill"`
	CancelLimitsOnModeChange bool    `json:"cancel_limits_on_mode_change"`
	FlatEMAThresholdPct      float64 `json:"flat_ema_threshold_pct"`
}

type TrendModeConfig struct {
	RegimeEMAFastW           int     `json:"regime_ema_fast_w"`
	RegimeEMASlowW           int     `json:"regime_ema_slow_w"`
	EntryEMAH4               int     `json:"entry_ema_h4"`
	RSIPeriodH4              int     `json:"rsi_period_h4"`
	RSILongMin               float64 `json:"rsi_long_min"`
	RSILongMax               float64 `json:"rsi_long_max"`
	RSIShortMin              float64 `json:"rsi_short_min"`
	RSIShortMax              float64 `json:"rsi_short_max"`
	ATRStopMultiplier        float64 `json:"atr_stop_multiplier"`
	Week52BlockDistancePips  float64 `json:"week52_block_distance_pips"`
	Week52HalfSizeDistancePips float64 `json:"week52_halfsize_distance_pips"`
}

type LLMGateConfig struct {
	VetoConfidence                   float64 `json:"veto_confidence"`
	FullSizeConfidence               float64 `json:"full_size_confidence"`
	SentimentPersistenceReadings     int     `json:"sentiment_persistence_readings"`
	SentimentPersistenceIntervalMin  int     `json:"sentiment_persistence_interval_minutes"`
}

func DefaultStrategyConfig() StrategyConfig {
	return StrategyConfig{
		Enabled:      false,
		CycleMinutes: 30,
		JournalDir:   "logs/fx_sentiment",
		DBPath:       "data/fx_sentiment/trades.db",
	}
}

func DefaultRangeModeConfig() RangeModeConfig {
	return RangeModeConfig{
		RangeLookbackWeeks:       13,
		MinRangeWidthPips:        300,
		MinBoundaryTouches:       2,
		ZoneATRMultiplier:        0.5,
		StopATRBeyondBoundary:    1.0,
		TP1Fraction:              0.5,
		PendingLimitsEnabled:     true,
		PendingLimitTimeInForce:  "GTC",
		CancelOppositeOnFill:     true,
		CancelLimitsOnModeChange: true,
		FlatEMAThresholdPct:      0.8,
	}
}

func DefaultTrendModeConfig() TrendModeConfig {
	return TrendModeConfig{
		RegimeEMAFastW:             20,
		RegimeEMASlowW:             50,
		EntryEMAH4:                 20,
		RSIPeriodH4:                14,
		RSILongMin:                 40,
		RSILongMax:                 65,
		RSIShortMin:                35,
		RSIShortMax:                60,
		ATRStopMultiplier:          2.5,
		Week52BlockDistancePips:    100,
		Week52HalfSizeDistancePips: 250,
	}
}

func DefaultLLMGateConfig() LLMGateConfig {
	return LLMGateConfig{
		VetoConfidence:                  0.65,
		FullSizeConfidence:              0.75,
		SentimentPersistenceReadings:    2,
		SentimentPersistenceIntervalMin: 60,
	}
}

func applyStrategyDefaults(c *Config) {
	defStrat := DefaultStrategyConfig()
	if c.Strategy.CycleMinutes == 0 {
		c.Strategy.CycleMinutes = defStrat.CycleMinutes
	}
	if c.Strategy.JournalDir == "" {
		c.Strategy.JournalDir = defStrat.JournalDir
	}
	if c.Strategy.DBPath == "" {
		c.Strategy.DBPath = defStrat.DBPath
	}

	defRange := DefaultRangeModeConfig()
	if c.RangeMode.RangeLookbackWeeks == 0 {
		c.RangeMode.RangeLookbackWeeks = defRange.RangeLookbackWeeks
	}
	if c.RangeMode.MinRangeWidthPips == 0 {
		c.RangeMode.MinRangeWidthPips = defRange.MinRangeWidthPips
	}
	if c.RangeMode.MinBoundaryTouches == 0 {
		c.RangeMode.MinBoundaryTouches = defRange.MinBoundaryTouches
	}
	if c.RangeMode.ZoneATRMultiplier == 0 {
		c.RangeMode.ZoneATRMultiplier = defRange.ZoneATRMultiplier
	}
	if c.RangeMode.StopATRBeyondBoundary == 0 {
		c.RangeMode.StopATRBeyondBoundary = defRange.StopATRBeyondBoundary
	}
	if c.RangeMode.TP1Fraction == 0 {
		c.RangeMode.TP1Fraction = defRange.TP1Fraction
	}
	if c.RangeMode.PendingLimitTimeInForce == "" {
		c.RangeMode.PendingLimitTimeInForce = defRange.PendingLimitTimeInForce
	}
	if c.RangeMode.FlatEMAThresholdPct == 0 {
		c.RangeMode.FlatEMAThresholdPct = defRange.FlatEMAThresholdPct
	}
	// bool defaults: only set PendingLimitsEnabled if zero value - use pointer or accept false as valid
	// For first load with empty struct, enable pending limits
	if c.RangeMode == (RangeModeConfig{}) {
		c.RangeMode = defRange
	}

	defTrend := DefaultTrendModeConfig()
	if c.TrendMode == (TrendModeConfig{}) {
		c.TrendMode = defTrend
	} else {
		if c.TrendMode.RegimeEMAFastW == 0 {
			c.TrendMode.RegimeEMAFastW = defTrend.RegimeEMAFastW
		}
		if c.TrendMode.RegimeEMASlowW == 0 {
			c.TrendMode.RegimeEMASlowW = defTrend.RegimeEMASlowW
		}
		if c.TrendMode.ATRStopMultiplier == 0 {
			c.TrendMode.ATRStopMultiplier = defTrend.ATRStopMultiplier
		}
		if c.TrendMode.Week52BlockDistancePips == 0 {
			c.TrendMode.Week52BlockDistancePips = defTrend.Week52BlockDistancePips
		}
	}

	defLLM := DefaultLLMGateConfig()
	if c.LLMGate.VetoConfidence == 0 {
		c.LLMGate.VetoConfidence = defLLM.VetoConfidence
	}
	if c.LLMGate.FullSizeConfidence == 0 {
		c.LLMGate.FullSizeConfidence = defLLM.FullSizeConfidence
	}
	if c.LLMGate.SentimentPersistenceReadings == 0 {
		c.LLMGate.SentimentPersistenceReadings = defLLM.SentimentPersistenceReadings
	}
	if c.LLMGate.SentimentPersistenceIntervalMin == 0 {
		c.LLMGate.SentimentPersistenceIntervalMin = defLLM.SentimentPersistenceIntervalMin
	}
}
