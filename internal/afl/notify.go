package afl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// FormatAlertSubject builds the AFLPulse email subject.
func FormatAlertSubject(prefix string, bet ValueBet) string {
	if prefix == "" {
		prefix = "[AFLPulse]"
	}
	side := "HOME"
	if !bet.IsHomePick {
		side = "AWAY"
	}
	return fmt.Sprintf("%s VALUE %s %s vs %s · %.2f @ %s · EV +%.1f%%",
		prefix, side, bet.Team, opponent(bet), bet.DecimalOdds, bet.Bookmaker, bet.EV*100)
}

func opponent(bet ValueBet) string {
	if bet.IsHomePick {
		return string(bet.AwayTeam)
	}
	return string(bet.HomeTeam)
}

// FormatAlertEmail renders the value bet alert body with reasons and contenders.
func FormatAlertEmail(top ValueBet, contenders []ValueBet) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Match: %s vs %s\n", top.HomeTeam, top.AwayTeam)
	if !top.Kickoff.IsZero() {
		fmt.Fprintf(&b, "Kickoff: %s\n", top.Kickoff.Format(time.RFC1123))
	}
	fmt.Fprintf(&b, "Selection: %s (%s)\n", top.Team, bookSide(top))
	fmt.Fprintf(&b, "Bookmaker: %s\n", top.Bookmaker)
	fmt.Fprintf(&b, "Decimal odds: %.2f\n", top.DecimalOdds)
	fmt.Fprintf(&b, "Model probability: %.1f%%\n", top.ModelProb*100)
	fmt.Fprintf(&b, "Implied probability: %.1f%%\n", top.ImpliedProb*100)
	fmt.Fprintf(&b, "Expected value (edge): +%.1f%%\n", top.EV*100)
	b.WriteString("\n")

	if len(top.Reasons) > 0 {
		b.WriteString("Why this bet:\n")
		for _, r := range top.Reasons {
			fmt.Fprintf(&b, "  • %s\n", r)
		}
		b.WriteString("\n")
	}

	writeContenders(&b, contenders, top.Team)
	b.WriteString("\n")
	b.WriteString("--- Manual execution required ---\n")
	b.WriteString("AFLPulse does NOT place bets automatically.\n")
	b.WriteString("Verify team news, lineups, and account limits before wagering.\n")
	return b.String()
}

func bookSide(bet ValueBet) string {
	if bet.IsHomePick {
		return "home"
	}
	return "away"
}

func writeContenders(b *strings.Builder, contenders []ValueBet, selected TeamID) {
	if len(contenders) == 0 {
		return
	}
	if len(contenders) < 5 {
		fmt.Fprintf(b, "Top contenders (%d value bets):\n", len(contenders))
	} else {
		b.WriteString("Top 5 contenders:\n")
	}
	for i, c := range contenders {
		marker := ""
		if c.Team == selected {
			marker = " ★ selected"
		}
		fmt.Fprintf(b, "  %d. %s vs %s — pick %s @ %.2f (%s) EV +%.1f%%%s\n",
			i+1, c.HomeTeam, c.AwayTeam, c.Team, c.DecimalOdds, c.Bookmaker, c.EV*100, marker)
	}
}

// SendAlert emails the top value bet using the shared notify package.
func SendAlert(n notify.Notifier, ctx context.Context, prefix string, top ValueBet, contenders []ValueBet) {
	subject := FormatAlertSubject(prefix, top)
	body := FormatAlertEmail(top, contenders)
	n.Send(ctx, subject, body)
}

// SendAlertMulti sends a summary when multiple value bets exist but highlights the best.
func SendAlertMulti(n notify.Notifier, ctx context.Context, cfg config.AFLConfig, notif config.NotificationsConfig, bets []ValueBet) {
	if len(bets) == 0 {
		return
	}
	topN := TopN(bets, cfg.AlertTopN)
	prefix := notif.EffectiveAFLPrefix()
	SendAlert(n, ctx, prefix, topN[0], topN)
}
