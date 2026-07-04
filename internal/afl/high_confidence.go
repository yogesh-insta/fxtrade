package afl

import (
	"fmt"
	"strings"
)

// HighConfidenceBet is a supplementary wager suggestion on top of the model prediction.
type HighConfidenceBet struct {
	Type   string   // h2h, margin, totals, player_prop
	Label  string
	Odds   *float64
	Source string // model, curated
	Why    string
}

// HighConfidenceBundle groups supplemental bets for one fixture.
type HighConfidenceBundle struct {
	MatchBets   []HighConfidenceBet
	PlayerProps []HighConfidenceBet
}

// BuildHighConfidenceBets assembles curated and model-aligned high-confidence picks
// without replacing the core AFLPulse prediction above.
func BuildHighConfidenceBets(r MatchReport) HighConfidenceBundle {
	var bundle HighConfidenceBundle
	bundle.MatchBets = append(bundle.MatchBets, curatedMatchBets(r)...)
	bundle.MatchBets = append(bundle.MatchBets, modelMatchBets(r, bundle.MatchBets)...)
	bundle.PlayerProps = curatedPlayerProps(r)
	return bundle
}

func curatedMatchBets(r MatchReport) []HighConfidenceBet {
	if r.Editorial == nil {
		return nil
	}
	var notes []string
	for _, n := range r.Editorial.Notes {
		if n = strings.TrimSpace(n); n != "" {
			notes = append(notes, n)
		}
	}
	notesTail := strings.Join(notes, " ")

	var out []HighConfidenceBet
	for _, p := range r.Editorial.SmartPicks {
		why := strings.TrimSpace(p.Why)
		if why == "" && notesTail != "" {
			why = notesTail
		}
		out = append(out, HighConfidenceBet{
			Type:   p.Type,
			Label:  p.FormatSmartPickLine(),
			Odds:   p.Odds,
			Source: "curated",
			Why:    why,
		})
	}
	return out
}

func curatedPlayerProps(r MatchReport) []HighConfidenceBet {
	if r.Editorial == nil {
		return nil
	}
	var out []HighConfidenceBet
	for _, p := range r.Editorial.PlayerProps {
		label := fmt.Sprintf("%s %s", p.Player, p.Market)
		if p.Team != "" {
			label += fmt.Sprintf(" (%s)", p.Team)
		}
		out = append(out, HighConfidenceBet{
			Type:   "player_prop",
			Label:  label,
			Source: "curated",
			Why:    p.Why,
		})
	}
	return out
}

func modelMatchBets(r MatchReport, existing []HighConfidenceBet) []HighConfidenceBet {
	var out []HighConfidenceBet
	winner := r.Score.PredictedWinner
	winProb := r.WinnerWinProbability()

	if winProb >= 0.58 && !hasBetType(existing, "h2h") {
		label := fmt.Sprintf("%s Head-to-Head", TeamDisplayName(winner))
		var odds *float64
		if mo, ok := r.MarketOdds[winner]; ok && mo.DecimalOdds > 1 {
			o := mo.DecimalOdds
			odds = &o
		}
		why := fmt.Sprintf("Model rates %s %.0f%% to win (trained H2H predictor)", TeamDisplayName(winner), winProb*100)
		if mo, ok := r.MarketOdds[winner]; ok && mo.DecimalOdds > 1 {
			why += fmt.Sprintf(" · market $%.2f (~%.0f%%)", mo.DecimalOdds, mo.ImpliedProb()*100)
		}
		out = append(out, HighConfidenceBet{
			Type: "h2h", Label: formatBetLabel(label, odds), Odds: odds, Source: "model", Why: why,
		})
	}

	margin := r.Score.Margin
	if margin > 0 && margin < 40 && !hasBetType(existing, "margin") {
		label := fmt.Sprintf("%s 1-39 Margin", TeamDisplayName(winner))
		out = append(out, HighConfidenceBet{
			Type:   "margin",
			Label:  label,
			Source: "model",
			Why:    fmt.Sprintf("Projected margin %d pts — inside the 1-39 window", margin),
		})
	}

	if r.TotalsLine != nil && !hasBetType(existing, "totals") {
		diff := float64(r.Score.TotalScore) - r.TotalsLine.Line
		switch {
		case diff > 8:
			label := fmt.Sprintf("OVER %.1f", r.TotalsLine.Line)
			out = append(out, HighConfidenceBet{
				Type: "totals", Label: label, Source: "model",
				Why: fmt.Sprintf("Model total %d is %.0f pts above line %.1f @ %s",
					r.Score.TotalScore, diff, r.TotalsLine.Line, r.TotalsLine.Bookmaker),
			})
		case diff < -8:
			label := fmt.Sprintf("UNDER %.1f", r.TotalsLine.Line)
			out = append(out, HighConfidenceBet{
				Type: "totals", Label: label, Source: "model",
				Why: fmt.Sprintf("Model total %d is %.0f pts below line %.1f @ %s",
					r.Score.TotalScore, -diff, r.TotalsLine.Line, r.TotalsLine.Bookmaker),
			})
		}
	}

	return out
}

func hasBetType(bets []HighConfidenceBet, typ string) bool {
	for _, b := range bets {
		if b.Type == typ {
			return true
		}
	}
	return false
}

func formatBetLabel(label string, odds *float64) string {
	if odds != nil && *odds > 1 {
		return fmt.Sprintf("%s ($%.2f)", label, *odds)
	}
	return label
}

func (b HighConfidenceBet) formatLine() string {
	line := b.Label
	if b.Source == "curated" {
		line += " [curated]"
	} else if b.Source == "model" {
		line += " [model]"
	}
	return line
}
