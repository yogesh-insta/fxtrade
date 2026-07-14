package stockscan

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// Universe labels used in dual-pick alerts.
const (
	UniverseNifty200     = "Nifty 200"
	UniverseNifty500Rest = "Nifty 500 (ex-200)"
)

// Pick is the final trade recommendation.
type Pick struct {
	Candidate   Candidate
	Universe    string
	Entry       float64
	StopLoss    float64
	Target      float64
	StopLossPct float64
	TargetPct   float64
	Reasons     []string
}

func BuildPick(c Candidate, cfg config.StockScanConfig, reasons []string) Pick {
	return BuildPickUniverse(c, cfg, reasons, "")
}

func BuildPickUniverse(c Candidate, cfg config.StockScanConfig, reasons []string, universe string) Pick {
	entry := RoundINR(c.Close)
	sl, tgt := Levels(c.Close, cfg.StopLossPct, cfg.TargetPct)
	return Pick{
		Candidate:   c,
		Universe:    universe,
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
	return FormatPicksEmail([]Pick{p}, map[string][]Contender{
		p.Candidate.Symbol: contenders,
	}, map[string]int{
		p.Candidate.Symbol: totalPassed,
	})
}

// FormatPicksEmail renders one or more universe picks in a single alert body.
func FormatPicksEmail(picks []Pick, contendersBySymbol map[string][]Contender, passedBySymbol map[string]int) string {
	var b strings.Builder
	if len(picks) == 0 {
		return ""
	}

	fmt.Fprintf(&b, "NiftyPulse daily picks: %d suggestion(s)\n\n", len(picks))
	for i, p := range picks {
		if i > 0 {
			b.WriteString("\n--------------------\n\n")
		}
		writePickSection(&b, p, i+1, len(picks))
		sym := strings.ToUpper(p.Candidate.Symbol)
		writeContendersSection(&b, contendersBySymbol[sym], passedBySymbol[sym])
	}

	b.WriteString("\n")
	b.WriteString("--- Manual execution required ---\n")
	b.WriteString("Place a GTT OCO order in Zerodha Kite for each pick you take:\n")
	b.WriteString("  • Trigger: Limit BUY at Limit Price\n")
	b.WriteString("  • OCO leg 1: Stop Loss (SL-M or SL) at Stop Loss price\n")
	b.WriteString("  • OCO leg 2: Target (Limit SELL) at Target price\n")
	b.WriteString("\n")
	b.WriteString("NiftyPulse does NOT place orders automatically. Verify liquidity, circuit limits, and corporate actions before trading.\n")
	return b.String()
}

func writePickSection(b *strings.Builder, p Pick, idx, total int) {
	symbol := strings.ToUpper(p.Candidate.Symbol)
	slPct, tgtPct := p.StopLossPct, p.TargetPct
	if slPct <= 0 {
		slPct = 0.02
	}
	if tgtPct <= 0 {
		tgtPct = 0.03
	}
	if total > 1 {
		label := p.Universe
		if label == "" {
			label = fmt.Sprintf("Pick %d", idx)
		}
		fmt.Fprintf(b, "=== %s ===\n", label)
	}
	if name := strings.TrimSpace(p.Candidate.Name); name != "" {
		fmt.Fprintf(b, "Instrument: %s — %s (Cash Equity Stock)\n", symbol, name)
	} else {
		fmt.Fprintf(b, "Instrument: %s (Cash Equity Stock)\n", symbol)
	}
	b.WriteString("Action: BUY\n")
	fmt.Fprintf(b, "Limit Price: ₹%.2f\n", p.Entry)
	fmt.Fprintf(b, "Stop Loss: ₹%.2f (Strict %s protection)\n", p.StopLoss, formatPctLabel(slPct))
	fmt.Fprintf(b, "Target: ₹%.2f (Strict %s profit goal)\n", p.Target, formatPctLabel(tgtPct))
	b.WriteString("\n")

	if len(p.Reasons) > 0 {
		b.WriteString("Why this pick:\n")
		for _, r := range p.Reasons {
			fmt.Fprintf(b, "  • %s\n", r)
		}
		b.WriteString("\n")
	}
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
		marker := ""
		if c.Selected {
			marker = " ★ selected"
		}
		fmt.Fprintf(b, "  %d. %s — %s%s\n", c.Rank, c.Candidate.DisplayName(), c.OneLiner, marker)
	}
	if totalPassed < 5 {
		fmt.Fprintf(b, "Note: Only %d symbol(s) passed today's SMA/RSI filters.\n", totalPassed)
	}
}

// FormatAlertSubject builds the NiftyPulse email subject line for a single pick.
func FormatAlertSubject(prefix string, p Pick) string {
	return FormatPicksSubject(prefix, []Pick{p})
}

// FormatPicksSubject builds the subject for one or more picks.
func FormatPicksSubject(prefix string, picks []Pick) string {
	if prefix == "" {
		prefix = "[NiftyPulse]"
	}
	if len(picks) == 0 {
		return prefix + " no picks"
	}
	if len(picks) == 1 {
		p := picks[0]
		return fmt.Sprintf("%s BUY %s · Limit ₹%.2f · SL ₹%.2f · TGT ₹%.2f",
			prefix, p.Candidate.DisplayName(), p.Entry, p.StopLoss, p.Target)
	}
	parts := make([]string, 0, len(picks))
	for _, p := range picks {
		label := shortUniverse(p.Universe)
		parts = append(parts, fmt.Sprintf("%s %s", label, p.Candidate.DisplayName()))
	}
	return fmt.Sprintf("%s BUY %s", prefix, strings.Join(parts, " · "))
}

func shortUniverse(u string) string {
	switch u {
	case UniverseNifty200:
		return "N200"
	case UniverseNifty500Rest:
		return "N500"
	case "":
		return "PICK"
	default:
		return u
	}
}

// SendAlert emails a single pick using the shared notify package.
func SendAlert(n notify.Notifier, ctx context.Context, prefix string, p Pick, contenders []Contender, totalPassed int) {
	SendPicksAlert(n, ctx, prefix, []Pick{p}, map[string][]Contender{
		p.Candidate.Symbol: contenders,
	}, map[string]int{
		p.Candidate.Symbol: totalPassed,
	})
}

// SendPicksAlert emails one or more universe picks.
func SendPicksAlert(
	n notify.Notifier,
	ctx context.Context,
	prefix string,
	picks []Pick,
	contendersBySymbol map[string][]Contender,
	passedBySymbol map[string]int,
) {
	subject := FormatPicksSubject(prefix, picks)
	body := FormatPicksEmail(picks, contendersBySymbol, passedBySymbol)
	n.Send(ctx, subject, body)
}
