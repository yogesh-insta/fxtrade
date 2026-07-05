package sqlite

import (
	"math"
	"time"
)

// PeriodMetrics aggregates closed trades over [start, endExclusive) UTC.
type PeriodMetrics struct {
	DayMetrics
	Start        time.Time
	EndExclusive time.Time
	MaxDrawdown  float64
	BestNetPL    float64
	WorstNetPL   float64
	AvgWin       float64
	AvgLoss      float64
	RealizedRR   float64
}

// PeriodMetricsUTC returns stats for trades closed in [start, endExclusive).
func (s *Store) PeriodMetricsUTC(start, endExclusive time.Time) (PeriodMetrics, error) {
	start = start.UTC()
	endExclusive = endExclusive.UTC()
	dm, err := s.metricsBetween(start, endExclusive, start)
	if err != nil {
		return PeriodMetrics{}, err
	}

	pm := PeriodMetrics{
		DayMetrics:   dm,
		Start:        start,
		EndExclusive: endExclusive,
	}

	var wins, losses []float64
	peak := 0.0
	equity := 0.0
	hasBest, hasWorst := false, false

	for _, t := range dm.Trades {
		n := t.NetPL
		if !hasBest || n > pm.BestNetPL {
			pm.BestNetPL = n
			hasBest = true
		}
		if !hasWorst || n < pm.WorstNetPL {
			pm.WorstNetPL = n
			hasWorst = true
		}
		if n > 0 {
			wins = append(wins, n)
		} else if n < 0 {
			losses = append(losses, n)
		}
		equity += n
		if equity > peak {
			peak = equity
		}
		if dd := peak - equity; dd > pm.MaxDrawdown {
			pm.MaxDrawdown = dd
		}
	}

	pm.AvgWin = avg(wins)
	pm.AvgLoss = avg(losses)
	if pm.AvgLoss != 0 {
		pm.RealizedRR = math.Abs(pm.AvgWin / pm.AvgLoss)
	}
	return pm, nil
}
