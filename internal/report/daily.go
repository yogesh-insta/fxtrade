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

// AccountDailyContext is live OANDA account figures for the daily email header.
type AccountDailyContext struct {
	NAV          float64
	Currency     string
	Baseline     float64
	TotalPL      float64 // NAV - baseline (all-time)
	DailyPL      float64 // NAV - prior snapshot
	DailyPLKnown bool
	PriorNAV     float64
	PriorDate    string
	UnrealizedPL float64
	Reconcile    ReconcileSummary
}

// DailyEmail formats the combined daily performance subject and body.
// analysis is optional text from FormatDailyAnalysis (may be empty).
func DailyEmail(reportDate time.Time, sections []BotDailySection, acct AccountDailyContext, analysis string) (subject, body string) {
	dateStr := reportDate.UTC().Format("2006-01-02")
	subject = fmt.Sprintf("fxtrade: daily bot summary %s", dateStr)

	var b strings.Builder
	fmt.Fprintf(&b, "Daily Bot Performance\nDate: %s UTC\n\n", dateStr)

	var totalTrades int
	var totalPL float64
	activeBots := 0
	for _, sec := range sections {
		if sec.Err != nil || sec.Day.TradeCount == 0 {
			continue
		}
		activeBots++
		totalTrades += sec.Day.TradeCount
		totalPL += sec.Day.TotalNetPL
	}

	if acct.NAV > 0 && acct.Baseline > 0 {
		b.WriteString("── Account (OANDA) ──\n")
		fmt.Fprintf(&b, "  NAV: %s %s\n", formatMoneySign(acct.NAV), acct.Currency)
		if acct.DailyPLKnown {
			pct := 0.0
			if acct.PriorNAV > 0 {
				pct = acct.DailyPL / acct.PriorNAV * 100
			}
			fmt.Fprintf(&b, "  Today (account): %s (%.2f%%) vs %s NAV %s\n",
				formatMoney(acct.DailyPL), pct, acct.PriorDate, formatMoneySign(acct.PriorNAV))
		} else {
			b.WriteString("  Today (account): n/a — first NAV snapshot; tomorrow will show daily change\n")
		}
		fmt.Fprintf(&b, "  All-time vs baseline: %s (%.2f%%) [baseline %s %s]\n",
			formatMoney(acct.TotalPL), acct.TotalPL/acct.Baseline*100,
			formatMoneySign(acct.Baseline), acct.Currency)
		if math.Abs(acct.UnrealizedPL) > 0.01 {
			fmt.Fprintf(&b, "  Unrealized (open positions): %s\n", formatMoney(acct.UnrealizedPL))
		}
		fmt.Fprintf(&b, "  Closed bots today: %s\n", formatMoney(totalPL))
		if acct.Reconcile.AttemptedTotal() > 0 {
			fmt.Fprintf(&b, "  Reconcile: backfilled %d / %d from OANDA",
				acct.Reconcile.TotalReconciled, acct.Reconcile.AttemptedTotal())
			if missing := acct.Reconcile.StillMissingTotal(); missing > 0 {
				fmt.Fprintf(&b, "; %d still missing (see Analysis)", missing)
			}
			b.WriteByte('\n')
		}
		var botAllTime float64
		for _, sec := range sections {
			if sec.Err == nil {
				botAllTime += sec.AllTime.TotalNetPL
			}
		}
		gap := acct.TotalPL - botAllTime
		fmt.Fprintf(&b, "  Bots tracked (all-time net): %s | gap vs account: %s",
			formatMoney(botAllTime), formatMoney(gap))
		if math.Abs(gap) > 1.0 {
			b.WriteString(" (fees, manual trades, open P&L, unreconciled rows)")
		}
		b.WriteString("\n\n")
	}

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

		fmt.Fprintf(&b, "  Trades today: %d\n", day.TradeCount)
		fmt.Fprintf(&b, "  Win rate: %s\n", formatWinRate(day.TradeCount, day.WinCount, day.LossCount, day.WinRate))
		fmt.Fprintf(&b, "  Net P&L today: %s\n", formatMoney(day.TotalNetPL))
		for _, line := range formatDayTradeLines(day.Trades) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if sec.AllTime.TradeCount > 0 {
			fmt.Fprintf(&b, "  All-time net P&L: %s (%d trades)\n", formatMoney(sec.AllTime.TotalNetPL), sec.AllTime.TradeCount)
		}
		b.WriteString("\n")
	}

	b.WriteString("── Combined (closed bots today) ──\n")
	if activeBots == 0 {
		b.WriteString("  No closed trades across any bot today.\n")
	} else {
		fmt.Fprintf(&b, "  Active bots: %d\n", activeBots)
		fmt.Fprintf(&b, "  Total trades: %d\n", totalTrades)
		fmt.Fprintf(&b, "  Combined net P&L today: %s\n", formatMoney(totalPL))
	}
	if strings.TrimSpace(analysis) != "" {
		b.WriteString("\n")
		b.WriteString(analysis)
	}
	return subject, b.String()
}

// FormatAccountPNL formats account NAV vs configured initial capital (legacy one-liner).
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
