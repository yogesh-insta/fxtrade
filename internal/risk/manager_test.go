package risk_test

import (
	"context"
	"testing"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/risk"
)

func TestSizeUnits(t *testing.T) {
	rm := risk.NewManager(config.DefaultRiskConfig())
	units := rm.SizeUnits(100000, 0.0080, 0.5)
	if units != 62500 {
		t.Fatalf("expected 62500 units, got %d", units)
	}
	unitsHigh := rm.SizeUnits(100000, 0.0080, 0.8)
	if unitsHigh != 125000 {
		t.Fatalf("expected 125000 units at high confidence, got %d", unitsHigh)
	}
}

func TestAllowEntryBlocksWideSpread(t *testing.T) {
	rm := risk.NewManager(config.DefaultRiskConfig())
	err := rm.AllowEntry(context.Background(), risk.EntryRequest{
		SpreadPips:     5,
		OpenPositions:  0,
		AccountBalance: 100000,
	})
	if err == nil {
		t.Fatal("expected spread guard error")
	}
}

func TestAllowEntryBlocksMaxPositions(t *testing.T) {
	rm := risk.NewManager(config.DefaultRiskConfig())
	err := rm.AllowEntry(context.Background(), risk.EntryRequest{
		SpreadPips:     1,
		OpenPositions:  1,
		AccountBalance: 100000,
	})
	if err == nil {
		t.Fatal("expected max positions error")
	}
}
