package scanner

import (
	"context"
	"fmt"
	"strings"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

type Notifier struct {
	cfg      config.NotificationsConfig
	email    config.EmailConfig
	inner    notify.Notifier
	envLabel string
	dryRun   bool
}

func NewNotifier(cfg *config.Config, n notify.Notifier, dryRun bool) *Notifier {
	env := strings.ToUpper(cfg.OANDA.Environment)
	if env == "" {
		env = "PRACTICE"
	}
	return &Notifier{
		cfg:      cfg.Notifications,
		email:    cfg.Email,
		inner:    n,
		envLabel: env,
		dryRun:   dryRun,
	}
}

func (n *Notifier) prefixSubject(subject string) string {
	p := strings.TrimSpace(n.cfg.Prefix)
	if p == "" {
		return subject
	}
	return p + " " + subject
}

func (n *Notifier) send(ctx context.Context, subject, body string) {
	if !n.cfg.Enabled {
		return
	}
	n.inner.Send(ctx, n.prefixSubject(subject), body)
}

func (n *Notifier) TradeEntry(ctx context.Context, setup Setup, runnersUp []Setup, balance float64, units int64, stop, tp float64) {
	if !n.cfg.OnTradeEntry {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Instrument: %s\nDirection: %s\nScore: %.2f\n", setup.Instrument, setup.BreakoutDirection, setup.Score)
	fmt.Fprintf(&b, "Opening range: %.5f - %.5f (%.1f pips)\n", setup.Range.Low, setup.Range.High, setup.Range.RangePips)
	fmt.Fprintf(&b, "Spread: %.2f pips | Range/spread: %.1f\n", setup.SpreadPips, setup.RangeSpreadRatio)
	fmt.Fprintf(&b, "H1 trend bias: %s\n", trendLabel(setup.TrendBias))
	fmt.Fprintf(&b, "Balance: $%.2f | Units: %d | SL: %.5f | TP: %.5f\n", balance, units, stop, tp)
	fmt.Fprintf(&b, "\nReasoning: top-ranked opening-range breakout above min score; 2× M15 range established after session open.\n")
	if n.cfg.EntryIncludeRunnersUp > 0 && len(runnersUp) > 0 {
		b.WriteString("\nRunners-up:\n")
		limit := n.cfg.EntryIncludeRunnersUp
		if limit > len(runnersUp) {
			limit = len(runnersUp)
		}
		for i := 0; i < limit; i++ {
			r := runnersUp[i]
			fmt.Fprintf(&b, "  %d) %s score=%.2f range=%.1f pips spread=%.2f\n", i+2, r.Instrument, r.Score, r.Range.RangePips, r.SpreadPips)
		}
	}
	subject := fmt.Sprintf("ENTRY %s %s", setup.Instrument, setup.BreakoutDirection)
	if n.dryRun {
		subject = "[DRY RUN] " + subject
	}
	n.send(ctx, subject, b.String())
}

func (n *Notifier) TradeExit(ctx context.Context, instrument, tradeID string, pl float64, reason string) {
	if !n.cfg.OnTradeExit {
		return
	}
	body := fmt.Sprintf("Instrument: %s\nTrade: %s\nRealized P&L: $%.2f\nReason: %s\n", instrument, tradeID, pl, reason)
	n.send(ctx, fmt.Sprintf("EXIT %s", instrument), body)
}

func (n *Notifier) DailySummary(ctx context.Context, dailyPnL, weeklyPnL, balance float64, trades int) {
	body := fmt.Sprintf("Balance: $%.2f\nDaily P&L: $%.2f\nWeekly P&L: $%.2f\nTrades today: %d\n", balance, dailyPnL, weeklyPnL, trades)
	n.send(ctx, "Daily P&L summary", body)
}

func (n *Notifier) WeeklySummary(ctx context.Context, weeklyPnL, balance float64, trades int) {
	body := fmt.Sprintf("Balance: $%.2f\nWeekly P&L: $%.2f\nTrades this week: %d\n", balance, weeklyPnL, trades)
	n.send(ctx, "Weekly summary", body)
}

func (n *Notifier) WeeklyTargetReached(ctx context.Context, weeklyPnL, targetPct, balance float64) {
	body := fmt.Sprintf("Weekly profit target reached.\nBalance: $%.2f\nWeekly P&L: $%.2f (target %.1f%%)\nTrading continues — no upside cap.\n", balance, weeklyPnL, targetPct)
	n.send(ctx, "Weekly profit target hit", body)
}

func (n *Notifier) DaemonStarted(ctx context.Context, universe []string, pollSeconds int) {
	body := fmt.Sprintf("Universe scanner running.\nEnvironment: %s\nInstruments: %d\nPoll: %ds\nSymbols: %s\n",
		n.envLabel, len(universe), pollSeconds, strings.Join(universe, ", "))
	n.send(ctx, "daemon started", body)
}
