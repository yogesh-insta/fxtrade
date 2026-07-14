package stockscan

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// Pick is the final trade recommendation.
type Pick struct {
	Candidate    Candidate
	Entry        float64
	StopLoss     float64
	Target       float64
	StopLossPct  float64
	TargetPct    float64
	Reasons      []string
}

func BuildPick(c Candidate, cfg config.StockScanConfig, reasons []string) Pick {
	entry := RoundINR(c.Close)
	sl, tgt := Levels(c.Close, cfg.StopLossPct, cfg.TargetPct)
	return Pick{
		Candidate:   c,
		Entry:       entry,
		StopLoss:    RoundINR(sl),
		Target:      RoundINR(tgt),
		StopLossPct: cfg.StopLossPct,
		TargetPct:   cfg.TargetPct,
		Reasons:     reasons,
	}
}

func formatPctLabel(pct float64) string {
	v := pct * 100
	if math.Abs(v-math.Round(v)) < 1e-9 {
		return fmt.Sprintf("%.0f%%", v)
	}
	return fmt.Sprintf("%.1f%%", v)
}

// FormatAlertEmail renders the Zerodha-style alert body with shortlist context.
func FormatAlertEmail(p Pick, contenders []Contender, totalPassed int) string {
	symbol := strings.ToUpper(p.Candidate.Symbol)
	slPct, tgtPct := p.StopLossPct, p.TargetPct
	if slPct <= 0 {
		slPct = 0.02
	}
	if tgtPct <= 0 {
		tgtPct = 0.03
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Instrument: %s (Cash Equity Stock)\n", symbol)
	b.WriteString("Action: BUY\n")
	fmt.Fprintf(&b, "Limit Price: ₹%.2f\n", p.Entry)
	fmt.Fprintf(&b, "Stop Loss: ₹%.2f (Strict %s protection)\n", p.StopLoss, formatPctLabel(slPct))
	fmt.Fprintf(&b, "Target: ₹%.2f (Strict %s profit goal)\n", p.Target, formatPctLabel(tgtPct))
	b.WriteString("\n")

	if len(p.Reasons) > 0 {
		b.WriteString("Why this pick:\n")
		for _, r := range p.Reasons {
			fmt.Fprintf(&b, "  • %s\n", r)
		}
		b.WriteString("\n")
	}

	writeContendersSection(&b, contenders, totalPassed)
	b.WriteString("\n")

	b.WriteString("--- Manual execution required ---\n")
	b.WriteString("Place a GTT OCO order in Zerodha Kite:\n")
	b.WriteString("  • Trigger: Limit BUY at Limit Price\n")
	b.WriteString("  • OCO leg 1: Stop Loss (SL-M or SL) at Stop Loss price\n")
	b.WriteString("  • OCO leg 2: Target (Limit SELL) at Target price\n")
	b.WriteString("\n")
	b.WriteString("NiftyPulse does NOT place orders automatically. Verify liquidity, circuit limits, and corporate actions before trading.\n")
	return b.String()
}

func writeContendersSection(b *strings.Builder, contenders []Contender, totalPassed int) {
	if len(contenders) == 0 {
		return
	}
	if totalPassed < 5 {
		fmt.Fprintf(b, "Top contenders (%d passed filters):\n", totalPassed)
	} else {
		b.WriteString("Top 5 contenders:\n")
	}
	for _, c := range contenders {
		sym := strings.ToUpper(c.Candidate.Symbol)
		marker := ""
		if c.Selected {
			marker = " ★ selected"
		}
		fmt.Fprintf(b, "  %d. %s — %s%s\n", c.Rank, sym, c.OneLiner, marker)
	}
	if totalPassed < 5 {
		fmt.Fprintf(b, "Note: Only %d symbol(s) passed today's SMA/RSI filters.\n", totalPassed)
	}
}

// FormatAlertSubject builds the NiftyPulse email subject line.
func FormatAlertSubject(prefix string, p Pick) string {
	if prefix == "" {
		prefix = "[NiftyPulse]"
	}
	symbol := strings.ToUpper(p.Candidate.Symbol)
	return fmt.Sprintf("%s BUY %s · Limit ₹%.2f · SL ₹%.2f · TGT ₹%.2f",
		prefix, symbol, p.Entry, p.StopLoss, p.Target)
}

// SendAlert emails the pick using the shared notify package.
func SendAlert(n notify.Notifier, ctx context.Context, prefix string, p Pick, contenders []Contender, totalPassed int) {
	subject := FormatAlertSubject(prefix, p)
	body := FormatAlertEmail(p, contenders, totalPassed)
	n.Send(ctx, subject, body)
}
