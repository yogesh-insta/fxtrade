package afl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// FormatPregameSubject builds the T-30 pregame email subject.
func FormatPregameSubject(prefix string, home, away TeamID) string {
	if prefix == "" {
		prefix = "[AFLPulse PRE]"
	}
	return fmt.Sprintf("%s T-30 · %s vs %s", prefix, home, away)
}

// FormatPregameFailureSubject builds the Gemini failure alert subject.
func FormatPregameFailureSubject(prefix string, home, away TeamID) string {
	if prefix == "" {
		prefix = "[AFLPulse PRE]"
	}
	return fmt.Sprintf("%s ALERT · Gemini failed · %s vs %s", prefix, home, away)
}

// FormatPregameEmail renders the T-30 pregame body from model report and LLM JSON.
// Empty or partial LLM content never prints Go zero values (e.g. "[]"); it notes incompleteness.
func FormatPregameEmail(game SquiggleFixture, report MatchReport, llm PregameLLMResponse) string {
	var b strings.Builder
	ctx := report.Context
	b.WriteString("AFLPulse Pre-Game (T-30)\n")
	for _, line := range FormatFixtureHeaderLines(ctx) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if ladder := FormatFixtureLadderLine(ctx); ladder != "" {
		b.WriteString(ladder)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Squiggle game ID: %d\n\n", game.ID)

	b.WriteString(fixtureRule)
	b.WriteString("\n")
	b.WriteString("MODEL BASELINE\n")
	b.WriteString(fixtureRule)
	b.WriteString("\n\n")
	writePredictionSection(&b, report)

	b.WriteString("\n\n")
	b.WriteString(fixtureRule)
	b.WriteString("\n")
	b.WriteString("LIVE ANALYTICS (Gemini)\n")
	b.WriteString(fixtureRule)
	b.WriteString("\n\n")
	writePregameLiveAnalytics(&b, llm)

	b.WriteString("\n")
	if s := strings.TrimSpace(llm.Disclaimer); s != "" {
		b.WriteString(s)
	} else {
		b.WriteString("Gambling involves risk. Bet responsibly.")
	}
	b.WriteString("\n\n")
	writeManualExecutionFooter(&b)
	return b.String()
}

func writePregameLiveAnalytics(b *strings.Builder, llm PregameLLMResponse) {
	if llm.IsEmpty() {
		b.WriteString("  Gemini returned incomplete analysis\n")
		b.WriteString("  Live analytics: unavailable\n")
		return
	}

	if llm.Incomplete() {
		b.WriteString("  Gemini returned incomplete analysis\n\n")
	}

	if s := strings.TrimSpace(llm.MatchDetails.LateChanges); s != "" {
		b.WriteString("  Late changes: ")
		b.WriteString(s)
		b.WriteString("\n")
	}
	if s := strings.TrimSpace(llm.MatchDetails.WeatherImpact); s != "" {
		b.WriteString("  Weather: ")
		b.WriteString(s)
		b.WriteString("\n")
	}
	if s := strings.TrimSpace(llm.MatchDetails.Lineups); s != "" {
		b.WriteString("  Lineups: ")
		b.WriteString(s)
		b.WriteString("\n")
	}

	sel := strings.TrimSpace(llm.MainBet.Selection)
	mkt := strings.TrimSpace(llm.MainBet.Market)
	conf := strings.TrimSpace(llm.MainBet.Confidence)
	if conf == "" {
		conf = "medium"
	}
	switch {
	case sel != "" && mkt != "":
		fmt.Fprintf(b, "  Main bet: %s %s (%s)\n", sel, mkt, conf)
	case sel != "":
		fmt.Fprintf(b, "  Main bet: %s (%s)\n", sel, conf)
	default:
		b.WriteString("  Main bet: unavailable\n")
	}

	if s := strings.TrimSpace(llm.MainBet.OddsNote); s != "" {
		b.WriteString("  Odds note: ")
		b.WriteString(s)
		b.WriteString("\n")
	}

	reasons := trimNonEmpty(llm.MainBet.Reasons)
	if len(reasons) > 0 {
		b.WriteString("  Why:\n")
		for _, r := range reasons {
			b.WriteString(wrapIndented("    • ", r, emailLineWidth, "      "))
			b.WriteString("\n")
		}
	}

	player := strings.TrimSpace(llm.PlayerProp.Player)
	propMkt := strings.TrimSpace(llm.PlayerProp.Market)
	if player != "" || propMkt != "" {
		b.WriteString("  Player prop: ")
		switch {
		case player != "" && propMkt != "":
			b.WriteString(player)
			b.WriteString(" — ")
			b.WriteString(propMkt)
		case player != "":
			b.WriteString(player)
		default:
			b.WriteString(propMkt)
		}
		b.WriteString("\n")
		for _, r := range trimNonEmpty(llm.PlayerProp.Reasons) {
			b.WriteString(wrapIndented("    • ", r, emailLineWidth, "      "))
			b.WriteString("\n")
		}
	}

	if s := strings.TrimSpace(llm.RiskNote); s != "" {
		b.WriteString("  Risks: ")
		b.WriteString(s)
		b.WriteString("\n")
	}

	if s := strings.TrimSpace(llm.ModelAgreement); s != "" {
		b.WriteString("  Model agreement: ")
		b.WriteString(s)
		b.WriteString("\n")
	}
}

// FormatPregameFailureAlert renders the Gemini failure alert body.
func FormatPregameFailureAlert(game SquiggleFixture, home, away TeamID, venue string, err error, logPath string) string {
	var b strings.Builder
	b.WriteString("AFLPulse pregame alert — Gemini failed after retries.\n\n")
	fmt.Fprintf(&b, "Match: %s vs %s\n", home, away)
	if !game.Kickoff.IsZero() {
		fmt.Fprintf(&b, "Kickoff: %s\n", formatKickoffMelbourne(game.Kickoff))
	}
	if venue != "" {
		fmt.Fprintf(&b, "Venue: %s\n", venue)
	}
	fmt.Fprintf(&b, "Squiggle ID: %d\n", game.ID)
	fmt.Fprintf(&b, "Timestamp: %s\n", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		fmt.Fprintf(&b, "Error: %s\n", err)
	}
	if logPath != "" {
		fmt.Fprintf(&b, "\nCheck logs: %s\n", logPath)
	}
	b.WriteString("\nModel-baseline pregame email was still sent (live analytics unavailable).\n")
	return b.String()
}

// SendPregameEmail sends the T-30 pregame report.
func SendPregameEmail(n notify.Notifier, ctx context.Context, notif config.NotificationsConfig, home, away TeamID, body string) {
	prefix := notif.EffectiveAFLPrefix()
	if !strings.Contains(prefix, "PRE") {
		prefix = strings.TrimSpace(prefix) + " PRE"
	}
	subject := FormatPregameSubject(prefix, home, away)
	n.Send(ctx, subject, body)
}

// SendPregameFailureAlert emails when Gemini fails after retries.
func SendPregameFailureAlert(n notify.Notifier, ctx context.Context, notif config.NotificationsConfig, home, away TeamID, body string) {
	prefix := notif.EffectiveAFLPrefix()
	if !strings.Contains(prefix, "PRE") {
		prefix = strings.TrimSpace(prefix) + " PRE"
	}
	subject := FormatPregameFailureSubject(prefix, home, away)
	n.Send(ctx, subject, body)
}
