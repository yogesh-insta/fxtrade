package bots

import (
	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/bots/btc_cfd"
	"github.com/ym/fxtrade/internal/bots/range_trend"
	"github.com/ym/fxtrade/internal/bots/universe_scanner"
)

func init() {
	bot.Register(universe_scanner.Meta, universe_scanner.New)
	bot.Register(range_trend.Meta, range_trend.New)
	bot.Register(btc_cfd.Meta, btc_cfd.New)
}
