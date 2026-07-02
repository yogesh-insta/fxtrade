package market

import "math"

func EMA(values []float64, period int) float64 {
	if len(values) == 0 || period <= 0 {
		return 0
	}
	if len(values) < period {
		period = len(values)
	}
	k := 2.0 / float64(period+1)
	ema := values[0]
	for i := 1; i < len(values); i++ {
		ema = values[i]*k + ema*(1-k)
	}
	return ema
}

func ATR(highs, lows, closes []float64, period int) float64 {
	if len(highs) == 0 || len(lows) == 0 || len(closes) == 0 {
		return 0
	}
	n := len(closes)
	if n < 2 {
		return highs[0] - lows[0]
	}
	trs := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		trs = append(trs, math.Max(hl, math.Max(hc, lc)))
	}
	if len(trs) < period {
		period = len(trs)
	}
	if period == 0 {
		return 0
	}
	sum := 0.0
	for i := len(trs) - period; i < len(trs); i++ {
		sum += trs[i]
	}
	return sum / float64(period)
}

func HighLow(values []float64) (high, low float64) {
	if len(values) == 0 {
		return 0, 0
	}
	high, low = values[0], values[0]
	for _, v := range values[1:] {
		if v > high {
			high = v
		}
		if v < low {
			low = v
		}
	}
	return high, low
}

func PctChange(from, to float64) float64 {
	if from == 0 {
		return 0
	}
	return (to - from) / from * 100
}

func RSI(closes []float64, period int) float64 {
	if len(closes) < period+1 || period <= 0 {
		return 50
	}
	var gains, losses float64
	for i := len(closes) - period; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		if diff >= 0 {
			gains += diff
		} else {
			losses -= diff
		}
	}
	if losses == 0 {
		return 100
	}
	rs := gains / losses
	return 100 - (100 / (1 + rs))
}

func EMASeries(values []float64, period int) []float64 {
	if len(values) == 0 || period <= 0 {
		return nil
	}
	out := make([]float64, len(values))
	out[0] = values[0]
	k := 2.0 / float64(period+1)
	for i := 1; i < len(values); i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}
