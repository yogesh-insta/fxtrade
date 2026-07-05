package btc_cfd

import (
	"fmt"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/oanda"
)

const (
	atrPeriod    = 14
	atrSMAPeriod = 20
)

// IndicatorSnapshot holds values computed on the last complete M5 bar.
type IndicatorSnapshot struct {
	SignalPrice float64
	RSI         float64
	PrevRSI     float64
	EMA50       float64
	EMA200      float64
	ATR         float64
	ATRSMA20    float64
	Deviation   float64 // (Close - EMA50) / ATR(14)
}

func parseCompleteCandles(candles []oanda.Candle) (closes, highs, lows []float64, err error) {
	for _, c := range candles {
		if !c.Complete {
			continue
		}
		closePx, err := oanda.ParsePrice(c.Mid.C)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse close: %w", err)
		}
		highPx, err := oanda.ParsePrice(c.Mid.H)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse high: %w", err)
		}
		lowPx, err := oanda.ParsePrice(c.Mid.L)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse low: %w", err)
		}
		closes = append(closes, closePx)
		highs = append(highs, highPx)
		lows = append(lows, lowPx)
	}
	if len(closes) == 0 {
		return nil, nil, nil, fmt.Errorf("no complete candles")
	}
	return closes, highs, lows, nil
}

// ComputeIndicators builds RSI(21), EMA50, EMA200, ATR(14), and ATR SMA(20) on complete bars.
func ComputeIndicators(candles []oanda.Candle, cfg config.BtcCfdConfig) (IndicatorSnapshot, error) {
	closes, highs, lows, err := parseCompleteCandles(candles)
	if err != nil {
		return IndicatorSnapshot{}, err
	}
	minBars := 200
	if cfg.RSIPeriod+2 > minBars {
		minBars = cfg.RSIPeriod + 2
	}
	if atrPeriod+atrSMAPeriod > minBars {
		minBars = atrPeriod + atrSMAPeriod
	}
	if len(closes) < minBars {
		return IndicatorSnapshot{}, fmt.Errorf("need %d complete candles, got %d", minBars, len(closes))
	}

	n := len(closes)
	rsiNow := market.RSI(closes[:n], cfg.RSIPeriod)
	rsiPrev := market.RSI(closes[:n-1], cfg.RSIPeriod)
	ema50 := market.EMA(closes, 50)
	ema200 := market.EMA(closes, 200)
	atr := market.ATR(highs, lows, closes, atrPeriod)

	atrValues := make([]float64, 0, atrSMAPeriod)
	for i := n - atrSMAPeriod; i < n; i++ {
		if i < atrPeriod {
			continue
		}
		atrValues = append(atrValues, market.ATR(highs[:i+1], lows[:i+1], closes[:i+1], atrPeriod))
	}
	if len(atrValues) == 0 {
		return IndicatorSnapshot{}, fmt.Errorf("insufficient data for ATR SMA(%d)", atrSMAPeriod)
	}
	var atrSum float64
	for _, v := range atrValues {
		atrSum += v
	}
	atrSMA := atrSum / float64(len(atrValues))

	closePx := closes[n-1]
	deviation := 0.0
	if atr > 0 {
		deviation = (closePx - ema50) / atr
	}

	return IndicatorSnapshot{
		SignalPrice: closePx,
		RSI:         rsiNow,
		PrevRSI:     rsiPrev,
		EMA50:       ema50,
		EMA200:      ema200,
		ATR:         atr,
		ATRSMA20:    atrSMA,
		Deviation:   deviation,
	}, nil
}
