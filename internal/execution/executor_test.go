package execution_test

import (
	"context"
	"testing"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/risk"
)

func TestPlaceMarketDryRunSkipsAccounting(t *testing.T) {
	cfg := config.DefaultRiskConfig()
	cfg.MaxOpenPositions = 1
	cfg.MaxSpreadPips = 5
	rm := risk.NewManager(cfg)
	exec := execution.NewExecutor(nil, rm, notify.LogNotifier{})
	exec.SetDryRun(true)

	result, err := exec.PlaceMarket(context.Background(), risk.EntryRequest{
		CorrelationID:  "test-corr",
		SpreadPips:     1.5,
		OpenPositions:  0,
		Confidence:     1,
		AccountBalance: 1000,
		StopDistance:   0.001,
	}, execution.MarketOrderParams{
		Instrument: "AUD_USD",
		Direction:  execution.DirectionLong,
		Units:      1000,
		StopLoss:   0.65000,
	})
	if err != nil {
		t.Fatalf("PlaceMarket dry-run: %v", err)
	}
	if result.TradeID == "" {
		t.Fatal("expected synthetic trade id")
	}
	if snap := rm.Snapshot(); snap.TradesOpened != 0 {
		t.Fatalf("dry-run should not record trade opened, got %d", snap.TradesOpened)
	}
}

func TestPlaceMarketDryRunRejectsWhenRiskBlocks(t *testing.T) {
	cfg := config.DefaultRiskConfig()
	cfg.MaxOpenPositions = 0
	rm := risk.NewManager(cfg)
	exec := execution.NewExecutor(nil, rm, notify.LogNotifier{})
	exec.SetDryRun(true)

	_, err := exec.PlaceMarket(context.Background(), risk.EntryRequest{
		SpreadPips:     1,
		OpenPositions:  1,
		AccountBalance: 1000,
		StopDistance:   0.001,
	}, execution.MarketOrderParams{
		Direction: execution.DirectionLong,
		Units:     100,
		StopLoss:  0.65,
	})
	if err == nil {
		t.Fatal("expected risk error in dry-run")
	}
}
