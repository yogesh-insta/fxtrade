package bots

import (
	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/bots/range_trend"
	"github.com/ym/fxtrade/internal/bots/universe_scanner"
)

func init() {
	bot.Register(universe_scanner.Meta, universe_scanner.New)
	bot.Register(range_trend.Meta, range_trend.New)
}
