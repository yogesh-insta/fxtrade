package btc_cfd_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/bots/btc_cfd"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

func TestComputeIndicatorsOnFlatSeries(t *testing.T) {
	cfg := config.DefaultBtcCfdConfig()
	candles := make([]oanda.Candle, 220)
	for i := range candles {
		candles[i].Complete = true
		candles[i].Mid = oanda.OHLCPrice{C: "100000.0", H: "100100.0", L: "99900.0"}
	}
	ind, err := btc_cfd.ComputeIndicators(candles, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ind.ATR <= 0 {
		t.Fatalf("ATR = %f", ind.ATR)
	}
	if ind.EMA50 <= 0 || ind.EMA200 <= 0 {
		t.Fatalf("EMA50=%f EMA200=%f", ind.EMA50, ind.EMA200)
	}
	if ind.Deviation != 0 {
		t.Fatalf("deviation on flat series = %f, want 0", ind.Deviation)
	}
}
