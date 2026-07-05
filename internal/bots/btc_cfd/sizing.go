package btc_cfd

import "math"

// PositionUnits sizes a BTC CFD position per spec §5.
func PositionUnits(balance, stopDistance, riskPct float64) int64 {
	if balance <= 0 || stopDistance <= 0 || riskPct <= 0 {
		return 0
	}
	riskAmount := balance * riskPct / 100
	units := riskAmount / stopDistance
	if units < 1 {
		return 0
	}
	return int64(math.Floor(units))
}
