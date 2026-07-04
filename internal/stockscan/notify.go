package stockscan

import (
	"context"
	"fmt"
	"strings"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// Pick is the final trade recommendation.
type Pick struct {
	Candidate Candidate
	Entry     float64
	StopLoss  float64
	Target    float64
}

func BuildPick(c Candidate, cfg config.StockScanConfig) Pick {
	entry := RoundINR(c.Close)
	sl, tgt := Levels(c.Close, cfg.StopLossPct, cfg.TargetPct)
	return Pick{
		Candidate: c,
		Entry:     entry,
		StopLoss:  RoundINR(sl),
		Target:    RoundINR(tgt),
	}
}

// FormatAlertEmail renders the Zerodha-style alert body.
func FormatAlertEmail(p Pick) string {
	symbol := strings.ToUpper(p.Candidate.Symbol)
	var b strings.Builder
	fmt.Fprintf(&b, "Instrument: %s (Cash Equity Stock)\n", symbol)
	b.WriteString("Action: BUY\n")
	fmt.Fprintf(&b, "Limit Price: ₹%.2f\n", p.Entry)
	fmt.Fprintf(&b, "Stop Loss: ₹%.2f (Strict 1.5%% protection)\n", p.StopLoss)
	fmt.Fprintf(&b, "Target: ₹%.2f (Strict 3%% profit goal)\n", p.Target)
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
func SendAlert(n notify.Notifier, ctx context.Context, prefix string, p Pick) {
	subject := FormatAlertSubject(prefix, p)
	body := FormatAlertEmail(p)
	n.Send(ctx, subject, body)
}
