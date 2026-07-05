package bots

import (
	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/bots/btc_cfd"
	"github.com/ym/fxtrade/internal/bots/fx_sentiment"
	"github.com/ym/fxtrade/internal/bots/universe_scanner"
	"github.com/ym/fxtrade/internal/config"
)

func init() {
	bot.Register(universe_scanner.Meta, universe_scanner.New)
	bot.Register(fx_sentiment.Meta, fx_sentiment.New)
	// Deprecated alias: systemd/CLI may still use range_trend for one release.
	bot.Register(bot.Meta{
		ID:          config.BotRangeTrend,
		Name:        "FX Sentiment Strategy (deprecated id)",
		Description: fx_sentiment.Meta.Description,
	}, fx_sentiment.New)
	bot.Register(btc_cfd.Meta, btc_cfd.New)
}
