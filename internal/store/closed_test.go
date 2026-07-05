package store_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/store"
)

func TestClosedTradeRowUsesMetaInstrument(t *testing.T) {
	meta := store.TradeMeta{
		Instrument: "GBP_USD",
		Direction:  "LONG",
		FillPrice:  1.25,
		Units:      1000,
	}
	row := store.ClosedTradeRow("", "t1", "corr", 10, meta, true)
	if row.Instrument != "GBP_USD" {
		t.Fatalf("instrument = %q", row.Instrument)
	}
	if row.Direction != "LONG" {
		t.Fatalf("direction = %q", row.Direction)
	}
}

func TestTradesDBForBot(t *testing.T) {
	if got := config.TradesDBForBot(config.BotFxSentiment); got != "data/fx_sentiment/trades.db" {
		t.Fatalf("got %q", got)
	}
	if got := config.JournalDirForBot(config.BotUniverseScanner); got != "logs/universe_scanner" {
		t.Fatalf("got %q", got)
	}
}
