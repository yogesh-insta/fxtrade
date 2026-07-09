package btc_cfd

import (
	"testing"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/oanda"
)

func TestEvaluateSignalLongEntry(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	ind := IndicatorSnapshot{
		SignalPrice: 100000,
		PrevRSI:     28,
		RSI:         32,
		EMA200:      99000,
		Deviation:   -2.0,
		ATR:         500,
		ATRSMA20:    400,
	}
	sig := EvaluateSignal(ind, cfg, EntryLimits{SpreadUSD: 10, AccountBalance: 10000, Tradeable: true})
	if sig.Direction != execution.DirectionLong {
		t.Fatalf("direction = %q, want LONG (%s)", sig.Direction, sig.Reason)
	}
}

func TestEvaluateSignalHoldOnATRSpike(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	ind := IndicatorSnapshot{
		PrevRSI:   28,
		RSI:       32,
		EMA200:    99000,
		Deviation: -2.0,
		ATR:       1000,
		ATRSMA20:  400,
	}
	sig := EvaluateSignal(ind, cfg, EntryLimits{SpreadUSD: 10, Tradeable: true})
	if !sig.Hold || sig.Direction != "" {
		t.Fatalf("expected hold on ATR spike, got %+v", sig)
	}
}

func TestEvaluateSignalHoldWhenNotTradeable(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	ind := IndicatorSnapshot{PrevRSI: 28, RSI: 32, EMA200: 99000, Deviation: -2, ATR: 500, ATRSMA20: 400}
	sig := EvaluateSignal(ind, cfg, EntryLimits{Tradeable: false})
	if sig.Reason != "instrument not tradeable (OANDA schedule/liquidity)" {
		t.Fatalf("reason = %q", sig.Reason)
	}
}

func TestEvaluateSignalM15Confirmation(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	cfg.M15Confirmation = true
	ind := IndicatorSnapshot{
		SignalPrice: 100000,
		PrevRSI:     28,
		RSI:         32,
		EMA200:      99000,
		Deviation:   -2.0,
		ATR:         500,
		ATRSMA20:    400,
	}
	// M15 below EMA200 blocks long
	sig := EvaluateSignal(ind, cfg, EntryLimits{SpreadUSD: 10, Tradeable: true, M15Close: 98000, M15EMA200: 99000})
	if sig.Direction != "" {
		t.Fatalf("expected hold, got %+v", sig)
	}
}

func TestEvaluateSignalHoldWhenOpen(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	ind := IndicatorSnapshot{PrevRSI: 28, RSI: 32, EMA200: 99000, Deviation: -2, ATR: 500, ATRSMA20: 400}
	sig := EvaluateSignal(ind, cfg, EntryLimits{HasOpenPosition: true, Tradeable: true})
	if sig.Reason != "open position" {
		t.Fatalf("reason = %q", sig.Reason)
	}
}

func TestStaleSignal(t *testing.T) {
	if !StaleSignal(100000, 100200, 0.1) {
		t.Fatal("expected stale at 0.2% drift with 0.1% max")
	}
	if StaleSignal(100000, 100050, 0.1) {
		t.Fatal("expected fresh at 0.05% drift")
	}
}

func TestStopTakeProfitLong(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	stop, tp, ok, reason := StopTakeProfit(execution.DirectionLong, 100000, 500, cfg)
	if !ok {
		t.Fatalf("unexpected fail: %s", reason)
	}
	if stop >= 100000 || tp <= 100000 {
		t.Fatalf("stop=%f tp=%f", stop, tp)
	}
}

func TestComputeIndicatorsNeedsEnoughBars(t *testing.T) {
	candles := make([]oanda.Candle, 50)
	for i := range candles {
		candles[i].Complete = true
		candles[i].Mid = oanda.OHLCPrice{C: "100.0", H: "101.0", L: "99.0"}
	}
	_, err := ComputeIndicators(candles, config.DefaultBtcCfdConfig())
	if err == nil {
		t.Fatal("expected error for short history")
	}
}

func TestPositionUnits(t *testing.T) {
	if got := PositionUnits(10000, 500, 0.5); got != "0.100" {
		t.Fatalf("units = %q, want 0.100", got)
	}
	if got := PositionUnits(100000, 500, 0.5); got != "1.000" {
		t.Fatalf("units = %q, want 1.000", got)
	}
	if got := PositionUnits(5000, 600, 0.5); got != "0.041" {
		t.Fatalf("units = %q, want 0.041 for $5k slice", got)
	}
	if got := PositionUnits(5000, 50, 0.5); got != "0.500" {
		t.Fatalf("units = %q, want 0.500", got)
	}
	if got := PositionUnits(10, 500, 0.5); got != "" {
		t.Fatalf("units = %q, want empty below minimum", got)
	}
}

func TestReconcileMaxTradesPerDay(t *testing.T) {
	n := config.DefaultBtcCfdConfig().MaxTradesPerDay
	if n != 5 {
		t.Fatalf("max trades = %d, want 5", n)
	}
}
