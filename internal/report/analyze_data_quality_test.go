package report_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestDataQualitySuppressesBadTweaks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	closed := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 19; i++ {
		if err := store.InsertTrade(sqlite.Trade{
			ClosedAt:      closed.Add(time.Duration(i) * time.Minute),
			Instrument:    "",
			Direction:     "LONG",
			TradeID:       fmt.Sprintf("bad%d", i),
			CorrelationID: "universe_scanner:scan-X-1",
			RealizedPL:    0,
			NetPL:         0,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.InsertTrade(sqlite.Trade{
		ClosedAt:      closed,
		Instrument:    "USD_JPY",
		Direction:     "SHORT",
		TradeID:       "good1",
		CorrelationID: "universe_scanner:scan-USD_JPY-1",
		RealizedPL:    -5.25,
		NetPL:         -5.25,
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Bots: config.BotsConfig{Enabled: []string{"universe_scanner"}},
		Scanner: config.ScannerConfig{
			DBPath:              path,
			OpeningRangeCandles: 2,
			MinSetupScore:       0.4,
			MinRangeSpreadRatio: 2.0,
			TakeProfitRR:        2.0,
		},
	}

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	a := report.AnalyzeBot(cfg, "universe_scanner", now)
	if a.Err != nil {
		t.Fatal(a.Err)
	}
	if a.DataQuality.UnreconciledCount != 19 {
		t.Fatalf("unreconciled=%d want 19", a.DataQuality.UnreconciledCount)
	}
	if a.DataQuality.MissingInstrumentCount != 19 {
		t.Fatalf("missing instrument=%d want 19", a.DataQuality.MissingInstrumentCount)
	}

	text := report.FormatAnalysis(a)
	if !strings.Contains(text, "n/a (19 unreconciled)") {
		t.Fatalf("expected unreconciled win rate label in:\n%s", text)
	}
	if strings.Contains(text, "remove from watchlist") || strings.Contains(text, "USD_JPY: 20 trades, 0 wins") {
		t.Fatalf("should not suggest instrument removal on unreconciled data:\n%s", text)
	}
	for _, s := range a.Suggestions {
		if strings.Contains(s, "Win rate") && strings.Contains(s, "below 40%") {
			t.Fatalf("should not suggest win rate tweak on bad data: %q", s)
		}
		if strings.Contains(s, "min_range_spread_ratio") && strings.Contains(s, "3.0") {
			t.Fatalf("should not push 3.0+ ratio when already at 2.0 on bad data: %q", s)
		}
	}
}

func TestListTradesNeedingReconciliation(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "trades.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	closed := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	if err := store.InsertTrade(sqlite.Trade{
		ClosedAt: closed, Instrument: "EUR_USD", TradeID: "ok", RealizedPL: 10, NetPL: 10,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertTrade(sqlite.Trade{
		ClosedAt: closed, Instrument: "", TradeID: "zero", RealizedPL: 0, NetPL: 0,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListTradesNeedingReconciliation()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].TradeID != "zero" {
		t.Fatalf("got %+v", rows)
	}
}
