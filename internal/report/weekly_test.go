package report_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestReportWeekUTC(t *testing.T) {
	now := time.Date(2026, 7, 6, 7, 0, 0, 0, time.UTC)
	start, end := report.ReportWeekUTC(now)
	wantStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("window = %v–%v want %v–%v", start, end, wantStart, wantEnd)
	}
}

func TestWeeklyEmail(t *testing.T) {
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	end := start.Add(7 * 24 * time.Hour)

	sections := []report.BotWeeklySection{
		{
			BotID: config.BotBtcCfd,
			Period: sqlite.PeriodMetrics{
				DayMetrics: sqlite.DayMetrics{
					TradeCount: 2,
					WinCount:   1,
					LossCount:  1,
					WinRate:    0.5,
					TotalNetPL: 7.3,
					TotalFees:  1.0,
				},
				MaxDrawdown: 5.2,
				BestNetPL:   12.5,
				WorstNetPL:  -5.2,
				AvgWin:      12.5,
				AvgLoss:     -5.2,
				RealizedRR:  2.4,
			},
			AllTime: sqlite.DayMetrics{TradeCount: 10, TotalNetPL: 142.5},
		},
		{
			BotID: config.BotFxSentiment,
		},
		{
			BotID: config.BotUniverseScanner,
			Period: sqlite.PeriodMetrics{
				DayMetrics: sqlite.DayMetrics{TradeCount: 0},
			},
		},
	}
	sections[1].Err = errNoData("database not found")

	subject, body := report.WeeklyEmail(start, end, sections)
	if subject != "fxtrade: weekly bot summary 2026-06-29 – 2026-07-05" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Period: 2026-06-29 – 2026-07-05 UTC",
		"── BTC CFD ──",
		"Trades: 2",
		"Net P&L: +$7.30",
		"Max drawdown: $5.20",
		"Best / worst trade: +$12.50 / -$5.20",
		"── FX Sentiment ──",
		"(no data",
		"── Universe Scanner ──",
		"No closed trades this week.",
		"Combined net P&L: +$7.30",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q\n%s", want, body)
		}
	}
}

type errNoData string

func (e errNoData) Error() string { return string(e) }
