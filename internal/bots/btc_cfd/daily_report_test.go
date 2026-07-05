package btc_cfd_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/bots/btc_cfd"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestReportDayUTC(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 30, 0, 0, time.UTC)
	got := btc_cfd.ReportDayUTC(now)
	want := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ReportDayUTC = %v want %v", got, want)
	}
}

func TestDailyEmail(t *testing.T) {
	reportDate := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	day := sqlite.DayMetrics{
		Date:       reportDate,
		TradeCount: 2,
		WinCount:   1,
		LossCount:  1,
		WinRate:    0.5,
		TotalNetPL: 7.3,
		TotalFees:  1.5,
		Trades: []sqlite.TradeBrief{
			{Direction: "long", NetPL: 12.5},
			{Direction: "short", NetPL: -5.2},
		},
	}
	all := sqlite.DayMetrics{
		TradeCount: 10,
		WinCount:   6,
		LossCount:  4,
		WinRate:    0.6,
		TotalNetPL: 142.5,
		TotalFees:  18.4,
	}
	open := &btc_cfd.OpenPosition{
		Instrument:   "BTC_USD",
		Direction:    "LONG",
		Units:        1,
		UnrealizedPL: 3.2,
	}

	subject, body := btc_cfd.DailyEmail(reportDate, day, all, open)
	if subject != "fxtrade: BTC daily summary 2026-07-04" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Date: 2026-07-04 UTC",
		"Trades: 2",
		"• LONG  net P&L +$12.50",
		"• SHORT  net P&L -$5.20",
		"Win rate (day): 50.0% (1W / 1L)",
		"Win rate (all-time): 60.0% (6W / 4L)",
		"Net P&L (day): +$7.30",
		"Net P&L (all-time): +$142.50",
		"Fees paid (day): $1.50",
		"Fees paid (all-time): $18.40",
		"Open position: LONG 1 BTC_USD  unrealized +$3.20",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q\n%s", want, body)
		}
	}

	_, bodyNone := btc_cfd.DailyEmail(reportDate, day, all, nil)
	if !strings.Contains(bodyNone, "Open position: none") {
		t.Fatalf("expected no open position line")
	}
}
