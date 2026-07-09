package btc_cfd

import (
	"math"
	"strconv"
)

const (
	btcMinTradeUnits    = 0.001 // OANDA BTC_USD minimum
	btcTradeUnitsPrec   = 3
)

// PositionUnits sizes a BTC CFD position per spec §5.
// Returns OANDA-formatted fractional units (e.g. "0.042"); empty when below minimum.
func PositionUnits(balance, stopDistance, riskPct float64) string {
	units, ok := positionUnitsRaw(balance, stopDistance, riskPct)
	if !ok {
		return ""
	}
	return formatBTCUnits(units)
}

func positionUnitsRaw(balance, stopDistance, riskPct float64) (float64, bool) {
	if balance <= 0 || stopDistance <= 0 || riskPct <= 0 {
		return 0, false
	}
	riskAmount := balance * riskPct / 100
	raw := riskAmount / stopDistance
	if raw < btcMinTradeUnits {
		return 0, false
	}
	scale := math.Pow10(btcTradeUnitsPrec)
	units := math.Floor(raw*scale) / scale
	if units < btcMinTradeUnits {
		return 0, false
	}
	return units, true
}

func formatBTCUnits(units float64) string {
	return strconv.FormatFloat(units, 'f', btcTradeUnitsPrec, 64)
}
