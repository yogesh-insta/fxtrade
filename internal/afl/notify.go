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
	b.WriteString("Predictions use live Squiggle ladder/form, rolling scoring trends, trained models\n")
	b.WriteString("(2018–2025 history), bookmaker odds, weather, and optional injuries.\n\n")

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
			for _, reason := range vb.Reasons {
				fmt.Fprintf(&b, "      • %s\n", reason)
			}
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

	b.WriteString(FormatMatchPredictions(r))
	b.WriteString("\n")
	writeScoreBreakdown(b, r.Score, ctx)
	writeTeamFormLine(b, ctx)
	writeLastFiveScores(b, ctx)
	writePredictionReasons(b, r)

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

	writeProvenance(b, r)

	if len(r.ValueBets) > 0 {
		vb := r.ValueBets[0]
		fmt.Fprintf(b, "  ★ VALUE: %s @ %.2f (%s) EV +%.1f%%\n",
			vb.Team, vb.DecimalOdds, vb.Bookmaker, vb.EV*100)
		if len(vb.Reasons) > 0 {
			b.WriteString("  Value bet detail:\n")
			for _, reason := range vb.Reasons {
				fmt.Fprintf(b, "    • %s\n", reason)
			}
		}
	}
}

func writePredictionReasons(b *strings.Builder, r MatchReport) {
	reasons := BuildMatchPredictionReasons(r)
	if len(reasons) == 0 {
		return
	}
	b.WriteString("  Why this prediction:\n")
	for _, reason := range reasons {
		fmt.Fprintf(b, "    • %s\n", reason)
	}
}

func writeScoreBreakdown(b *strings.Builder, score ScoreProjection, ctx MatchDayContext) {
	d := score.Breakdown
	if d.BaselineTotal == 0 {
		return
	}
	if d.TeamBasedTotal > 0 {
		fmt.Fprintf(b, "  Total model: %s %.0f for / %.0f against · %s %.0f for / %.0f against\n",
			ctx.HomeTeam, teamPtsFor(ctx.HomeStats), teamPtsAgainst(ctx.HomeStats),
			ctx.AwayTeam, teamPtsFor(ctx.AwayStats), teamPtsAgainst(ctx.AwayStats))
		fmt.Fprintf(b, "  Expected: %s %.0f + %s %.0f = %.0f team-based",
			ctx.HomeTeam, d.HomeExpected, ctx.AwayTeam, d.AwayExpected, d.TeamBasedTotal)
		if d.VenueAvgTotal > 0 {
			fmt.Fprintf(b, " · venue avg %.0f → blended %.0f", d.VenueAvgTotal, d.BlendedTotal)
		}
		fmt.Fprintf(b, " × weather %.2f = %.0f\n", d.WeatherFactor, d.AdjustedTotal)
	} else {
		fmt.Fprintf(b, "  Score model: baseline %.0f × weather %.2f = %.0f total\n",
			d.BaselineTotal, d.WeatherFactor, d.AdjustedTotal)
	}
	fmt.Fprintf(b, "  Margin model: offensive %s %.2f vs %s %.2f (profile %+.2f, win %+.2f → margin %+.1f)\n",
		ctx.HomeTeam, d.HomeOffensive, ctx.AwayTeam, d.AwayOffensive, d.ProfileEdge, d.WinEdge, d.RawMargin)
	if d.HomeGoalsBehind != "" {
		fmt.Fprintf(b, "  AFL format: %s %s – %s %s\n",
			ctx.HomeTeam, d.HomeGoalsBehind, ctx.AwayTeam, d.AwayGoalsBehind)
	}
}

func teamPtsFor(s TeamStats) float64 {
	if s.PointsForPerGame > 0 {
		return s.PointsForPerGame
	}
	return LeagueBaselineTeamPoints
}

func teamPtsAgainst(s TeamStats) float64 {
	if s.PointsAgainstPerGame > 0 {
		return s.PointsAgainstPerGame
	}
	return LeagueBaselineTeamPoints
}

func writeLastFiveScores(b *strings.Builder, ctx MatchDayContext) {
	if line := FormatTeamLast5(ctx.HomeTeam, ctx.HomeStats.Last5Scores); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if line := FormatTeamLast5(ctx.AwayTeam, ctx.AwayStats.Last5Scores); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func writeTeamFormLine(b *strings.Builder, ctx MatchDayContext) {
	h := ctx.HomeStats
	a := ctx.AwayStats
	fmt.Fprintf(b, "  Form (last 10): %s %d-%d", ctx.HomeTeam, h.FormWinsLast10, h.FormLossesLast10)
	if h.RecentPointsForPerGame > 0 {
		fmt.Fprintf(b, " · scoring %.0f (trend %+.0f)", h.RecentPointsForPerGame, h.ScoringTrendFor)
	}
	if h.H2HGamesVsOpponent > 0 {
		fmt.Fprintf(b, " · H2H %d-%d", h.H2HWinsVsOpponent, h.H2HGamesVsOpponent-h.H2HWinsVsOpponent)
	}
	fmt.Fprintf(b, " | %s %d-%d", ctx.AwayTeam, a.FormWinsLast10, a.FormLossesLast10)
	if a.RecentPointsForPerGame > 0 {
		fmt.Fprintf(b, " · scoring %.0f (trend %+.0f)", a.RecentPointsForPerGame, a.ScoringTrendFor)
	}
	b.WriteString("\n")
}

func writeProvenance(b *strings.Builder, r MatchReport) {
	p := r.Provenance
	if p.StatsSource == "" && p.PredictorType == "" {
		return
	}
	b.WriteString("  Data: ")
	parts := make([]string, 0, 5)
	if p.StatsSource != "" {
		line := p.StatsSource
		if p.StatsDetail != "" {
			line += " (" + p.StatsDetail + ")"
		}
		if !p.StatsAsOf.IsZero() {
			line += " as of " + p.StatsAsOf.Format("2006-01-02 15:04 UTC")
		}
		parts = append(parts, "stats "+line)
	}
	if p.OddsSource != "" {
		line := p.OddsSource
		if !p.OddsUpdatedAt.IsZero() {
			line += " @ " + p.OddsUpdatedAt.Format("2006-01-02 15:04 UTC")
		}
		parts = append(parts, "odds "+line)
	}
	if p.WeatherSource != "" {
		parts = append(parts, "weather "+p.WeatherSource)
	}
	if p.PredictorType != "" {
		parts = append(parts, "model "+p.PredictorType)
	}
	if p.InjuriesApplied {
		parts = append(parts, fmt.Sprintf("injuries %s (%d out)", p.InjuriesFile, p.InjuriesCount))
	} else if p.InjuriesFile != "" {
		parts = append(parts, "injuries none flagged")
	}
	fmt.Fprintf(b, "%s\n", strings.Join(parts, " · "))
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
