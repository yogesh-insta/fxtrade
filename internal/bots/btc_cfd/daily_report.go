package btc_cfd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

// ReportDayUTC returns the UTC calendar day covered by a daily summary sent at now.
// Trades are counted for the previous full UTC day (00:00–23:59:59 UTC), so a job
// scheduled at notifications.daily_summary_utc (default 12:00 UTC) reports yesterday.
func ReportDayUTC(now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return today.Add(-24 * time.Hour)
}

// OpenPosition describes a live OANDA trade for the daily email.
type OpenPosition struct {
	Instrument   string
	Direction    string
	Units        int64
	UnrealizedPL float64
}

// DailyEmail formats the plain-text daily summary body and subject.
func DailyEmail(reportDate time.Time, day, all sqlite.DayMetrics, open *OpenPosition) (subject, body string) {
	dateStr := reportDate.UTC().Format("2006-01-02")
	subject = fmt.Sprintf("fxtrade: BTC daily summary %s", dateStr)

	var b strings.Builder
	fmt.Fprintf(&b, "BTC CFD Daily Summary\nDate: %s UTC\n\n", dateStr)

	fmt.Fprintf(&b, "Trades: %d\n", day.TradeCount)
	if len(day.Trades) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, t := range day.Trades {
			dir := strings.ToUpper(strings.TrimSpace(t.Direction))
			if dir == "" {
				dir = "?"
			}
			fmt.Fprintf(&b, "  • %s  net P&L %s\n", dir, formatMoney(t.NetPL))
		}
	}

	b.WriteString("\n")
	fmt.Fprintf(&b, "Win rate (day): %s\n", formatWinRate(day))
	fmt.Fprintf(&b, "Win rate (all-time): %s\n", formatWinRate(all))

	b.WriteString("\n")
	fmt.Fprintf(&b, "Net P&L (day): %s\n", formatMoney(day.TotalNetPL))
	fmt.Fprintf(&b, "Net P&L (all-time): %s\n", formatMoney(all.TotalNetPL))

	b.WriteString("\n")
	fmt.Fprintf(&b, "Fees paid (day): %s\n", formatFees(day.TotalFees))
	fmt.Fprintf(&b, "Fees paid (all-time): %s\n", formatFees(all.TotalFees))

	b.WriteString("\n")
	if open == nil {
		b.WriteString("Open position: none\n")
	} else {
		fmt.Fprintf(&b, "Open position: %s %d %s  unrealized %s\n",
			open.Direction, open.Units, open.Instrument, formatMoney(open.UnrealizedPL))
	}
	return subject, b.String()
}

func formatWinRate(m sqlite.DayMetrics) string {
	if m.TradeCount == 0 {
		return "n/a (0 trades)"
	}
	return fmt.Sprintf("%.1f%% (%dW / %dL)", m.WinRate*100, m.WinCount, m.LossCount)
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

// FetchOpenPosition returns the open trade for instrument, if any.
func FetchOpenPosition(ctx context.Context, client *oanda.Client, instrument string) (*OpenPosition, error) {
	resp, err := client.OpenTrades(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range resp.Trades {
		if t.Instrument != instrument {
			continue
		}
		units, err := strconv.ParseInt(t.CurrentUnits, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse units: %w", err)
		}
		if units == 0 {
			continue
		}
		dir := "LONG"
		absUnits := units
		if units < 0 {
			dir = "SHORT"
			absUnits = -units
		}
		upl, _ := oanda.ParsePrice(t.UnrealizedPL)
		return &OpenPosition{
			Instrument:   t.Instrument,
			Direction:    dir,
			Units:        absUnits,
			UnrealizedPL: upl,
		}, nil
	}
	return nil, nil
}
