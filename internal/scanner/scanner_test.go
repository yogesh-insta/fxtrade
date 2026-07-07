package scanner

import (
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

func TestBuildOpeningRange(t *testing.T) {
	sessionOpen := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)
	candles := []oanda.Candle{
		{Complete: true, Time: "2026-07-03T07:45:00Z", Mid: oanda.OHLCPrice{O: "1.1000", H: "1.1010", L: "1.0990", C: "1.1005"}},
		{Complete: true, Time: "2026-07-03T08:00:00Z", Mid: oanda.OHLCPrice{O: "1.1000", H: "1.1030", L: "1.0995", C: "1.1020"}},
		{Complete: true, Time: "2026-07-03T08:15:00Z", Mid: oanda.OHLCPrice{O: "1.1020", H: "1.1040", L: "1.1010", C: "1.1035"}},
	}
	gr, ok, reason := BuildOpeningRange(candles, sessionOpen, 2, 0.0001)
	if !ok {
		t.Fatalf("expected range, got reason %q", reason)
	}
	if gr.High != 1.1040 || gr.Low != 1.0995 {
		t.Fatalf("unexpected range high/low: %.4f/%.4f", gr.High, gr.Low)
	}
	if gr.RangePips < 40 || gr.RangePips > 50 {
		t.Fatalf("expected ~45 pips, got %.1f", gr.RangePips)
	}
}

func TestScoreSetupVetoSpread(t *testing.T) {
	_, ok, reason := ScoreSetup(30, 5, 3, 3, 1)
	if ok {
		t.Fatal("expected spread veto")
	}
	if reason != "spread too wide" {
		t.Fatalf("unexpected reason: %q", reason)
	}
}

func TestRankSetupsOrder(t *testing.T) {
	setups := []Setup{
		{Instrument: "EUR_USD", Ready: true, Score: 0.70, RangeSpreadRatio: 5, SpreadPips: 1.2},
		{Instrument: "GBP_JPY", Ready: true, Score: 0.80, RangeSpreadRatio: 4, SpreadPips: 2.0},
	}
	ranked := RankSetups(setups, 0.65)
	if len(ranked) != 2 || ranked[0].Instrument != "GBP_JPY" {
		t.Fatalf("unexpected rank order: %+v", ranked)
	}
}

func TestInSessionFX(t *testing.T) {
	cfg := config.DefaultScannerConfig()
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	if !InSession(cfg, "EUR_USD", "CURRENCY", now) {
		t.Fatal("expected EUR_USD in session at 10:00 UTC")
	}
	now = time.Date(2026, 7, 3, 22, 0, 0, 0, time.UTC)
	if InSession(cfg, "EUR_USD", "CURRENCY", now) {
		t.Fatal("expected EUR_USD outside session at 22:00 UTC")
	}
}

func TestPresetVolatilePhase1Count(t *testing.T) {
	syms := PresetSymbols(config.PresetVolatilePhase1)
	if len(syms) != 19 {
		t.Fatalf("expected 19 symbols, got %d", len(syms))
	}
}

func TestShouldForceFlatFXAt2000UTC(t *testing.T) {
	cfg := config.DefaultScannerConfig()
	before := time.Date(2026, 7, 7, 19, 59, 0, 0, time.UTC)
	at := time.Date(2026, 7, 7, 20, 0, 0, 0, time.UTC)
	after := time.Date(2026, 7, 7, 20, 1, 0, 0, time.UTC)

	if ShouldForceFlat(cfg, "GBP_USD", "CURRENCY", before) {
		t.Fatal("expected no force flat before 20:00 UTC")
	}
	if !ShouldForceFlat(cfg, "GBP_USD", "CURRENCY", at) {
		t.Fatal("expected force flat at 20:00 UTC")
	}
	if !ShouldForceFlat(cfg, "GBP_USD", "CURRENCY", after) {
		t.Fatal("expected force flat after 20:00 UTC")
	}
}

func TestShouldForceFlatCryptoDisabled(t *testing.T) {
	cfg := config.DefaultScannerConfig()
	now := time.Date(2026, 7, 7, 23, 0, 0, 0, time.UTC)
	if ShouldForceFlat(cfg, "BTC_USD", "CFD", now) {
		t.Fatal("expected no force flat for crypto by default")
	}
}
