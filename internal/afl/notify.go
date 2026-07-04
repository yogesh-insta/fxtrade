package afl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

const fixtureRule = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

var melbourneLoc *time.Location

func init() {
	melbourneLoc, _ = time.LoadLocation("Australia/Melbourne")
}

var teamDisplayNames = map[TeamID]string{
	"ADE":  "Adelaide",
	"BRI":  "Brisbane",
	"CARL": "Carlton",
	"COLL": "Collingwood",
	"ESS":  "Essendon",
	"FRE":  "Fremantle",
	"GEE":  "Geelong",
	"GCS":  "Gold Coast",
	"GWS":  "GWS",
	"HAW":  "Hawthorn",
	"MEL":  "Melbourne",
	"NMFC": "North Melbourne",
	"PORT": "Port Adelaide",
	"RICH": "Richmond",
	"STK":  "St Kilda",
	"SYD":  "Sydney",
	"WCE":  "West Coast",
	"WB":   "Western Bulldogs",
}

// TeamDisplayName returns a readable club name for email output.
func TeamDisplayName(id TeamID) string {
	if name, ok := teamDisplayNames[id]; ok {
		return name
	}
	return string(id)
}

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
	writeRoundReportHeader(&b, reports, allValueBets)
	b.WriteString("\n")

	for i, r := range reports {
		if i > 0 {
			b.WriteString("\n")
		}
		writeMatchReport(&b, r)
	}

	if len(allValueBets) > 0 {
		b.WriteString("\n")
		writeValueBetsSummary(&b, allValueBets)
	}

	b.WriteString("\n")
	writeManualExecutionFooter(&b)
	return b.String()
}

func writeRoundReportHeader(b *strings.Builder, reports []MatchReport, valueBets []ValueBet) {
	b.WriteString("AFLPulse Round Scan\n")
	line := fmt.Sprintf("%d fixtures", len(reports))
	if len(valueBets) > 0 {
		line += fmt.Sprintf(" · %d value bets", len(valueBets))
	}
	if len(reports) > 0 {
		if meta := provenanceHeaderLine(reports[0].Provenance); meta != "" {
			line += " · " + meta
		}
	}
	b.WriteString(line)
	b.WriteString("\n")
}

func provenanceHeaderLine(p DataProvenance) string {
	var parts []string
	if p.StatsSource != "" {
		stats := p.StatsSource
		if p.StatsDetail != "" {
			stats += " " + p.StatsDetail
		}
		parts = append(parts, "stats "+stats)
	}
	if p.PredictorType != "" {
		parts = append(parts, "trained models")
	}
	return strings.Join(parts, " · ")
}

func writeMatchReport(b *strings.Builder, r MatchReport) {
	ctx := r.Context
	b.WriteString(fixtureRule)
	b.WriteString("\n")
	fmt.Fprintf(b, "%s vs %s", ctx.HomeTeam, ctx.AwayTeam)
	if kick := formatKickoffMelbourne(ctx.Kickoff); kick != "" {
		fmt.Fprintf(b, "  ·  %s", kick)
	}
	b.WriteString("\n")
	b.WriteString(fixtureRule)
	b.WriteString("\n\n")

	writePredictionSection(b, r)
	b.WriteString("\n\n")

	writeLastFiveSection(b, ctx)
	b.WriteString("\n\n")

	writeBookmakerSection(b, r)
	b.WriteString("\n\n")

	writeWhySection(b, r)
	b.WriteString("\n\n")

	writeContextSection(b, r)

	if hasScoreBreakdown(r.Score) {
		b.WriteString("\n\n")
		writeModelDetailSection(b, r)
	}

	if len(r.ValueBets) > 0 {
		b.WriteString("\n\n")
		writeFixtureValueBet(b, r.ValueBets[0])
	}
}

func writePredictionSection(b *strings.Builder, r MatchReport) {
	ctx := r.Context
	winner := r.Score.PredictedWinner
	b.WriteString("PREDICTION\n")
	fmt.Fprintf(b, "  Winner:     %s (%.0f%%)\n", TeamDisplayName(winner), r.WinnerWinProbability()*100)
	fmt.Fprintf(b, "  Score:      %s %d – %s %d  (margin %d)\n",
		TeamDisplayName(ctx.HomeTeam), r.Score.HomeScore,
		TeamDisplayName(ctx.AwayTeam), r.Score.AwayScore, r.Score.Margin)
	fmt.Fprintf(b, "  Total:      %d points\n", r.Score.TotalScore)
}

func writeLastFiveSection(b *strings.Builder, ctx MatchDayContext) {
	b.WriteString("LAST 5 GAMES\n")
	if line := formatTeamLast5Line(ctx.HomeTeam, ctx.HomeStats.Last5Scores); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if line := formatTeamLast5Line(ctx.AwayTeam, ctx.AwayStats.Last5Scores); line != "" {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(ctx.HomeStats.Last5Scores) == 0 && len(ctx.AwayStats.Last5Scores) == 0 {
		b.WriteString("  (no recent results on file)\n")
	}
}

func formatTeamLast5Line(team TeamID, scores []RecentMatchScore) string {
	if len(scores) == 0 {
		return ""
	}
	parts := make([]string, len(scores))
	for i, m := range scores {
		parts[i] = formatRecentMatchCompact(m)
	}
	return fmt.Sprintf("  %s:  %s", team, strings.Join(parts, " | "))
}

func formatRecentMatchCompact(m RecentMatchScore) string {
	tag := "D"
	switch {
	case m.For > m.Against:
		tag = "W"
	case m.For < m.Against:
		tag = "L"
	}
	total := m.Total
	if total <= 0 {
		total = m.For + m.Against
	}
	venue := ShortVenueName(m.Venue)
	if venue == "" {
		venue = "unknown venue"
	}
	return fmt.Sprintf("%d-%d vs %s @ %s (%d, %s)", m.For, m.Against, m.Opponent, venue, total, tag)
}

func writeBookmakerSection(b *strings.Builder, r MatchReport) {
	b.WriteString("VS BOOKMAKER\n")
	winner := r.Score.PredictedWinner
	if mo, ok := r.MarketOdds[winner]; ok && mo.DecimalOdds > 1 {
		fmt.Fprintf(b, "  H2H:        %s $%.2f (market ~%.0f%%) — %s\n",
			winner, mo.DecimalOdds, mo.ImpliedProb()*100, h2hModelNote(r, mo))
	} else if vb := valueBetForTeam(r.ValueBets, winner); vb != nil {
		fmt.Fprintf(b, "  H2H:        %s $%.2f (market ~%.0f%%) — %s\n",
			winner, vb.DecimalOdds, vb.ImpliedProb*100, h2hModelNoteFromVB(r, *vb))
	}

	if r.TotalsLine != nil {
		diff := float64(r.Score.TotalScore) - r.TotalsLine.Line
		lean := "NEAR"
		if diff > 3 {
			lean = "OVER"
		} else if diff < -3 {
			lean = "UNDER"
		}
		fmt.Fprintf(b, "  Total line: %.1f @ %s — model %d (lean %s, Δ %+.1f)\n",
			r.TotalsLine.Line, r.TotalsLine.Bookmaker, r.Score.TotalScore, lean, diff)
	}
}

func valueBetForTeam(bets []ValueBet, team TeamID) *ValueBet {
	for i := range bets {
		if bets[i].Team == team {
			return &bets[i]
		}
	}
	return nil
}

func h2hModelNote(r MatchReport, mo MarketOdds) string {
	modelProb := r.WinnerWinProbability()
	marketProb := mo.ImpliedProb()
	diff := modelProb - marketProb
	if diff > 0.05 {
		return "model more bullish than market"
	}
	if diff < -0.05 {
		return "model less bullish on blowout"
	}
	return "model aligned with market"
}

func h2hModelNoteFromVB(r MatchReport, vb ValueBet) string {
	return h2hModelNote(r, MarketOdds{DecimalOdds: vb.DecimalOdds})
}

func writeWhySection(b *strings.Builder, r MatchReport) {
	reasons := BuildMatchPredictionReasons(r)
	if len(reasons) == 0 {
		return
	}
	b.WriteString("WHY\n")
	for _, reason := range reasons {
		fmt.Fprintf(b, "  • %s\n", reason)
	}
}

func writeContextSection(b *strings.Builder, r MatchReport) {
	ctx := r.Context
	b.WriteString("CONTEXT\n")
	venueLine := fmt.Sprintf("  Venue: %s (%s)", shortVenueLabel(ctx.Venue.Name), ctx.Venue.Dimension)
	if ctx.Venue.AvgTotalScore > 0 {
		venueLine += fmt.Sprintf(" · avg total %.0f", ctx.Venue.AvgTotalScore)
	}
	b.WriteString(venueLine)
	b.WriteString("\n")
	fmt.Fprintf(b, "  Weather: %.1fmm rain, %.0f km/h wind\n",
		ctx.Weather.RainMM, ctx.Weather.WindKPH)
	writeProvenance(b, r)
	writeTeamFormLine(b, ctx)
}

func shortVenueLabel(name string) string {
	if short := ShortVenueName(name); short != "" && short != strings.TrimSpace(name) {
		return short
	}
	return name
}

func writeModelDetailSection(b *strings.Builder, r MatchReport) {
	b.WriteString("MODEL DETAIL\n")
	writeScoreBreakdown(b, r.Score, r.Context)
}

func writeFixtureValueBet(b *strings.Builder, vb ValueBet) {
	fmt.Fprintf(b, "★ VALUE BET: %s @ $%.2f (%s) EV +%.1f%%",
		TeamDisplayName(vb.Team), vb.DecimalOdds, vb.Bookmaker, vb.EV*100)
	if len(vb.Reasons) > 0 {
		b.WriteString("\n")
		for _, reason := range vb.Reasons {
			fmt.Fprintf(b, "  • %s\n", reason)
		}
	}
}

func writeValueBetsSummary(b *strings.Builder, allValueBets []ValueBet) {
	b.WriteString(fixtureRule)
	fmt.Fprintf(b, "\nVALUE BETS (%d)\n", len(allValueBets))
	b.WriteString(fixtureRule)
	b.WriteString("\n\n")
	for i, vb := range allValueBets {
		fmt.Fprintf(b, "%d. %s vs %s — %s @ $%.2f (%s) EV +%.1f%%\n",
			i+1, vb.HomeTeam, vb.AwayTeam, TeamDisplayName(vb.Team), vb.DecimalOdds, vb.Bookmaker, vb.EV*100)
		for _, reason := range vb.Reasons {
			fmt.Fprintf(b, "   • %s\n", reason)
		}
		if i < len(allValueBets)-1 {
			b.WriteString("\n")
		}
	}
}

func writeManualExecutionFooter(b *strings.Builder) {
	b.WriteString("--- Manual execution required ---\n")
	b.WriteString("AFLPulse does NOT place bets automatically.\n")
	b.WriteString("Verify team news, lineups, and account limits before wagering.\n")
}

func formatKickoffMelbourne(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if melbourneLoc != nil {
		return t.In(melbourneLoc).Format("Mon 2 Jan 3:04 PM MST")
	}
	return t.UTC().Format("Mon 2 Jan 3:04 PM UTC")
}

func hasScoreBreakdown(score ScoreProjection) bool {
	return score.Breakdown.BaselineTotal > 0 || score.Breakdown.RawMargin != 0
}

func writeScoreBreakdown(b *strings.Builder, score ScoreProjection, ctx MatchDayContext) {
	d := score.Breakdown
	if d.BaselineTotal == 0 && d.RawMargin == 0 {
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
	} else if d.BaselineTotal > 0 {
		fmt.Fprintf(b, "  Score model: baseline %.0f × weather %.2f = %.0f total\n",
			d.BaselineTotal, d.WeatherFactor, d.AdjustedTotal)
	}
	if d.RawMargin != 0 || d.ProfileEdge != 0 {
		fmt.Fprintf(b, "  Margin model: offensive %s %.2f vs %s %.2f (profile %+.2f, win %+.2f → margin %+.1f)\n",
			ctx.HomeTeam, d.HomeOffensive, ctx.AwayTeam, d.AwayOffensive, d.ProfileEdge, d.WinEdge, d.RawMargin)
	}
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
	if kick := formatKickoffMelbourne(top.Kickoff); kick != "" {
		fmt.Fprintf(&b, "Kickoff: %s\n", kick)
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
	writeManualExecutionFooter(&b)
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
