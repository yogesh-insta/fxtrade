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
// analysis is optional text from FormatDailyTweaks (may be empty).
func DailyEmail(reportDate time.Time, sections []BotDailySection, acct AccountDailyContext, analysis string) (subject, body string) {
	dateStr := reportDate.UTC().Format("2006-01-02")
	subject = dailyEmailSubject(dateStr, acct)

	var b strings.Builder
	fmt.Fprintf(&b, "Daily Bot Performance\nDate: %s UTC\n\n", dateStr)

	dayUnique := uniqueTradesAcrossBots(sections, true)

	writePLSummary(&b, sections, acct, dayUnique)

	if dayUnique.Count > 0 {
		b.WriteString("\n── Today's trades ──\n")
		for _, sec := range sections {
			if sec.Err != nil || sec.Day.TradeCount == 0 {
				continue
			}
			writeBotDayDetail(&b, sec)
		}
	} else {
		b.WriteString("\nNo closed trades across any bot today.\n")
	}

	if foot := accountFootnotes(acct, sections); foot != "" {
		b.WriteString("\n── Notes ──\n")
		b.WriteString(foot)
	}

	if strings.TrimSpace(analysis) != "" {
		b.WriteString("\n")
		b.WriteString(analysis)
	}
	return subject, b.String()
}

func dailyEmailSubject(dateStr string, acct AccountDailyContext) string {
	label := "FLAT"
	if acct.DailyPLKnown {
		label = plKind(acct.DailyPL)
	}
	return fmt.Sprintf("fxtrade: daily summary %s | account %s", dateStr, label)
}

func writePLSummary(b *strings.Builder, sections []BotDailySection, acct AccountDailyContext, dayUnique uniqueTradeTotals) {
	b.WriteString("═══ P&L SUMMARY ═══\n\n")
	b.WriteString("                      Today          All-time\n")

	if acct.NAV > 0 && acct.Baseline > 0 {
		todayCol := "n/a"
		if acct.DailyPLKnown {
			pct := 0.0
			if acct.PriorNAV > 0 {
				pct = acct.DailyPL / acct.PriorNAV * 100
			}
			todayCol = formatPLWithPct(acct.DailyPL, pct)
		}
		allTimeCol := formatPLWithPct(acct.TotalPL, acct.TotalPL/acct.Baseline*100)
		fmt.Fprintf(b, "Account (OANDA)       %-22s %s\n", todayCol, allTimeCol)
		fmt.Fprintf(b, "NAV %s %s\n", formatMoneySign(acct.NAV), acct.Currency)
	}

	for _, sec := range sections {
		name := BotDisplayName(sec.BotID)
		if sec.Err != nil {
			fmt.Fprintf(b, "%-22s (no data)\n", name)
			continue
		}
		todayCol := formatPL(sec.Day.TotalNetPL)
		allTimeCol := formatPL(sec.AllTime.TotalNetPL)
		fmt.Fprintf(b, "%-22s %-14s %s", name, todayCol, allTimeCol)
		if sec.AllTime.TradeCount > 0 {
			fmt.Fprintf(b, "  (%d trades)", sec.AllTime.TradeCount)
		}
		b.WriteByte('\n')
	}

	fmt.Fprintf(b, "\nBots combined today:  %s", formatPL(dayUnique.NetPL))
	if dayUnique.Count > 0 {
		fmt.Fprintf(b, "  (%d unique trades)", dayUnique.Count)
		if dayUnique.Duplicates > 0 {
			fmt.Fprintf(b, "; %d duplicate trade_id(s) excluded", dayUnique.Duplicates)
		}
	}
	b.WriteByte('\n')
}

func writeBotDayDetail(b *strings.Builder, sec BotDailySection) {
	day := sec.Day
	fmt.Fprintf(b, "\n%s  %s  (%d trades, %s)\n",
		BotDisplayName(sec.BotID),
		formatPL(day.TotalNetPL),
		day.TradeCount,
		formatWinRate(day.TradeCount, day.WinCount, day.LossCount, day.WinRate),
	)
	for _, line := range formatDayTradeLines(day.Trades) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func accountFootnotes(acct AccountDailyContext, sections []BotDailySection) string {
	var notes []string

	if acct.NAV > 0 && acct.Baseline > 0 {
		if acct.DailyPLKnown {
			notes = append(notes, fmt.Sprintf("Prior NAV (%s): %s", acct.PriorDate, formatMoneySign(acct.PriorNAV)))
		}
		notes = append(notes, fmt.Sprintf("Baseline: %s %s", formatMoneySign(acct.Baseline), acct.Currency))
		if math.Abs(acct.UnrealizedPL) > 0.01 {
			notes = append(notes, fmt.Sprintf("Unrealized open P&L: %s", formatPL(acct.UnrealizedPL)))
		}

		allUnique := uniqueTradesAcrossBots(sections, false)
		gap := acct.TotalPL - allUnique.NetPL
		if math.Abs(gap) > 1.0 {
			note := fmt.Sprintf("Bots tracked all-time (unique): %s | gap vs account: %s (fees, manual trades, open P&L)",
				formatPL(allUnique.NetPL), formatPL(gap))
			if allUnique.Duplicates > 0 {
				note += fmt.Sprintf("; %d duplicate trade_id(s) excluded", allUnique.Duplicates)
			}
			notes = append(notes, note)
		}
	}

	if acct.Reconcile.AttemptedTotal() > 0 {
		line := fmt.Sprintf("Reconciled %d / %d trades from OANDA before this report",
			acct.Reconcile.TotalReconciled, acct.Reconcile.AttemptedTotal())
		if missing := acct.Reconcile.StillMissingTotal(); missing > 0 {
			line += fmt.Sprintf("; %d still missing P/L", missing)
		}
		notes = append(notes, line)
	}

	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	for _, n := range notes {
		fmt.Fprintf(&b, "  • %s\n", n)
	}
	return b.String()
}

// uniqueTradeTotals is P/L after collapsing the same OANDA trade_id across bots.
type uniqueTradeTotals struct {
	Count      int
	NetPL      float64
	Duplicates int
}

// uniqueTradesAcrossBots collapses shared OANDA trade_ids so Combined / account gap
// are not double-counted when the same close is stored in multiple bot DBs.
// Prefer the first TradingBotIDs() order (universe_scanner, then fx_sentiment, then btc_cfd).
func uniqueTradesAcrossBots(sections []BotDailySection, dayOnly bool) uniqueTradeTotals {
	seen := make(map[string]struct{})
	var out uniqueTradeTotals
	anon := 0
	for _, sec := range sections {
		if sec.Err != nil {
			continue
		}
		trades := sec.AllTime.Trades
		if dayOnly {
			trades = sec.Day.Trades
		}
		for _, t := range trades {
			key := strings.TrimSpace(t.TradeID)
			if key == "" {
				anon++
				key = fmt.Sprintf("anon:%s:%d", sec.BotID, anon)
			}
			if _, ok := seen[key]; ok {
				out.Duplicates++
				continue
			}
			seen[key] = struct{}{}
			out.Count++
			out.NetPL += t.NetPL
		}
	}
	return out
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
		formatPL(total), pct)
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
		inst := strings.TrimSpace(t.Instrument)
		if inst == "" {
			inst = "?"
		}
		exit := exitReason(t.CorrelationID)
		closed := t.ClosedAt.UTC().Format("15:04")
		lines = append(lines, fmt.Sprintf("    %s  %s %s  %s UTC  %s",
			formatPL(t.NetPL), inst, dir, closed, exit))
	}
	if zeroCount == 1 {
		lines = append(lines, "    FLAT $0.00  (1 unreconciled trade)")
	} else if zeroCount > 1 {
		lines = append(lines, fmt.Sprintf("    FLAT $0.00  (%d unreconciled trades)", zeroCount))
	}
	return lines
}
