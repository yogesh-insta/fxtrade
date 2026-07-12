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
			BotID:   config.BotFxSentiment,
			Day:     sqlite.DayMetrics{TradeCount: 0},
			AllTime: sqlite.DayMetrics{TradeCount: 14, TotalNetPL: -100},
		},
	}
	analysis := "── Suggested tweaks ──\nUniverse Scanner\n  • test tweak\n"
	acct := report.AccountDailyContext{
		NAV:          100076,
		Currency:     "AUD",
		Baseline:     100250,
		TotalPL:      -174,
		DailyPLKnown: true,
		PriorNAV:     100100,
		PriorDate:    "2026-07-06",
		DailyPL:      -24,
	}

	subject, body := report.DailyEmail(reportDate, sections, acct, analysis)
	if subject != "fxtrade: daily summary 2026-07-07 | account LOSS" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"═══ P&L SUMMARY ═══",
		"Account (OANDA)",
		"LOSS -$24.00",
		"LOSS -$174.00",
		"Universe Scanner",
		"LOSS -$25.50",
		"FX Sentiment",
		"FLAT $0.00",
		"LOSS -$100.00",
		"── Today's trades ──",
		"LOSS -$12.75  LONG",
		"2 unreconciled trades",
		"Bots combined today:  LOSS -$25.50",
		"── Suggested tweaks ──",
		"test tweak",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "• LONG  $0.00") {
		t.Fatalf("should collapse zero P/L rows:\n%s", body)
	}
	if strings.Contains(body, "By exit reason") {
		t.Fatalf("daily email should not include analysis buckets:\n%s", body)
	}
}

func TestDailyEmailFlatDay(t *testing.T) {
	reportDate := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	sections := []report.BotDailySection{
		{
			BotID:   config.BotUniverseScanner,
			AllTime: sqlite.DayMetrics{TradeCount: 39, TotalNetPL: -887.85},
		},
		{
			BotID:   config.BotFxSentiment,
			AllTime: sqlite.DayMetrics{TradeCount: 14, TotalNetPL: -773.66},
		},
	}
	acct := report.AccountDailyContext{
		NAV:          99273.04,
		Currency:     "AUD",
		Baseline:     100250,
		TotalPL:      -976.96,
		DailyPLKnown: true,
		PriorNAV:     99273.04,
		PriorDate:    "2026-07-11",
		DailyPL:      0,
	}

	_, body := report.DailyEmail(reportDate, sections, acct, "")
	if !strings.Contains(body, "FLAT $0.00") {
		t.Fatalf("expected FLAT today:\n%s", body)
	}
	if !strings.Contains(body, "LOSS -$976.96") {
		t.Fatalf("expected LOSS all-time account:\n%s", body)
	}
	if !strings.Contains(body, "No closed trades across any bot today") {
		t.Fatalf("expected no-trades message:\n%s", body)
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

func TestFormatDailyTweaksEmptyWhenNoSuggestions(t *testing.T) {
	cfg := &config.Config{
		Bots:     config.BotsConfig{Enabled: report.TradingBotIDs()},
		Scanner:  config.ScannerConfig{DBPath: t.TempDir() + "/missing.db"},
		Strategy: config.StrategyConfig{DBPath: t.TempDir() + "/missing2.db"},
		BtcCfd:   config.BtcCfdConfig{DBPath: t.TempDir() + "/missing3.db"},
	}
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	if body := report.FormatDailyTweaks(cfg, now); body != "" {
		t.Fatalf("expected empty tweaks, got:\n%s", body)
	}
}
