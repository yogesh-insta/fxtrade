package config

type BtcCfdConfig struct {
	// AllocatedCapitalUSD caps position sizing and daily loss when > 0 (per-bot slice of account).
	AllocatedCapitalUSD     float64 `json:"allocated_capital_usd"`
	Instrument              string  `json:"instrument"`
	Granularity             string  `json:"granularity"`
	CandleCount             int     `json:"candle_count"`
	JournalDir              string  `json:"journal_dir"`
	DBPath                  string  `json:"db_path"`
	RSIPeriod               int     `json:"rsi_period"`
	DeviationATR            float64 `json:"deviation_atr_multiple"`
	SLATRMultiple           float64 `json:"sl_atr_multiple"`
	TargetRR                float64 `json:"target_rr"`
	PerTradeRiskPct         float64 `json:"per_trade_risk_pct"`
	MaxDailyLossPct         float64 `json:"max_daily_loss_pct"`
	MaxTradesPerDay         int     `json:"max_trades_per_day"`
	MaxSpreadUSD            float64 `json:"max_spread_usd"`
	MaxSlippagePct          float64 `json:"max_slippage_pct"`
	ATRSpikeMultiple        float64 `json:"atr_spike_multiple"`
	M15Confirmation         bool    `json:"m15_confirmation"`
	MaxHoldHours            float64 `json:"max_hold_hours"`
	APIFailureAlertAfter    int     `json:"api_failure_alert_after"`
}

func DefaultBtcCfdConfig() BtcCfdConfig {
	return BtcCfdConfig{
		Instrument:           "BTC_USD",
		Granularity:          "M5",
		CandleCount:          200,
		JournalDir:           "logs/btc_cfd",
		DBPath:               "data/btc_cfd/trades.db",
		RSIPeriod:            21,
		DeviationATR:         1.5,
		SLATRMultiple:        1.5,
		TargetRR:             1.5,
		PerTradeRiskPct:      0.5,
		MaxDailyLossPct:      2.5,
		MaxTradesPerDay:      5,
		MaxSpreadUSD:         80,
		MaxSlippagePct:       0.1,
		ATRSpikeMultiple:     2.0,
		M15Confirmation:      false,
		MaxHoldHours:         0,
		APIFailureAlertAfter: 3,
	}
}

func applyBtcCfdDefaults(c *Config) {
	def := DefaultBtcCfdConfig()
	if c.BtcCfd.Instrument == "" {
		c.BtcCfd.Instrument = def.Instrument
	}
	if c.BtcCfd.Granularity == "" {
		c.BtcCfd.Granularity = def.Granularity
	}
	if c.BtcCfd.CandleCount == 0 {
		c.BtcCfd.CandleCount = def.CandleCount
	}
	if c.BtcCfd.JournalDir == "" {
		c.BtcCfd.JournalDir = def.JournalDir
	}
	if c.BtcCfd.DBPath == "" {
		c.BtcCfd.DBPath = def.DBPath
	}
	if c.BtcCfd.RSIPeriod == 0 {
		c.BtcCfd.RSIPeriod = def.RSIPeriod
	}
	if c.BtcCfd.DeviationATR == 0 {
		c.BtcCfd.DeviationATR = def.DeviationATR
	}
	if c.BtcCfd.SLATRMultiple == 0 {
		c.BtcCfd.SLATRMultiple = def.SLATRMultiple
	}
	if c.BtcCfd.TargetRR == 0 {
		c.BtcCfd.TargetRR = def.TargetRR
	}
	if c.BtcCfd.TargetRR < 1.2 {
		c.BtcCfd.TargetRR = 1.2
	}
	if c.BtcCfd.PerTradeRiskPct == 0 {
		c.BtcCfd.PerTradeRiskPct = def.PerTradeRiskPct
	}
	if c.BtcCfd.MaxDailyLossPct == 0 {
		c.BtcCfd.MaxDailyLossPct = def.MaxDailyLossPct
	}
	if c.BtcCfd.MaxTradesPerDay == 0 {
		c.BtcCfd.MaxTradesPerDay = reconcileMaxTradesPerDay(c.BtcCfd.MaxDailyLossPct, c.BtcCfd.PerTradeRiskPct)
	}
	if c.BtcCfd.MaxSpreadUSD == 0 {
		c.BtcCfd.MaxSpreadUSD = def.MaxSpreadUSD
	}
	if c.BtcCfd.MaxSlippagePct == 0 {
		c.BtcCfd.MaxSlippagePct = def.MaxSlippagePct
	}
	if c.BtcCfd.ATRSpikeMultiple == 0 {
		c.BtcCfd.ATRSpikeMultiple = def.ATRSpikeMultiple
	}
	if c.BtcCfd.APIFailureAlertAfter == 0 {
		c.BtcCfd.APIFailureAlertAfter = def.APIFailureAlertAfter
	}
}

func reconcileMaxTradesPerDay(maxDailyLossPct, perTradeRiskPct float64) int {
	if perTradeRiskPct <= 0 {
		return DefaultBtcCfdConfig().MaxTradesPerDay
	}
	n := int(maxDailyLossPct / perTradeRiskPct)
	if n < 1 {
		return 1
	}
	return n
}

func (c *Config) BtcCfdEnabled() bool {
	for _, id := range c.EnabledBots() {
		if id == BotBtcCfd {
			return true
		}
	}
	return false
}
