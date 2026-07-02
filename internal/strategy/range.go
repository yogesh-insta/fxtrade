package strategy

import (
	"math"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/oanda"
)

func DetectRange(snap market.Snapshot, cfg config.RangeModeConfig) RangeBand {
	lookback := cfg.RangeLookbackWeeks * 5
	if lookback > len(snap.Daily) {
		lookback = len(snap.Daily)
	}
	if lookback < 10 {
		return RangeBand{}
	}

	daily := snap.Daily[len(snap.Daily)-lookback:]
	high, low := rangeHighLow(daily)

	zone := snap.ATR14Daily * cfg.ZoneATRMultiplier
	band := RangeBand{
		High:               high,
		Low:                low,
		Mid:                (high + low) / 2,
		WidthPips:          (high - low) / oanda.PipSize,
		SupportZoneLow:     low - zone,
		SupportZoneHigh:    low + zone,
		ResistanceZoneLow:  high - zone,
		ResistanceZoneHigh: high + zone,
		BuyLimitPrice:      low,
		SellLimitPrice:     high,
	}

	band.TouchesHigh, band.TouchesLow = countTouches(daily, band.SupportZoneHigh, band.ResistanceZoneLow)

	band.Valid = band.WidthPips >= cfg.MinRangeWidthPips &&
		band.TouchesHigh >= cfg.MinBoundaryTouches &&
		band.TouchesLow >= cfg.MinBoundaryTouches &&
		weeklyEMAsFlat(snap, cfg.FlatEMAThresholdPct)

	return band
}

func rangeHighLow(daily []oanda.Candle) (high, low float64) {
	first := true
	for _, c := range daily {
		h, err1 := oanda.ParsePrice(c.Mid.H)
		l, err2 := oanda.ParsePrice(c.Mid.L)
		if err1 != nil || err2 != nil {
			continue
		}
		if first {
			high, low = h, l
			first = false
			continue
		}
		if h > high {
			high = h
		}
		if l < low {
			low = l
		}
	}
	return high, low
}

func countTouches(daily []oanda.Candle, supportTop, resistanceBottom float64) (highTouches, lowTouches int) {
	for _, c := range daily {
		h, err1 := oanda.ParsePrice(c.Mid.H)
		l, err2 := oanda.ParsePrice(c.Mid.L)
		if err1 != nil || err2 != nil {
			continue
		}
		if l <= supportTop {
			lowTouches++
		}
		if h >= resistanceBottom {
			highTouches++
		}
	}
	return highTouches, lowTouches
}

func weeklyEMAsFlat(snap market.Snapshot, thresholdPct float64) bool {
	if snap.Mid == 0 {
		return false
	}
	diffPct := math.Abs(snap.EMA20W-snap.EMA50W) / snap.Mid * 100
	return diffPct <= thresholdPct
}

func DetectMode(snap market.Snapshot, band RangeBand, cfg config.TrendModeConfig) string {
	if weeklyBreakout(snap, band) || strongWeeklyTrend(snap, cfg) {
		return ModeTrend
	}
	if band.Valid {
		return ModeRange
	}
	return ModeStandAside
}

func weeklyBreakout(snap market.Snapshot, band RangeBand) bool {
	if !band.Valid {
		return false
	}
	return snap.LastWeeklyClose > band.ResistanceZoneHigh || snap.LastWeeklyClose < band.SupportZoneLow
}

func strongWeeklyTrend(snap market.Snapshot, cfg config.TrendModeConfig) bool {
	_ = cfg
	if snap.EMA20W > snap.EMA50W && snap.Mid > snap.EMA20W {
		return true
	}
	if snap.EMA20W < snap.EMA50W && snap.Mid < snap.EMA20W {
		return true
	}
	return false
}
