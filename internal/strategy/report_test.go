package strategy

import (
	"strings"
	"testing"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/sentiment"
)

func TestFormatCycleEmailKillSwitchNoNaN(t *testing.T) {
	cfg := &config.Config{
		RangeMode: config.DefaultRangeModeConfig(),
	}
	body := FormatCycleEmail(market.Snapshot{}, RangeBand{}, cfg, CycleSummary{
		Mode:         ModeStandAside,
		Action:       "no_trade",
		Reason:       "kill switch active",
		IntervalMins: 30,
	}, sentiment.SentimentSignal{}, false)

	if strings.Contains(body, "NaN") {
		t.Fatalf("unexpected NaN in body: %s", body)
	}
	if !strings.Contains(body, "kill switch active") {
		t.Fatalf("missing kill switch reason: %s", body)
	}
}
