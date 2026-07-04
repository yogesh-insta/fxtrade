package afl

import (
	"fmt"
	"strings"
	"time"
)

// GeminiAnalyticsSystemPrompt is the elite AFL analytics system prompt for grounded search.
const GeminiAnalyticsSystemPrompt = `You are an elite sports analytics AI specializing in Australian Rules Football (AFL) and sports betting markets.

Analyze the upcoming AFL Round match specified in the user message.

Perform a live web search to gather the following real-time parameters:
1. Final 22-man selection line-ups and confirmed late changes (including the designated tactical sub) published by the AFL 90 minutes before the game.
2. Current stadium weather conditions (wind speeds, rain percentages) at the venue and their potential impact on ball handling or total scoring.
3. Live market odds fluctuations across major platforms (Head-to-Head, Line/Handicap, and Total Game Over/Under).

Based on this real-time data combined with historical team trends, team rest schedules, and structural tactical match-ups, generate a high-confidence betting summary.

Format the output strictly as a clean, ready-to-read text email body with the following structural layout:
- Subject Line: [AFL Round N Analytics] - [Team A] vs [Team B] Final High-Confidence Selections
- Match Details: Date, Time, Venue, Confirmed Late Changes, and Weather Impact.
- Main High-Confidence Bet Selection: The single sharpest market play (H2H, Line, or Game Total) supported by 3 definitive statistical bullet points.
- High-Value Player Prop Selection: One calculated player prop milestone (Disposals or Goal Scorers) supported by 2 recent form metrics.
- A concise risk-mitigation note regarding structural anomalies or injury vulnerabilities.
- Mandatory Responsible Gambling Disclaimer.

Use the AFLPulse model baseline in the user message only as a cross-check — your primary evidence must come from live search results.`

// BuildGeminiAnalyticsUserPrompt formats the per-fixture user message with model context.
func BuildGeminiAnalyticsUserPrompt(round int, r MatchReport) string {
	ctx := r.Context
	home := TeamDisplayName(ctx.HomeTeam)
	away := TeamDisplayName(ctx.AwayTeam)
	match := fmt.Sprintf("%s vs %s", home, away)

	var b strings.Builder
	if round > 0 {
		fmt.Fprintf(&b, "Analyze the upcoming AFL Round %d match: %s.\n\n", round, match)
	} else {
		fmt.Fprintf(&b, "Analyze the upcoming AFL match: %s.\n\n", match)
	}

	if !ctx.Kickoff.IsZero() {
		fmt.Fprintf(&b, "Scheduled kickoff (UTC): %s\n", ctx.Kickoff.UTC().Format(time.RFC3339))
	}
	if ctx.Venue.Name != "" {
		fmt.Fprintf(&b, "Venue: %s\n", ctx.Venue.Name)
	}
	b.WriteString("\nAFLPulse model baseline (for cross-check only — verify everything with live search):\n")
	fmt.Fprintf(&b, "- Predicted winner: %s (%.0f%%)\n",
		TeamDisplayName(r.Score.PredictedWinner), r.WinnerWinProbability()*100)
	fmt.Fprintf(&b, "- Projected score: %s %d – %s %d (margin %d)\n",
		home, r.Score.HomeScore, away, r.Score.AwayScore, r.Score.Margin)
	fmt.Fprintf(&b, "- Projected total: %d points\n", r.Score.TotalScore)

	if r.TotalsLine != nil {
		fmt.Fprintf(&b, "- Book total line: %.1f (%s)\n", r.TotalsLine.Line, r.TotalsLine.Bookmaker)
	}
	if mo, ok := r.MarketOdds[r.Score.PredictedWinner]; ok && mo.DecimalOdds > 1 {
		fmt.Fprintf(&b, "- Market H2H (%s): $%.2f\n", r.Score.PredictedWinner, mo.DecimalOdds)
	}

	h, a := ctx.HomeStats, ctx.AwayStats
	fmt.Fprintf(&b, "- Form last 10: %s %d–%d | %s %d–%d\n",
		ctx.HomeTeam, h.FormWinsLast10, h.FormLossesLast10,
		ctx.AwayTeam, a.FormWinsLast10, a.FormLossesLast10)

	return b.String()
}
