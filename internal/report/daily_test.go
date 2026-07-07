package report_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestReportDayUTC(t *testing.T) {
	now := time.Date(2026, 7, 7, 20, 30, 0, 0, time.UTC)
	if got := report.ReportDayUTC(now, true); !got.Equal(time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("same-day = %v", got)
	}
	if got := report.ReportDayUTC(now, false); !got.Equal(time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("previous day = %v", got)
	}
}

func TestDailyEmail(t *testing.T) {
	reportDate := time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC)
	sections := []report.BotDailySection{
		{
			BotID: config.BotUniverseScanner,
			Day: sqlite.DayMetrics{
				TradeCount: 4,
				WinCount:   0,
				LossCount:  2,
				WinRate:    0,
				TotalNetPL: -25.5,
				Trades: []sqlite.TradeBrief{
					{Direction: "LONG", NetPL: -12.75},
					{Direction: "SHORT", NetPL: -12.75},
					{Direction: "LONG", NetPL: 0},
					{Direction: "SHORT", NetPL: 0},
				},
			},
			AllTime: sqlite.DayMetrics{TradeCount: 20, TotalNetPL: -25.5},
		},
		{
			BotID: config.BotFxSentiment,
			Day:   sqlite.DayMetrics{TradeCount: 0},
		},
	}
	analysis := "── Analysis & Suggested Tweaks ──\nUniverse Scanner\n  • test tweak\n"
	accountPNL := "Account NAV: $100076.00 AUD | vs baseline $100250.00 AUD | total P&L -$174.00 (-0.17%)"
	accountTotalPL := -174.0

	subject, body := report.DailyEmail(reportDate, sections, accountPNL, accountTotalPL, analysis)
	if subject != "fxtrade: daily bot summary 2026-07-07" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Account NAV:",
		"Bots tracked (all-time net): -$25.50 | gap vs account: -$148.50",
		"── Universe Scanner ──",
		"Net P&L: -$25.50",
		"2 trades @ $0.00 (unreconciled)",
		"── FX Sentiment ──",
		"No closed trades today.",
		"Combined net P&L: -$25.50",
		"── Analysis & Suggested Tweaks ──",
		"test tweak",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "• LONG  $0.00") {
		t.Fatalf("should collapse zero P/L rows:\n%s", body)
	}
}

func TestFormatDailyAnalysisEmptyDB(t *testing.T) {
	cfg := &config.Config{
		Bots:     config.BotsConfig{Enabled: report.TradingBotIDs()},
		Scanner:  config.ScannerConfig{DBPath: t.TempDir() + "/missing.db"},
		Strategy: config.StrategyConfig{DBPath: t.TempDir() + "/missing2.db"},
		BtcCfd:   config.BtcCfdConfig{DBPath: t.TempDir() + "/missing3.db"},
	}
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	body := report.FormatDailyAnalysis(cfg, now)
	if !strings.Contains(body, "── Analysis & Suggested Tweaks ──") {
		t.Fatalf("missing analysis header:\n%s", body)
	}
	if !strings.Contains(body, "Universe Scanner") {
		t.Fatalf("missing bot section:\n%s", body)
	}
}
