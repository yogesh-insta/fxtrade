package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestPeriodMetricsUTC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)
	inside := start.Add(2 * 24 * time.Hour).Add(10 * time.Hour)
	outside := start.Add(-24 * time.Hour)

	trades := []struct {
		at       time.Time
		net, fee float64
	}{
		{inside, 20, 1},
		{inside.Add(2 * time.Hour), -8, 0.5},
		{inside.Add(4 * time.Hour), 15, 0.5},
		{outside, 99, 0},
	}
	for i, tr := range trades {
		if err := store.InsertTrade(sqlite.Trade{
			ClosedAt:   tr.at,
			Instrument: "EUR_USD",
			TradeID:    "t" + string(rune('a'+i)),
			Direction:  "long",
			RealizedPL: tr.net + tr.fee,
			SwapCost:   tr.fee,
			NetPL:      tr.net,
		}); err != nil {
			t.Fatal(err)
		}
	}

	pm, err := store.PeriodMetricsUTC(start, end)
	if err != nil {
		t.Fatal(err)
	}
	if pm.TradeCount != 3 {
		t.Fatalf("trade_count = %d", pm.TradeCount)
	}
	if pm.WinCount != 2 || pm.LossCount != 1 {
		t.Fatalf("wins=%d losses=%d", pm.WinCount, pm.LossCount)
	}
	if pm.TotalNetPL != 27 {
		t.Fatalf("total_net_pl = %f", pm.TotalNetPL)
	}
	if pm.BestNetPL != 20 || pm.WorstNetPL != -8 {
		t.Fatalf("best=%f worst=%f", pm.BestNetPL, pm.WorstNetPL)
	}
	if pm.MaxDrawdown != 8 {
		t.Fatalf("max_drawdown = %f", pm.MaxDrawdown)
	}
}
