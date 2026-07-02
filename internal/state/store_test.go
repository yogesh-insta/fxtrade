package state_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/state"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := state.NewStore(filepath.Join(dir, "state.json"))

	now := time.Now().UTC()
	snap := state.Snapshot{
		Risk: risk.State{
			DailyPnL:        -10,
			WeeklyPnL:       -10,
			TradesThisMonth: 2,
			MonthKey:        now.Format("2006-01"),
			DayStart:        now,
			WeekStart:       now,
		},
		LastTrade: &state.LastTrade{TradeID: "42", RealizedPL: -1.5},
	}

	if err := store.Save(snap); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Risk.TradesThisMonth != 2 {
		t.Fatalf("expected 2 trades, got %d", loaded.Risk.TradesThisMonth)
	}
	if loaded.LastTrade == nil || loaded.LastTrade.TradeID != "42" {
		t.Fatal("last trade not restored")
	}
}
