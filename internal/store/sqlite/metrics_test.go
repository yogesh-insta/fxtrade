package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestMetricsReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	closed := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	for _, pl := range []float64{10, -5, 8, -4} {
		if err := store.InsertTrade(sqlite.Trade{
			ClosedAt:   closed,
			Instrument: "BTC_USD",
			TradeID:    "t",
			RealizedPL: pl,
			NetPL:      pl,
		}); err != nil {
			t.Fatal(err)
		}
		closed = closed.Add(time.Hour)
	}

	m, err := store.MetricsReport(100000, 2.5)
	if err != nil {
		t.Fatal(err)
	}
	if m.TradeCount != 4 {
		t.Fatalf("trade_count = %d", m.TradeCount)
	}
	if m.WinCount != 2 || m.LossCount != 2 {
		t.Fatalf("wins=%d losses=%d", m.WinCount, m.LossCount)
	}
	if m.WinRate != 0.5 {
		t.Fatalf("win_rate = %f", m.WinRate)
	}
}
