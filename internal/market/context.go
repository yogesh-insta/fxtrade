package market

import (
	"context"
	"fmt"
	"time"

	"github.com/ym/fxtrade/internal/oanda"
)

type PriceContext struct {
	CurrentBid              float64 `json:"current_bid"`
	CurrentAsk              float64 `json:"current_ask"`
	SpreadPips              float64 `json:"spread_pips"`
	Change1hPct             float64 `json:"change_1h_pct"`
	Change24hPct            float64 `json:"change_24h_pct"`
	EMA20W                  float64 `json:"ema20_w"`
	EMA50W                  float64 `json:"ema50_w"`
	Week52High              float64 `json:"week52_high"`
	Week52Low               float64 `json:"week52_low"`
	DistanceTo52wHighPips   float64 `json:"distance_to_52w_high_pips"`
	DistanceTo52wLowPips    float64 `json:"distance_to_52w_low_pips"`
	RangePositionPct        float64 `json:"range_position_pct"`
	ATR14DailyPips          float64 `json:"atr14_daily_pips"`
	Trend                   string  `json:"trend"`
	Session                 string  `json:"session"`
}

func BuildPriceContext(ctx context.Context, client *oanda.Client, instrument string, now time.Time) (PriceContext, error) {
	if instrument == "" {
		instrument = oanda.DefaultInstrument
	}

	pricing, err := client.Pricing(ctx, instrument)
	if err != nil {
		return PriceContext{}, err
	}
	if len(pricing.Prices) == 0 {
		return PriceContext{}, fmt.Errorf("no pricing for %s", instrument)
	}
	tick, err := pricing.Prices[0].ToUpdate()
	if err != nil {
		return PriceContext{}, err
	}

	h1, err := client.Candles(ctx, instrument, "H1", 30)
	if err != nil {
		return PriceContext{}, err
	}
	daily, err := client.Candles(ctx, instrument, "D", 20)
	if err != nil {
		return PriceContext{}, err
	}
	weekly, err := client.Candles(ctx, instrument, "W", 52)
	if err != nil {
		return PriceContext{}, err
	}

	h1Closes := candleCloses(h1.Candles)
	dHighs, dLows, dCloses := candleOHLC(daily.Candles)
	wCloses := candleCloses(weekly.Candles)

	pc := PriceContext{
		CurrentBid:  tick.Bid,
		CurrentAsk:  tick.Ask,
		SpreadPips:  oanda.SpreadPips(tick.Spread),
		EMA20W:      EMA(wCloses, 20),
		EMA50W:      EMA(wCloses, 50),
		Session:     tradingSession(now),
	}
	pc.Week52High, pc.Week52Low = HighLow(wCloses)
	mid := (tick.Bid + tick.Ask) / 2
	pc.DistanceTo52wHighPips = (pc.Week52High - mid) / oanda.PipSize
	pc.DistanceTo52wLowPips = (mid - pc.Week52Low) / oanda.PipSize
	if pc.Week52High > pc.Week52Low {
		pc.RangePositionPct = (mid - pc.Week52Low) / (pc.Week52High - pc.Week52Low) * 100
	}

	atr := ATR(dHighs, dLows, dCloses, 14)
	pc.ATR14DailyPips = atr / oanda.PipSize

	if len(h1Closes) >= 2 {
		pc.Change1hPct = PctChange(h1Closes[len(h1Closes)-2], h1Closes[len(h1Closes)-1])
	}
	if len(h1Closes) >= 25 {
		pc.Change24hPct = PctChange(h1Closes[len(h1Closes)-25], h1Closes[len(h1Closes)-1])
	}

	switch {
	case pc.EMA20W > pc.EMA50W:
		pc.Trend = "bullish"
	case pc.EMA20W < pc.EMA50W:
		pc.Trend = "bearish"
	default:
		pc.Trend = "neutral"
	}

	return pc, nil
}

func candleCloses(candles []oanda.Candle) []float64 {
	out := make([]float64, 0, len(candles))
	for _, c := range candles {
		if p, err := oanda.ParsePrice(c.Mid.C); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func candleOHLC(candles []oanda.Candle) (highs, lows, closes []float64) {
	for _, c := range candles {
		h, err1 := oanda.ParsePrice(c.Mid.H)
		l, err2 := oanda.ParsePrice(c.Mid.L)
		cl, err3 := oanda.ParsePrice(c.Mid.C)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		highs = append(highs, h)
		lows = append(lows, l)
		closes = append(closes, cl)
	}
	return highs, lows, closes
}

// DailyOHLC exports parsed daily candle prices for strategy code.
func DailyOHLC(candles []oanda.Candle) (highs, lows, closes []float64) {
	return candleOHLC(candles)
}

func tradingSession(now time.Time) string {
	h := now.UTC().Hour()
	switch {
	case h >= 13 && h < 17:
		return "london_ny_overlap"
	case h >= 7 && h < 16:
		return "london"
	case h >= 12 && h < 21:
		return "new_york"
	case h >= 22 || h < 6:
		return "sydney"
	default:
		return "off_hours"
	}
}
