package scanner

import (
	"math"
	"sort"
)

type Setup struct {
	Instrument        string
	Class             string
	Range             OpeningRange
	SpreadPips        float64
	RangeSpreadRatio  float64
	TrendBias         int
	Score             float64
	InSession         bool
	Ready             bool
	SkipReason        string
	BreakoutDirection string
}

func ScoreSetup(rangePips, spreadPips, maxSpreadPips, minRangeSpreadRatio float64, trendBias int) (score float64, ok bool, reason string) {
	if spreadPips <= 0 {
		return 0, false, "invalid spread"
	}
	if maxSpreadPips > 0 && spreadPips > maxSpreadPips {
		return 0, false, "spread too wide"
	}
	ratio := rangePips / spreadPips
	if minRangeSpreadRatio > 0 && ratio < minRangeSpreadRatio {
		return 0, false, "range/spread ratio too low"
	}

	rangeScore := math.Min(1.0, rangePips/50.0)
	spreadScore := 1.0
	if maxSpreadPips > 0 {
		spreadScore = math.Max(0, 1.0-spreadPips/maxSpreadPips)
	}
	ratioScore := math.Min(1.0, ratio/10.0)
	trendScore := 0.5
	if trendBias != 0 {
		trendScore = 0.75
	}

	score = 0.35*rangeScore + 0.20*spreadScore + 0.30*ratioScore + 0.15*trendScore
	return score, true, ""
}

// BreakoutAlignedWithTrend returns true when direction matches H1 bias, or bias is neutral.
func BreakoutAlignedWithTrend(direction string, trendBias int) bool {
	switch direction {
	case "LONG":
		return trendBias >= 0
	case "SHORT":
		return trendBias <= 0
	default:
		return true
	}
}

func RankSetups(setups []Setup, minScore float64) []Setup {
	var ready []Setup
	for _, s := range setups {
		if !s.Ready {
			continue
		}
		if s.Score < minScore {
			continue
		}
		ready = append(ready, s)
	}
	sort.Slice(ready, func(i, j int) bool {
		a, b := ready[i], ready[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.RangeSpreadRatio != b.RangeSpreadRatio {
			return a.RangeSpreadRatio > b.RangeSpreadRatio
		}
		if a.SpreadPips != b.SpreadPips {
			return a.SpreadPips < b.SpreadPips
		}
		return a.Instrument < b.Instrument
	})
	return ready
}

func DetectBreakout(s Setup, bid, ask float64) (direction string) {
	if bid > s.Range.High {
		return "LONG"
	}
	if ask < s.Range.Low {
		return "SHORT"
	}
	return ""
}
