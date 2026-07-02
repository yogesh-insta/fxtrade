package strategy_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/strategy"
)

func TestDetectRangeValidBand(t *testing.T) {
	cfg := config.DefaultRangeModeConfig()
	cfg.MinRangeWidthPips = 100
	cfg.MinBoundaryTouches = 2

	daily := make([]oanda.Candle, 0, 70)
	for i := 0; i < 70; i++ {
		low := 0.6500
		high := 0.6900
		if i%10 == 0 {
			low = 0.6500
		}
		if i%12 == 0 {
			high = 0.6900
		}
		daily = append(daily, oanda.Candle{
			Complete: true,
			Mid: oanda.OHLCPrice{
				O: "0.6700", H: format(high), L: format(low), C: "0.6700",
			},
		})
	}

	snap := market.Snapshot{
		Mid:        0.6700,
		Daily:      daily,
		ATR14Daily: 0.0050,
		EMA20W:     0.6690,
		EMA50W:     0.6685,
	}
	band := strategy.DetectRange(snap, cfg)
	if !band.Valid {
		t.Fatalf("expected valid range, got %+v", band)
	}
	if band.WidthPips < 100 {
		t.Fatalf("unexpected width: %f", band.WidthPips)
	}
}

func TestDetectModeStandAsideWhenNoRange(t *testing.T) {
	snap := market.Snapshot{Mid: 0.67, EMA20W: 0.67, EMA50W: 0.67}
	band := strategy.RangeBand{Valid: false}
	mode := strategy.DetectMode(snap, band, config.DefaultTrendModeConfig())
	if mode != strategy.ModeStandAside {
		t.Fatalf("expected stand aside, got %s", mode)
	}
}

func format(p float64) string {
	return oanda.FormatPrice(p)
}
