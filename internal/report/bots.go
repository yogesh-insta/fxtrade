package report

import "github.com/ym/fxtrade/internal/config"

// TradingBotIDs returns the platform bots included in combined performance reports.
func TradingBotIDs() []string {
	return []string{config.BotUniverseScanner, config.BotFxSentiment, config.BotBtcCfd}
}

// BotDisplayName returns a human-readable label for a bot id.
func BotDisplayName(botID string) string {
	switch config.NormalizeBotID(botID) {
	case config.BotUniverseScanner:
		return "Universe Scanner"
	case config.BotFxSentiment:
		return "FX Sentiment"
	case config.BotBtcCfd:
		return "BTC CFD"
	default:
		return botID
	}
}

// BotDBPath resolves the SQLite trades.db path from config.
func BotDBPath(cfg *config.Config, botID string) string {
	switch config.NormalizeBotID(botID) {
	case config.BotBtcCfd:
		return cfg.BtcCfd.DBPath
	case config.BotFxSentiment:
		return cfg.Strategy.DBPath
	case config.BotUniverseScanner:
		return cfg.Scanner.DBPath
	default:
		return config.TradesDBForBot(botID)
	}
}
