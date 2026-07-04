package config

type NotificationsConfig struct {
	Enabled               bool   `json:"enabled"`
	Prefix                string `json:"prefix"`
	OnTradeEntry          bool   `json:"on_trade_entry"`
	OnTradeExit           bool   `json:"on_trade_exit"`
	EntryIncludeRunnersUp int    `json:"entry_include_runners_up"`
	DailySummaryUTC       string `json:"daily_summary_utc"`
	WeeklySummaryUTC      string `json:"weekly_summary_utc"`
}

func DefaultNotificationsConfig() NotificationsConfig {
	return NotificationsConfig{
		Enabled:               true,
		Prefix:                "[fxtrade DEMO]",
		OnTradeEntry:          true,
		OnTradeExit:           true,
		EntryIncludeRunnersUp: 3,
		DailySummaryUTC:       "12:00",
		WeeklySummaryUTC:      "Fri 07:00",
	}
}

func applyNotificationsDefaults(c *Config) {
	def := DefaultNotificationsConfig()
	if c.Notifications == (NotificationsConfig{}) {
		c.Notifications = def
		return
	}
	if c.Notifications.Prefix == "" {
		c.Notifications.Prefix = def.Prefix
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
