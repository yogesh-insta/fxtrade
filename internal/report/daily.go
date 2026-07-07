package report

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/store/sqlite"
)

// BotDailySection is one bot's performance for the daily email.
type BotDailySection struct {
	BotID   string
	DBPath  string
	Day     sqlite.DayMetrics
	AllTime sqlite.DayMetrics
	Err     error
}

// ReportDayUTC returns the UTC calendar day covered by a daily summary sent at now.
// Reports the previous full UTC day by default; at 20:30 UTC same-day mode reports today.
func ReportDayUTC(now time.Time, sameDay bool) time.Time {
	y, m, d := now.UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if sameDay {
		return today
	}
	return today.Add(-24 * time.Hour)
}

// DailyEmail formats the combined daily performance subject and body.
// accountTotalPL is cumulative account P/L vs baseline (pass 0 when unknown).
// analysis is optional text from FormatDailyAnalysis (may be empty).
func DailyEmail(reportDate time.Time, sections []BotDailySection, accountPNL string, accountTotalPL float64, analysis string) (subject, body string) {
	dateStr := reportDate.UTC().Format("2006-01-02")
	subject = fmt.Sprintf("fxtrade: daily bot summary %s", dateStr)

	var b strings.Builder
	fmt.Fprintf(&b, "Daily Bot Performance\nDate: %s UTC\n\n", dateStr)
	if accountPNL != "" {
		fmt.Fprintf(&b, "%s\n", accountPNL)
		var botAllTime float64
		for _, sec := range sections {
			if sec.Err == nil {
				botAllTime += sec.AllTime.TotalNetPL
			}
		}
		gap := accountTotalPL - botAllTime
		fmt.Fprintf(&b, "Bots tracked (all-time net): %s | gap vs account: %s",
			formatMoney(botAllTime), formatMoney(gap))
		if math.Abs(gap) > 1.0 {
			b.WriteString(" (fees, manual trades, open positions, or unreconciled rows)")
		}
		b.WriteString("\n\n")
	}

	var totalTrades int
	var totalPL float64
	activeBots := 0

	for _, sec := range sections {
		fmt.Fprintf(&b, "── %s ──\n", BotDisplayName(sec.BotID))
		if sec.Err != nil {
			fmt.Fprintf(&b, "  (no data — %v)\n\n", sec.Err)
			continue
		}
		day := sec.Day
		if day.TradeCount == 0 {
			b.WriteString("  No closed trades today.\n")
			if sec.AllTime.TradeCount > 0 {
				fmt.Fprintf(&b, "  All-time: %d trades, net P&L %s\n", sec.AllTime.TradeCount, formatMoney(sec.AllTime.TotalNetPL))
			}
			b.WriteString("\n")
			continue
		}

		activeBots++
		totalTrades += day.TradeCount
		totalPL += day.TotalNetPL

		fmt.Fprintf(&b, "  Trades: %d\n", day.TradeCount)
		fmt.Fprintf(&b, "  Win rate: %s\n", formatWinRate(day.TradeCount, day.WinCount, day.LossCount, day.WinRate))
		fmt.Fprintf(&b, "  Net P&L: %s\n", formatMoney(day.TotalNetPL))
		for _, line := range formatDayTradeLines(day.Trades) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if sec.AllTime.TradeCount > 0 {
			fmt.Fprintf(&b, "  All-time net P&L: %s (%d trades)\n", formatMoney(sec.AllTime.TotalNetPL), sec.AllTime.TradeCount)
		}
		b.WriteString("\n")
	}

	b.WriteString("── Combined ──\n")
	if activeBots == 0 {
		b.WriteString("  No closed trades across any bot today.\n")
	} else {
		fmt.Fprintf(&b, "  Active bots: %d\n", activeBots)
		fmt.Fprintf(&b, "  Total trades: %d\n", totalTrades)
		fmt.Fprintf(&b, "  Combined net P&L: %s\n", formatMoney(totalPL))
	}
	if strings.TrimSpace(analysis) != "" {
		b.WriteString("\n")
		b.WriteString(analysis)
	}
	return subject, b.String()
}

// FormatAccountPNL formats account NAV vs configured initial capital.
func FormatAccountPNL(nav, baseline float64, currency string) string {
	if baseline <= 0 {
		return ""
	}
	if currency == "" {
		currency = "AUD"
	}
	total := nav - baseline
	pct := total / baseline * 100
	return fmt.Sprintf("Account NAV: %s %s | vs baseline %s %s | total P&L %s (%.2f%%)",
		formatMoneySign(nav), currency,
		formatMoneySign(baseline), currency,
		formatMoney(total), pct)
}

func formatMoneySign(v float64) string {
	if v < 0 {
		return fmt.Sprintf("-$%.2f", -v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func absMoney(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func formatDayTradeLines(trades []sqlite.TradeBrief) []string {
	var lines []string
	zeroCount := 0
	for _, t := range trades {
		if math.Abs(t.NetPL) < 0.01 {
			zeroCount++
			continue
		}
		dir := strings.ToUpper(strings.TrimSpace(t.Direction))
		if dir == "" {
			dir = "?"
		}
		lines = append(lines, fmt.Sprintf("    • %s  %s", dir, formatMoney(t.NetPL)))
	}
	if zeroCount == 1 {
		lines = append(lines, "    • 1 trade @ $0.00 (unreconciled)")
	} else if zeroCount > 1 {
		lines = append(lines, fmt.Sprintf("    • %d trades @ $0.00 (unreconciled)", zeroCount))
	}
	return lines
}
