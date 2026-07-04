package config

import "strings"

type NotificationsConfig struct {
	Enabled               bool   `json:"enabled"`
	Prefix                string `json:"prefix"` // legacy FX alias; prefer fx_prefix
	NSEPrefix             string `json:"nse_prefix"`
	FXPrefix              string `json:"fx_prefix"`
	OnTradeEntry          bool   `json:"on_trade_entry"`
	OnTradeExit           bool   `json:"on_trade_exit"`
	EntryIncludeRunnersUp int    `json:"entry_include_runners_up"`
	DailySummaryUTC       string `json:"daily_summary_utc"`
	WeeklySummaryUTC      string `json:"weekly_summary_utc"`
}

func DefaultNotificationsConfig() NotificationsConfig {
	return NotificationsConfig{
		Enabled:               true,
		Prefix:                "[FXPulse]",
		NSEPrefix:             "[NiftyPulse]",
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
