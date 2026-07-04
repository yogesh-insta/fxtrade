package stockscan

import "math"

// SMA returns the simple moving average of the last period closes.
func SMA(closes []float64, period int) (float64, bool) {
	if period <= 0 || len(closes) < period {
		return 0, false
	}
	sum := 0.0
	for i := len(closes) - period; i < len(closes); i++ {
		sum += closes[i]
	}
	return sum / float64(period), true
}

// RSI computes Wilder's RSI over the given period.
func RSI(closes []float64, period int) (float64, bool) {
	if period <= 0 || len(closes) < period+1 {
		return 0, false
	}

	var gainSum, lossSum float64
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff >= 0 {
			gainSum += diff
		} else {
			lossSum -= diff
		}
	}
	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)

	for i := period + 1; i < len(closes); i++ {
		diff := closes[i] - closes[i-1]
		var gain, loss float64
		if diff >= 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}

	if avgLoss == 0 {
		if avgGain == 0 {
			return 50, true
		}
		return 100, true
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs)), true
}

// PassesFilter reports whether close is above SMA and RSI is within [rsiMin, rsiMax].
func PassesFilter(close, sma, rsi, rsiMin, rsiMax float64) bool {
	if close <= sma {
		return false
	}
	if rsi < rsiMin || rsi > rsiMax {
		return false
	}
	return true
}

// Levels computes stop-loss and target from entry using fixed percentages.
func Levels(entry, stopLossPct, targetPct float64) (stopLoss, target float64) {
	stopLoss = entry * (1 - stopLossPct)
	target = entry * (1 + targetPct)
	return stopLoss, target
}

// RoundINR rounds to two decimal places for rupee display.
func RoundINR(v float64) float64 {
	return math.Round(v*100) / 100
}
