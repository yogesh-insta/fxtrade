package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestDayMetricsUTC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	day := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	other := time.Date(2026, 7, 3, 15, 0, 0, 0, time.UTC)

	trades := []struct {
		at       time.Time
		dir      string
		net, fee float64
	}{
		{day.Add(10 * time.Hour), "long", 12.5, 1.0},
		{day.Add(14 * time.Hour), "short", -5.2, 0.5},
		{day.Add(20 * time.Hour), "long", 8.0, 0.6},
		{other, "long", 99, 0},
	}
	for i, tr := range trades {
		if err := store.InsertTrade(sqlite.Trade{
			ClosedAt:   tr.at,
			Instrument: "BTC_USD",
			TradeID:    "t" + string(rune('a'+i)),
			Direction:  tr.dir,
			RealizedPL: tr.net + tr.fee,
			SwapCost:   tr.fee,
			NetPL:      tr.net,
		}); err != nil {
			t.Fatal(err)
		}
	}

	m, err := store.DayMetricsUTC(day)
	if err != nil {
		t.Fatal(err)
	}
	if m.TradeCount != 3 {
		t.Fatalf("trade_count = %d", m.TradeCount)
	}
	if m.WinCount != 2 || m.LossCount != 1 {
		t.Fatalf("wins=%d losses=%d", m.WinCount, m.LossCount)
	}
	if m.TotalNetPL != 15.3 {
		t.Fatalf("total_net_pl = %f", m.TotalNetPL)
	}
	if m.TotalFees != 2.1 {
		t.Fatalf("total_fees = %f", m.TotalFees)
	}

	all, err := store.AllTimeDayMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if all.TradeCount != 4 {
		t.Fatalf("all-time trade_count = %d", all.TradeCount)
	}
}
