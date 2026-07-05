package btc_cfd

import (
	"fmt"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
)

// EntryLimits captures daily caps and open-position state for signal gating.
type EntryLimits struct {
	HasOpenPosition bool
	TradesToday     int
	DailyPnL        float64
	AccountBalance  float64
	SpreadUSD       float64
}

// SignalResult is the strategy output for one M5 cycle.
type SignalResult struct {
	Direction string // execution.DirectionLong, DirectionShort, or empty
	Reason    string
	Hold      bool
}

// EvaluateSignal applies spec §4 entry/hold rules on the latest indicator snapshot.
// Phase-2 gaps: M15 confirmation, tradable-hours window, idempotent client order IDs.
func EvaluateSignal(ind IndicatorSnapshot, cfg config.BtcCfdConfig, lim EntryLimits) SignalResult {
	if lim.HasOpenPosition {
		return SignalResult{Hold: true, Reason: "open position"}
	}
	if lim.TradesToday >= cfg.MaxTradesPerDay {
		return SignalResult{Hold: true, Reason: fmt.Sprintf("max trades/day (%d)", cfg.MaxTradesPerDay)}
	}
	if lim.AccountBalance > 0 {
		cap := lim.AccountBalance * cfg.MaxDailyLossPct / 100
		if lim.DailyPnL <= -cap {
			return SignalResult{Hold: true, Reason: fmt.Sprintf("daily loss cap (%.2f)", lim.DailyPnL)}
		}
	}
	if lim.SpreadUSD > cfg.MaxSpreadUSD {
		return SignalResult{Hold: true, Reason: fmt.Sprintf("spread %.2f > max %.2f", lim.SpreadUSD, cfg.MaxSpreadUSD)}
	}
	if ind.ATR <= 0 {
		return SignalResult{Hold: true, Reason: "ATR unavailable"}
	}
	if ind.ATRSMA20 > 0 && ind.ATR > cfg.ATRSpikeMultiple*ind.ATRSMA20 {
		return SignalResult{Hold: true, Reason: "ATR volatility spike"}
	}

	longCross := ind.PrevRSI < 30 && ind.RSI >= 30
	shortCross := ind.PrevRSI > 70 && ind.RSI <= 70
	if !longCross && !shortCross && ind.RSI > 30 && ind.RSI < 70 {
		return SignalResult{Hold: true, Reason: "RSI mid-band (no cross-back)"}
	}

	longOK := longCross &&
		ind.SignalPrice > ind.EMA200 &&
		ind.Deviation <= -cfg.DeviationATR

	shortOK := shortCross &&
		ind.SignalPrice < ind.EMA200 &&
		ind.Deviation >= cfg.DeviationATR

	switch {
	case longOK:
		return SignalResult{Direction: execution.DirectionLong, Reason: "RSI cross-back long"}
	case shortOK:
		return SignalResult{Direction: execution.DirectionShort, Reason: "RSI cross-back short"}
	default:
		return SignalResult{Hold: true, Reason: "no entry setup"}
	}
}

// StaleSignal reports whether live price drift exceeds max_slippage_pct vs signal close.
func StaleSignal(signalPrice, livePrice, maxSlippagePct float64) bool {
	if signalPrice <= 0 || maxSlippagePct <= 0 {
		return false
	}
	driftPct := (livePrice - signalPrice) / signalPrice
	if driftPct < 0 {
		driftPct = -driftPct
	}
	return driftPct*100 > maxSlippagePct
}

// StopTakeProfit derives SL/TP distances from ATR per spec §5.
func StopTakeProfit(direction string, entry, atr float64, cfg config.BtcCfdConfig) (stop, tp float64, ok bool, reason string) {
	if entry <= 0 || atr <= 0 {
		return 0, 0, false, "invalid entry or ATR"
	}
	slDist := atr * cfg.SLATRMultiple
	tpDist := slDist * cfg.TargetRR
	if tpDist/entry*100 < 0.2 || tpDist/entry*100 > 1.5 {
		return 0, 0, false, fmt.Sprintf("TP distance %.4f%% outside sanity band", tpDist/entry*100)
	}
	switch direction {
	case execution.DirectionLong:
		return entry - slDist, entry + tpDist, true, ""
	case execution.DirectionShort:
		return entry + slDist, entry - tpDist, true, ""
	default:
		return 0, 0, false, "unknown direction"
	}
}
