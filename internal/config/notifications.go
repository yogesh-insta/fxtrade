package config

import "strings"

type NotificationsConfig struct {
	Enabled               bool   `json:"enabled"`
	Prefix                string `json:"prefix"` // legacy FX alias; prefer fx_prefix
	NSEPrefix             string `json:"nse_prefix"`
	AFLPrefix             string `json:"afl_prefix"`
	FXPrefix              string `json:"fx_prefix"`
	// EmailOnTradeOnly suppresses routine strategy/sentiment digests and other
	// non-trade alerts when true. Omit or set true (default); set false to restore
	// full email digests.
	EmailOnTradeOnly      *bool  `json:"email_on_trade_only,omitempty"`
	OnTradeEntry          bool   `json:"on_trade_entry"`
	OnTradeExit           bool   `json:"on_trade_exit"`
	EntryIncludeRunnersUp int    `json:"entry_include_runners_up"`
	DailySummaryUTC       string `json:"daily_summary_utc"`
	WeeklySummaryUTC      string `json:"weekly_summary_utc"`
}

// TradeOnlyEmail reports whether only trade-related emails should be sent.
func (n NotificationsConfig) TradeOnlyEmail() bool {
	if n.EmailOnTradeOnly == nil {
		return true
	}
	return *n.EmailOnTradeOnly
}

func DefaultNotificationsConfig() NotificationsConfig {
	return NotificationsConfig{
		Enabled:               true,
		Prefix:                "[FXPulse]",
		NSEPrefix:             "[NiftyPulse]",
		AFLPrefix:             "[AFLPulse]",
		FXPrefix:              "[FXPulse]",
		OnTradeEntry:          true,
		OnTradeExit:           true,
		EntryIncludeRunnersUp: 3,
		DailySummaryUTC:       "12:00",
		WeeklySummaryUTC:      "Fri 07:00",
	}
}

// EffectiveNSEPrefix returns the email subject prefix for NSE (NiftyPulse) alerts.
func (n NotificationsConfig) EffectiveNSEPrefix() string {
	if p := trimPrefix(n.NSEPrefix); p != "" {
		return p
	}
	return "[NiftyPulse]"
}

// EffectiveAFLPrefix returns the email subject prefix for AFLPulse alerts.
func (n NotificationsConfig) EffectiveAFLPrefix() string {
	if p := trimPrefix(n.AFLPrefix); p != "" {
		return p
	}
	return "[AFLPulse]"
}

// EffectiveFXPrefix returns the email subject prefix for OANDA (FXPulse) alerts.
func (n NotificationsConfig) EffectiveFXPrefix(dryRun bool) string {
	if p := trimPrefix(n.FXPrefix); p != "" {
		return p
	}
	if p := trimPrefix(n.Prefix); p != "" {
		return p
	}
	if dryRun {
		return "[FXPulse PRACTICE]"
	}
	return "[FXPulse]"
}

func trimPrefix(s string) string {
	return strings.TrimSpace(s)
}

func applyNotificationsDefaults(c *Config) {
	def := DefaultNotificationsConfig()
	if c.Notifications == (NotificationsConfig{}) {
		c.Notifications = def
		return
	}

	// Legacy: fx_prefix inherits prefix when unset.
	if c.Notifications.FXPrefix == "" && c.Notifications.Prefix != "" {
		c.Notifications.FXPrefix = c.Notifications.Prefix
	}
	if c.Notifications.FXPrefix == "" {
		c.Notifications.FXPrefix = def.FXPrefix
	}
	if c.Notifications.NSEPrefix == "" {
		c.Notifications.NSEPrefix = def.NSEPrefix
	}
	if c.Notifications.AFLPrefix == "" {
		c.Notifications.AFLPrefix = def.AFLPrefix
	}
	if c.Notifications.Prefix == "" {
		c.Notifications.Prefix = c.Notifications.FXPrefix
	}
	if c.Notifications.EntryIncludeRunnersUp == 0 {
		c.Notifications.EntryIncludeRunnersUp = def.EntryIncludeRunnersUp
	}
	if c.Notifications.DailySummaryUTC == "" {
		c.Notifications.DailySummaryUTC = def.DailySummaryUTC
	}
	if c.Notifications.WeeklySummaryUTC == "" {
		c.Notifications.WeeklySummaryUTC = def.WeeklySummaryUTC
	}
}
