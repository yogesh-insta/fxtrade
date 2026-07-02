package strategy

import (
	"fmt"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/sentiment"
)

func CheckMarketConditions(snap market.Snapshot, sig sentiment.SentimentSignal, hasSig bool, cfg config.RiskConfig, now time.Time) (bool, string) {
	if snap.SpreadPips > cfg.MaxSpreadPips {
		return false, fmt.Sprintf("spread %.1f pips > max %.1f", snap.SpreadPips, cfg.MaxSpreadPips)
	}
	if inRolloverBlackout(now) {
		return false, "rollover blackout (NY 16:45-18:15)"
	}
	if inFridayEntryBlackout(now) {
		return false, "within 2h of Friday close"
	}
	if hasSig && sig.EventRisk == "high" {
		return false, "LLM event_risk=high"
	}
	return true, ""
}

func inRolloverBlackout(now time.Time) bool {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	t := now.In(ny)
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	mins := t.Hour()*60 + t.Minute()
	start := 16*60 + 45
	end := 18*60 + 15
	return mins >= start && mins <= end
}

func inFridayEntryBlackout(now time.Time) bool {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	t := now.In(ny)
	if t.Weekday() != time.Friday {
		return false
	}
	// block new entries after 15:00 NY on Friday
	return t.Hour() >= 15
}

func SentimentVetoBuy(sig sentiment.SentimentSignal, hasSig bool, cfg config.LLMGateConfig) bool {
	if !hasSig {
		return false
	}
	if sig.EventRisk == "high" {
		return true
	}
	return sig.AUDBias == "bearish" && sig.Confidence >= cfg.VetoConfidence
}

func SentimentVetoSell(sig sentiment.SentimentSignal, hasSig bool, cfg config.LLMGateConfig) bool {
	if !hasSig {
		return false
	}
	if sig.EventRisk == "high" {
		return true
	}
	return sig.AUDBias == "bullish" && sig.Confidence >= cfg.VetoConfidence
}

func SentimentAligned(direction string, sig sentiment.SentimentSignal, hasSig bool) bool {
	if !hasSig {
		return false
	}
	switch direction {
	case "LONG":
		return sig.Direction == "LONG" || sig.AUDBias == "bullish"
	case "SHORT":
		return sig.Direction == "SHORT" || sig.AUDBias == "bearish"
	}
	return false
}

func SentimentConfidence(direction string, sig sentiment.SentimentSignal, hasSig bool, cfg config.LLMGateConfig) float64 {
	if !hasSig {
		return 0.5
	}
	if SentimentAligned(direction, sig, hasSig) && sig.Confidence >= cfg.FullSizeConfidence {
		return sig.Confidence
	}
	if sig.Confidence >= cfg.VetoConfidence {
		return sig.Confidence
	}
	return 0.5
}

func SentimentPersistence(direction string, history []sentiment.SentimentSignal, cfg config.LLMGateConfig) (bool, string) {
	need := cfg.SentimentPersistenceReadings
	if need <= 0 {
		need = 2
	}
	if len(history) < need {
		return false, fmt.Sprintf("need %d sentiment readings, have %d", need, len(history))
	}
	for i := 0; i < need; i++ {
		if history[i].Direction != direction {
			return false, fmt.Sprintf("sentiment reading %d disagrees (%s)", i+1, history[i].Direction)
		}
		if history[i].Confidence < cfg.VetoConfidence {
			return false, fmt.Sprintf("sentiment reading %d low confidence", i+1)
		}
	}
	return true, ""
}

func TrendEntry(snap market.Snapshot, band RangeBand, cfg config.TrendModeConfig) (string, bool, string) {
	// Gate 1 weekly regime
	longRegime := snap.EMA20W > snap.EMA50W && snap.Mid > snap.EMA20W
	shortRegime := snap.EMA20W < snap.EMA50W && snap.Mid < snap.EMA50W
	if !longRegime && !shortRegime {
		return "", false, "weekly regime chop"
	}

	distHigh := (band.High - snap.Mid) / oanda.PipSize
	if band.Valid {
		distHigh = (snap.Week52High - snap.Mid) / oanda.PipSize
	}
	distLow := (snap.Mid - snap.Week52Low) / oanda.PipSize

	if longRegime {
		if distHigh <= cfg.Week52BlockDistancePips && snap.LastWeeklyClose <= snap.Week52High {
			return "", false, "long blocked near 52w high"
		}
		if !h4LongSetup(snap, cfg) {
			return "", false, "H4 long setup not met"
		}
		return "LONG", true, ""
	}

	if distLow <= cfg.Week52BlockDistancePips && snap.LastWeeklyClose >= snap.Week52Low {
		return "", false, "short blocked near 52w low"
	}
	if !h4ShortSetup(snap, cfg) {
		return "", false, "H4 short setup not met"
	}
	return "SHORT", true, ""
}

func h4LongSetup(snap market.Snapshot, cfg config.TrendModeConfig) bool {
	nearEMA := mathAbs(snap.LastH4Close-snap.EMA20H4) <= snap.ATR14Daily
	return nearEMA &&
		snap.RSI14H4 >= cfg.RSILongMin && snap.RSI14H4 <= cfg.RSILongMax &&
		snap.LastH4Close >= snap.EMA20H4
}

func h4ShortSetup(snap market.Snapshot, cfg config.TrendModeConfig) bool {
	nearEMA := mathAbs(snap.LastH4Close-snap.EMA20H4) <= snap.ATR14Daily
	return nearEMA &&
		snap.RSI14H4 >= cfg.RSIShortMin && snap.RSI14H4 <= cfg.RSIShortMax &&
		snap.LastH4Close <= snap.EMA20H4
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
