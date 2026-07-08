package risk_test

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestAllowEntryLiveEnforcesMonthlyCapAndCooldown(t *testing.T) {
	cfg := config.DefaultRiskConfig()
	cfg.MaxTradesPerMonth = 2
	cfg.CooldownAfterLossDays = 3
	rm := risk.NewManagerForEnv(cfg, config.EnvLive)

	rm.RestoreState(risk.State{
		DayStart:        time.Now(),
		WeekStart:       time.Now(),
		MonthKey:        time.Now().Format("2006-01"),
		TradesThisMonth: 2,
	})
	err := rm.AllowEntry(context.Background(), risk.EntryRequest{
		SpreadPips:     1,
		OpenPositions:  0,
		AccountBalance: 100000,
	})
	if err == nil || !strings.Contains(err.Error(), "max trades per month") {
		t.Fatalf("expected monthly cap error on live, got %v", err)
	}

	rm = risk.NewManagerForEnv(cfg, config.EnvLive)
	rm.RestoreState(risk.State{
		DayStart:   time.Now(),
		WeekStart:  time.Now(),
		MonthKey:   time.Now().Format("2006-01"),
		LastLossAt: time.Now().Add(-time.Hour),
	})
	err = rm.AllowEntry(context.Background(), risk.EntryRequest{
		SpreadPips:     1,
		OpenPositions:  0,
		AccountBalance: 100000,
	})
	if err == nil || !strings.Contains(err.Error(), "cooldown after loss") {
		t.Fatalf("expected cooldown error on live, got %v", err)
	}
}

func TestAllowEntryPracticeSkipsMonthlyCapAndCooldown(t *testing.T) {
	cfg := config.DefaultRiskConfig()
	cfg.MaxTradesPerMonth = 2
	cfg.CooldownAfterLossDays = 3
	rm := risk.NewManagerForEnv(cfg, config.EnvPractice)

	rm.RestoreState(risk.State{
		DayStart:        time.Now(),
		WeekStart:       time.Now(),
		MonthKey:        time.Now().Format("2006-01"),
		TradesThisMonth: 99,
		LastLossAt:      time.Now().Add(-time.Minute),
	})
	err := rm.AllowEntry(context.Background(), risk.EntryRequest{
		SpreadPips:     1,
		OpenPositions:  0,
		AccountBalance: 100000,
	})
	if err != nil {
		t.Fatalf("practice should skip trade caps, got %v", err)
	}
}
