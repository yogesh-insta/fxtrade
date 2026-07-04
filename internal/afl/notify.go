package afl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// FormatRoundReportSubject builds the weekly AFL round scan email subject.
func FormatRoundReportSubject(prefix string, fixtureCount, valueCount int) string {
	if prefix == "" {
		prefix = "[AFLPulse]"
	}
	if valueCount > 0 {
		return fmt.Sprintf("%s Round scan · %d fixtures · %d value bet(s)", prefix, fixtureCount, valueCount)
	}
	return fmt.Sprintf("%s Round scan · %d fixtures", prefix, fixtureCount)
}

// FormatRoundReportEmail renders the full weekly scan with predictions and highlighted value bets.
func FormatRoundReportEmail(reports []MatchReport, allValueBets []ValueBet) string {
	var b strings.Builder
	b.WriteString("AFLPulse weekly round scan\n")
	b.WriteString("Predictions use form, inside-50, clearances, venue fit, weather (Open-Meteo), injuries, and bookmaker odds.\n\n")

	for i, r := range reports {
		if i > 0 {
			b.WriteString("\n")
		}
		writeMatchReport(&b, r)
	}

	if len(allValueBets) > 0 {
		b.WriteString("\n--- Value bets (EV above threshold) ---\n")
		for i, vb := range allValueBets {
			side := "HOME"
			if !vb.IsHomePick {
				side = "AWAY"
			}
			fmt.Fprintf(&b, "  %d. %s vs %s — %s %s @ %.2f (%s) EV +%.1f%%\n",
				i+1, vb.HomeTeam, vb.AwayTeam, side, vb.Team, vb.DecimalOdds, vb.Bookmaker, vb.EV*100)
		}
	}

	b.WriteString("\n--- Manual execution required ---\n")
	b.WriteString("AFLPulse does NOT place bets automatically.\n")
	b.WriteString("Verify team news, lineups, and account limits before wagering.\n")
	return b.String()
}

func writeMatchReport(b *strings.Builder, r MatchReport) {
	ctx := r.Context
	fmt.Fprintf(b, "▸ %s vs %s", ctx.HomeTeam, ctx.AwayTeam)
	if !ctx.Kickoff.IsZero() {
		fmt.Fprintf(b, " · %s", ctx.Kickoff.Format(time.RFC1123))
	}
	b.WriteString("\n")

	winnerSide := "home"
	if r.Score.PredictedWinner == ctx.AwayTeam {
		winnerSide = "away"
	}
	fmt.Fprintf(b, "  Winner: %s (%s) · %.0f%% home win prob\n",
		r.Score.PredictedWinner, winnerSide, r.HomeWinProb*100)
	fmt.Fprintf(b, "  Predicted score: %s %d – %d %s (total %d, margin %d)\n",
		ctx.HomeTeam, r.Score.HomeScore, r.Score.AwayScore, ctx.AwayTeam, r.Score.TotalScore, r.Score.Margin)

	if r.TotalsLine != nil {
		diff := float64(r.Score.TotalScore) - r.TotalsLine.Line
		bias := "near"
		if diff > 3 {
			bias = "over"
		} else if diff < -3 {
			bias = "under"
		}
		fmt.Fprintf(b, "  Book total line: %.1f @ %s (model %s, Δ %+.1f)\n",
			r.TotalsLine.Line, r.TotalsLine.Bookmaker, bias, diff)
	}

	fmt.Fprintf(b, "  Venue: %s (%s) · weather rain %.1fmm wind %.0f km/h\n",
		ctx.Venue.Name, ctx.Venue.Dimension, ctx.Weather.RainMM, ctx.Weather.WindKPH)

	if len(r.ValueBets) > 0 {
		vb := r.ValueBets[0]
		fmt.Fprintf(b, "  ★ VALUE: %s @ %.2f (%s) EV +%.1f%%\n",
			vb.Team, vb.DecimalOdds, vb.Bookmaker, vb.EV*100)
	}
}

// FormatAlertSubject builds the AFLPulse value-bet email subject (legacy single-bet alert).
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

// SendRoundReport emails the weekly full round scan.
func SendRoundReport(n notify.Notifier, ctx context.Context, cfg config.AFLConfig, notif config.NotificationsConfig, reports []MatchReport, valueBets []ValueBet) {
	if len(reports) == 0 {
		return
	}
	prefix := notif.EffectiveAFLPrefix()
	subject := FormatRoundReportSubject(prefix, len(reports), len(valueBets))
	body := FormatRoundReportEmail(reports, valueBets)
	n.Send(ctx, subject, body)
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
