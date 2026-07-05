package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestInsertTrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	closed := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	if err := store.InsertTrade(sqlite.Trade{
		ClosedAt:      closed,
		Instrument:    "BTC_USD",
		Direction:     "LONG",
		TradeID:       "123",
		CorrelationID: "sig-1",
		RealizedPL:    12.5,
		SwapCost:      0.5,
	}); err != nil {
		t.Fatal(err)
	}
}
