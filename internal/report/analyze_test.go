package report_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestAnalyzeBotAllLossesYesterday(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	yesterday := time.Date(2026, 7, 6, 14, 0, 0, 0, time.UTC)
	for i, inst := range []string{"GBP_USD", "EUR_JPY", "XAU_USD"} {
		if err := store.InsertTrade(sqlite.Trade{
			ClosedAt:      yesterday.Add(time.Duration(i) * time.Hour),
			Instrument:    inst,
			Direction:     "LONG",
			TradeID:       fmt.Sprintf("t%d", i+1),
			CorrelationID: "universe_scanner:scan-" + inst + "-1",
			RealizedPL:    -12.5,
			NetPL:         -12.5,
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.InsertSignal(sqlite.Signal{
			At:            yesterday,
			BotID:         "universe_scanner",
			Instrument:    inst,
			Action:        "entry_taken",
			CorrelationID: "universe_scanner:scan-" + inst + "-1",
			SetupScore:    0.35 + float64(i)*0.05,
		}); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{
		Bots: config.BotsConfig{Enabled: []string{"universe_scanner"}},
		Scanner: config.ScannerConfig{
			DBPath:              path,
			OpeningRangeCandles: 1,
			MinSetupScore:       0.4,
			MinRangeSpreadRatio: 1.5,
			TakeProfitRR:        1.5,
		},
	}

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	a := report.AnalyzeBot(cfg, "universe_scanner", now)
	if a.Err != nil {
		t.Fatal(a.Err)
	}
	if a.Yesterday.TradeCount != 3 || a.Yesterday.WinCount != 0 {
		t.Fatalf("yesterday: trades=%d wins=%d", a.Yesterday.TradeCount, a.Yesterday.WinCount)
	}
	if len(a.Suggestions) == 0 {
		t.Fatal("expected suggestions")
	}
	text := report.FormatAnalysis(a)
	if !contains(text, "all losses") {
		t.Fatalf("expected all-losses suggestion in:\n%s", text)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
