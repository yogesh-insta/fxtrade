package report_test

import (
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestIsTradeReconciled(t *testing.T) {
	if !report.IsTradeReconciled(1.5, 0) {
		t.Fatal("expected reconciled net P/L")
	}
	if !report.IsTradeReconciled(0, -2.0) {
		t.Fatal("expected reconciled realized P/L")
	}
	if report.IsTradeReconciled(0, 0) {
		t.Fatal("expected unreconciled zero P/L")
	}
	if report.IsTradeReconciled(0.005, 0) {
		t.Fatal("expected near-zero net as unreconciled")
	}
}

func TestUpdateTradeReconciliation(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.Open(dir + "/trades.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	closed := time.Date(2026, 7, 7, 15, 0, 0, 0, time.UTC)
	if err := store.InsertTrade(sqlite.Trade{
		ClosedAt:   closed,
		Instrument: "",
		Direction:  "LONG",
		TradeID:    "t-1",
		RealizedPL: 0,
		NetPL:      0,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListTradesNeedingReconciliation()
	if err != nil || len(rows) != 1 {
		t.Fatalf("need reconciliation rows: %d err %v", len(rows), err)
	}

	if err := store.UpdateTradeReconciliation("t-1", "EUR_USD", 12.5, 12.0); err != nil {
		t.Fatal(err)
	}

	rows, err = store.ListTradesNeedingReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no rows needing reconciliation, got %d", len(rows))
	}

	all, err := store.ListClosedTrades()
	if err != nil || len(all) != 1 {
		t.Fatalf("list trades: %v len %d", err, len(all))
	}
	if all[0].Instrument != "EUR_USD" || all[0].NetPL != 12.0 {
		t.Fatalf("unexpected trade %+v", all[0])
	}
}
