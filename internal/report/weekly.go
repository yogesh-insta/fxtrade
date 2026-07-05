package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

// BotWeeklySection is one bot's performance for the weekly email.
type BotWeeklySection struct {
	BotID   string
	DBPath  string
	Period  sqlite.PeriodMetrics
	AllTime sqlite.DayMetrics
	Err     error // set when the database could not be read
}

// ReportWeekUTC returns the UTC window for a weekly summary sent at now.
// Covers the previous seven full UTC calendar days ending yesterday (inclusive).
func ReportWeekUTC(now time.Time) (start, endExclusive time.Time) {
	y, m, d := now.UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	endExclusive = today
	start = today.Add(-7 * 24 * time.Hour)
	return start, endExclusive
}

// WeeklyEmail formats the combined weekly performance subject and body.
func WeeklyEmail(weekStart, weekEndExclusive time.Time, sections []BotWeeklySection) (subject, body string) {
	endDay := weekEndExclusive.Add(-24 * time.Hour)
	startStr := weekStart.Format("2006-01-02")
	endStr := endDay.Format("2006-01-02")
	subject = fmt.Sprintf("fxtrade: weekly bot summary %s – %s", startStr, endStr)

	var b strings.Builder
	fmt.Fprintf(&b, "Weekly Bot Performance\nPeriod: %s – %s UTC\n\n", startStr, endStr)

	var totalTrades int
	var totalPL float64
	activeBots := 0

	for _, sec := range sections {
		fmt.Fprintf(&b, "── %s ──\n", BotDisplayName(sec.BotID))
		if sec.Err != nil {
			fmt.Fprintf(&b, "  (no data — %v)\n\n", sec.Err)
			continue
		}
		pm := sec.Period
		if pm.TradeCount == 0 {
			b.WriteString("  No closed trades this week.\n")
			if sec.AllTime.TradeCount > 0 {
				fmt.Fprintf(&b, "  All-time: %d trades, net P&L %s\n", sec.AllTime.TradeCount, formatMoney(sec.AllTime.TotalNetPL))
			}
			b.WriteString("\n")
			continue
		}

		activeBots++
		totalTrades += pm.TradeCount
		totalPL += pm.TotalNetPL

		fmt.Fprintf(&b, "  Trades: %d\n", pm.TradeCount)
		fmt.Fprintf(&b, "  Win rate: %s\n", formatWinRate(pm.TradeCount, pm.WinCount, pm.LossCount, pm.WinRate))
		fmt.Fprintf(&b, "  Net P&L: %s\n", formatMoney(pm.TotalNetPL))
		fmt.Fprintf(&b, "  Max drawdown: %s\n", formatDrawdown(pm.MaxDrawdown))
		fmt.Fprintf(&b, "  Realized R:R: %s\n", formatRR(pm.RealizedRR, pm.TradeCount))
		fmt.Fprintf(&b, "  Avg win / avg loss: %s / %s\n", formatMoney(pm.AvgWin), formatMoney(pm.AvgLoss))
		fmt.Fprintf(&b, "  Best / worst trade: %s / %s\n", formatMoney(pm.BestNetPL), formatMoney(pm.WorstNetPL))
		fmt.Fprintf(&b, "  Fees (swap): %s\n", formatFees(pm.TotalFees))
		if sec.AllTime.TradeCount > 0 {
			fmt.Fprintf(&b, "  All-time net P&L: %s (%d trades)\n", formatMoney(sec.AllTime.TotalNetPL), sec.AllTime.TradeCount)
		}
		b.WriteString("\n")
	}

	b.WriteString("── Combined ──\n")
	if activeBots == 0 {
		b.WriteString("  No closed trades across any bot this week.\n")
	} else {
		fmt.Fprintf(&b, "  Active bots: %d\n", activeBots)
		fmt.Fprintf(&b, "  Total trades: %d\n", totalTrades)
		fmt.Fprintf(&b, "  Combined net P&L: %s\n", formatMoney(totalPL))
	}
	return subject, b.String()
}

func formatWinRate(trades, wins, losses int, rate float64) string {
	if trades == 0 {
		return "n/a (0 trades)"
	}
	return fmt.Sprintf("%.1f%% (%dW / %dL)", rate*100, wins, losses)
}

func formatMoney(v float64) string {
	sign := "+"
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s$%.2f", sign, v)
}

func formatFees(v float64) string {
	if v < 0 {
		v = -v
	}
	return fmt.Sprintf("$%.2f", v)
}

func formatDrawdown(v float64) string {
	if v == 0 {
		return "$0.00"
	}
	return fmt.Sprintf("$%.2f", v)
}

func formatRR(rr float64, trades int) string {
	if trades == 0 || rr == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", rr)
}
