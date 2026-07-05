package sqlite

import (
	"database/sql"
	"fmt"
	"math"
	"time"
)

// Metrics aggregates closed-trade performance (spec §11).
type Metrics struct {
	TradeCount       int     `json:"trade_count"`
	WinCount         int     `json:"win_count"`
	LossCount        int     `json:"loss_count"`
	WinRate          float64 `json:"win_rate"`
	AvgWinGross      float64 `json:"avg_win_gross"`
	AvgLossGross     float64 `json:"avg_loss_gross"`
	AvgWinNet        float64 `json:"avg_win_net"`
	AvgLossNet       float64 `json:"avg_loss_net"`
	RealizedRR       float64 `json:"realized_rr"`
	MaxDrawdown      float64 `json:"max_drawdown"`
	TotalNetPL       float64 `json:"total_net_pl"`
	DailyPnL         float64 `json:"daily_pnl"`
	DailyLossCap     float64 `json:"daily_loss_cap"`
	DailyHeadroom    float64 `json:"daily_headroom"`
	TotalSwapCost    float64 `json:"total_swap_cost"`
	AvgSlippagePct   float64 `json:"avg_slippage_pct"`
}

// MetricsReport returns performance stats; accountBalance and maxDailyLossPct drive daily headroom.
func (s *Store) MetricsReport(accountBalance, maxDailyLossPct float64) (Metrics, error) {
	rows, err := s.db.Query(`
SELECT closed_at, realized_pl, COALESCE(net_pl, realized_pl), COALESCE(swap_cost, 0), COALESCE(slippage_pct, 0)
FROM trades ORDER BY closed_at ASC`)
	if err != nil {
		return Metrics{}, fmt.Errorf("query trades: %w", err)
	}
	defer rows.Close()

	var m Metrics
	var winsGross, lossesGross, winsNet, lossesNet []float64
	var slippageSum float64
	var slippageN int
	peak := 0.0
	equity := 0.0
	maxDD := 0.0
	today := time.Now().UTC().Format("2006-01-02")

	for rows.Next() {
		var closedStr string
		var gross, net, swap, slip sql.NullFloat64
		if err := rows.Scan(&closedStr, &gross, &net, &swap, &slip); err != nil {
			return Metrics{}, err
		}
		g := gross.Float64
		n := net.Float64
		if !net.Valid {
			n = g - swap.Float64
		}
		m.TradeCount++
		m.TotalNetPL += n
		m.TotalSwapCost += swap.Float64
		if slip.Valid && slip.Float64 != 0 {
			slippageSum += slip.Float64
			slippageN++
		}
		if closedStr >= today {
			m.DailyPnL += n
		}
		if g > 0 {
			winsGross = append(winsGross, g)
		} else if g < 0 {
			lossesGross = append(lossesGross, g)
		}
		if n > 0 {
			winsNet = append(winsNet, n)
		} else if n < 0 {
			lossesNet = append(lossesNet, n)
		}
		equity += n
		if equity > peak {
			peak = equity
		}
		if dd := peak - equity; dd > maxDD {
			maxDD = dd
		}
	}
	if err := rows.Err(); err != nil {
		return Metrics{}, err
	}

	m.WinCount = len(winsNet)
	m.LossCount = len(lossesNet)
	if m.TradeCount > 0 {
		m.WinRate = float64(m.WinCount) / float64(m.TradeCount)
	}
	m.AvgWinGross = avg(winsGross)
	m.AvgLossGross = avg(lossesGross)
	m.AvgWinNet = avg(winsNet)
	m.AvgLossNet = avg(lossesNet)
	if m.AvgLossNet != 0 {
		m.RealizedRR = math.Abs(m.AvgWinNet / m.AvgLossNet)
	}
	m.MaxDrawdown = maxDD
	if slippageN > 0 {
		m.AvgSlippagePct = slippageSum / float64(slippageN)
	}
	if accountBalance > 0 && maxDailyLossPct > 0 {
		m.DailyLossCap = accountBalance * maxDailyLossPct / 100
		m.DailyHeadroom = m.DailyLossCap + m.DailyPnL
	}
	return m, nil
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
