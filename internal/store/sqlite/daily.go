package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// TradeBrief is a closed trade row for daily summaries.
type TradeBrief struct {
	ClosedAt  time.Time
	Direction string
	NetPL     float64
	SwapCost  float64
}

// DayMetrics aggregates closed trades for one UTC calendar day or all time.
type DayMetrics struct {
	Date       time.Time
	Trades     []TradeBrief
	TradeCount int
	WinCount   int
	LossCount  int
	WinRate    float64
	TotalNetPL float64
	TotalFees  float64
}

// DayMetricsUTC returns stats for trades closed on the given UTC calendar day (00:00–23:59:59).
func (s *Store) DayMetricsUTC(day time.Time) (DayMetrics, error) {
	day = day.UTC()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	return s.metricsBetween(start, end, start)
}

// AllTimeDayMetrics returns aggregate stats for every closed trade in the database.
func (s *Store) AllTimeDayMetrics() (DayMetrics, error) {
	return s.metricsBetween(time.Time{}, time.Time{}, time.Time{})
}

func (s *Store) metricsBetween(start, end, label time.Time) (DayMetrics, error) {
	query := `
SELECT closed_at, direction, COALESCE(net_pl, realized_pl), COALESCE(swap_cost, 0)
FROM trades`
	var args []any
	switch {
	case !start.IsZero() && !end.IsZero():
		query += ` WHERE closed_at >= ? AND closed_at < ?`
		args = append(args, start.Format(time.RFC3339), end.Format(time.RFC3339))
	case !start.IsZero():
		query += ` WHERE closed_at >= ?`
		args = append(args, start.Format(time.RFC3339))
	}
	query += ` ORDER BY closed_at ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return DayMetrics{}, fmt.Errorf("query trades: %w", err)
	}
	defer rows.Close()

	var m DayMetrics
	m.Date = label
	for rows.Next() {
		var closedStr string
		var direction sql.NullString
		var net, swap sql.NullFloat64
		if err := rows.Scan(&closedStr, &direction, &net, &swap); err != nil {
			return DayMetrics{}, err
		}
		closed, err := time.Parse(time.RFC3339, closedStr)
		if err != nil {
			return DayMetrics{}, fmt.Errorf("parse closed_at %q: %w", closedStr, err)
		}
		n := net.Float64
		fee := swap.Float64
		m.Trades = append(m.Trades, TradeBrief{
			ClosedAt:  closed.UTC(),
			Direction: direction.String,
			NetPL:     n,
			SwapCost:  fee,
		})
		m.TradeCount++
		m.TotalNetPL += n
		m.TotalFees += fee
		if n > 0 {
			m.WinCount++
		} else if n < 0 {
			m.LossCount++
		}
	}
	if err := rows.Err(); err != nil {
		return DayMetrics{}, err
	}
	if m.TradeCount > 0 {
		m.WinRate = float64(m.WinCount) / float64(m.TradeCount)
	}
	return m, nil
}
