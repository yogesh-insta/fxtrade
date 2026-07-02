package report_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/report"
)

func TestExpectancyFromTrades(t *testing.T) {
	trades := []journal.TradeRecord{
		{RealizedPL: 100, Confidence: 0.8},
		{RealizedPL: 50, Confidence: 0.7},
		{RealizedPL: -50, Confidence: 0.6},
		{RealizedPL: -25, Confidence: 0.8},
	}
	r := report.ExpectancyFromTrades(trades)
	if r.TradeCount != 4 {
		t.Fatalf("expected 4 trades, got %d", r.TradeCount)
	}
	if r.WinCount != 2 || r.LossCount != 2 {
		t.Fatalf("unexpected win/loss: %d/%d", r.WinCount, r.LossCount)
	}
	if r.Expectancy <= 0 {
		t.Fatalf("expected positive expectancy, got %f", r.Expectancy)
	}
}
