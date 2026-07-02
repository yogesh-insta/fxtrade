package report

import (
	"fmt"
	"math"

	"github.com/ym/fxtrade/internal/journal"
)

type ExpectancyReport struct {
	TradeCount   int     `json:"trade_count"`
	WinCount     int     `json:"win_count"`
	LossCount    int     `json:"loss_count"`
	WinRate      float64 `json:"win_rate"`
	AvgWin       float64 `json:"avg_win"`
	AvgLoss      float64 `json:"avg_loss"`
	Expectancy   float64 `json:"expectancy"`
	TotalPL      float64 `json:"total_pl"`
	HighConfWins int     `json:"high_conf_wins"`
	HighConfN    int     `json:"high_conf_trades"`
}

func ExpectancyFromTrades(trades []journal.TradeRecord) ExpectancyReport {
	var wins, losses []float64
	var highConfWins, highConfN int
	r := ExpectancyReport{}

	for _, t := range trades {
		r.TradeCount++
		r.TotalPL += t.RealizedPL
		if t.Confidence >= 0.75 {
			highConfN++
			if t.RealizedPL > 0 {
				highConfWins++
			}
		}
		if t.RealizedPL > 0 {
			wins = append(wins, t.RealizedPL)
		} else if t.RealizedPL < 0 {
			losses = append(losses, t.RealizedPL)
		}
	}

	r.WinCount = len(wins)
	r.LossCount = len(losses)
	r.HighConfWins = highConfWins
	r.HighConfN = highConfN

	if r.TradeCount > 0 {
		r.WinRate = float64(r.WinCount) / float64(r.TradeCount)
	}
	if len(wins) > 0 {
		r.AvgWin = avg(wins)
	}
	if len(losses) > 0 {
		r.AvgLoss = avg(losses)
	}

	lossRate := 1.0 - r.WinRate
	r.Expectancy = r.WinRate*r.AvgWin + lossRate*r.AvgLoss

	return r
}

func (r ExpectancyReport) String() string {
	return fmt.Sprintf(
		"trades=%d win_rate=%.1f%% avg_win=%.2f avg_loss=%.2f expectancy=%.2f total_pl=%.2f high_conf=%d/%d",
		r.TradeCount, r.WinRate*100, r.AvgWin, r.AvgLoss, r.Expectancy, r.TotalPL, r.HighConfWins, r.HighConfN,
	)
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}
