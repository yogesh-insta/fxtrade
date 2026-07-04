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
	b.WriteString("LIVE ANALYTICS (Gemini + Google Search)\n")
	b.WriteString(fixtureRule)
	b.WriteString("\n\n")

	if s := strings.TrimSpace(llm.MatchDetails.LateChanges); s != "" {
		b.WriteString("Late changes\n")
		b.WriteString(wrapIndented("  • ", s, emailLineWidth, "    "))
		b.WriteString("\n\n")
	}
	if s := strings.TrimSpace(llm.MatchDetails.WeatherImpact); s != "" {
		b.WriteString("Weather impact\n")
		b.WriteString(wrapIndented("  • ", s, emailLineWidth, "    "))
		b.WriteString("\n\n")
	}

	conf := strings.TrimSpace(llm.MainBet.Confidence)
	if conf == "" {
		conf = "medium"
	}
	fmt.Fprintf(&b, "Main bet (%s confidence): %s [%s]\n",
		conf, llm.MainBet.Selection, strings.TrimSpace(llm.MainBet.Market))
	for _, r := range llm.MainBet.Reasons {
		if s := strings.TrimSpace(r); s != "" {
			b.WriteString(wrapIndented("  • ", s, emailLineWidth, "    "))
			b.WriteString("\n")
		}
	}

	if p := strings.TrimSpace(llm.PlayerProp.Player); p != "" {
		b.WriteString("\nPlayer prop: ")
		b.WriteString(p)
		if m := strings.TrimSpace(llm.PlayerProp.Market); m != "" {
			b.WriteString(" — ")
			b.WriteString(m)
		}
		b.WriteString("\n")
		for _, r := range llm.PlayerProp.Reasons {
			if s := strings.TrimSpace(r); s != "" {
				b.WriteString(wrapIndented("  • ", s, emailLineWidth, "    "))
				b.WriteString("\n")
			}
		}
	}

	if s := strings.TrimSpace(llm.RiskNote); s != "" {
		b.WriteString("\nRisk note\n")
		b.WriteString(wrapIndented("  • ", s, emailLineWidth, "    "))
		b.WriteString("\n")
	}

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
	b.WriteString("\nPregame email was NOT sent. Dedup state unchanged — will retry on next poll if still in window.\n")
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
