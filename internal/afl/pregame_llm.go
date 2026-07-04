package afl

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/afl/gemini"
	"github.com/ym/fxtrade/internal/config"
)

// PregameLLMSystemPrompt instructs Gemini to return structured JSON for T-30 emails.
const PregameLLMSystemPrompt = `You are an elite AFL sports analytics AI with live web search.

The user message is compact JSON describing one upcoming AFL fixture and an AFLPulse model baseline.
Use Google Search to verify and enrich:
1. Final 22-man line-ups and confirmed late changes (including tactical sub) ~90 minutes before bounce.
2. Current venue weather (wind, rain) and scoring impact.
3. Live market odds (H2H, line/handicap, total over/under) across major AU books.

Respond with ONLY valid JSON matching this schema (no markdown fences, no prose outside JSON):
{
  "match_details": {
    "late_changes": "string — confirmed ins/outs and tactical sub",
    "weather_impact": "string — wind/rain and scoring/handling impact"
  },
  "main_bet": {
    "market": "H2H | Line | Total",
    "selection": "string — e.g. St Kilda H2H or Over 168.5",
    "confidence": "high | medium",
    "reasons": ["reason 1", "reason 2", "reason 3"]
  },
  "player_prop": {
    "player": "string",
    "market": "string — e.g. 25+ disposals",
    "reasons": ["reason 1", "reason 2"]
  },
  "risk_note": "string — structural anomalies or injury vulnerabilities",
  "disclaimer": "Gambling involves risk. Bet responsibly."
}

Ground every field in live search results. Use the model baseline only as a cross-check.`

// PregameLLMResponse is the structured Gemini output for pregame emails.
type PregameLLMResponse struct {
	MatchDetails struct {
		LateChanges   string `json:"late_changes"`
		WeatherImpact string `json:"weather_impact"`
	} `json:"match_details"`
	MainBet struct {
		Market     string   `json:"market"`
		Selection  string   `json:"selection"`
		Confidence string   `json:"confidence"`
		Reasons    []string `json:"reasons"`
	} `json:"main_bet"`
	PlayerProp struct {
		Player  string   `json:"player"`
		Market  string   `json:"market"`
		Reasons []string `json:"reasons"`
	} `json:"player_prop"`
	RiskNote    string `json:"risk_note"`
	Disclaimer  string `json:"disclaimer"`
}

// PregameLLMPayload is the compact JSON sent to Gemini as the user message.
type PregameLLMPayload struct {
	SquiggleID int    `json:"squiggle_id"`
	Round      int    `json:"round,omitempty"`
	HomeTeam   string `json:"home_team"`
	AwayTeam   string `json:"away_team"`
	Venue      string `json:"venue,omitempty"`
	KickoffUTC string `json:"kickoff_utc,omitempty"`
	Model      struct {
		Winner       string  `json:"predicted_winner"`
		WinProbPct   float64 `json:"win_prob_pct"`
		HomeScore    int     `json:"home_score"`
		AwayScore    int     `json:"away_score"`
		Margin       int     `json:"margin"`
		Total        int     `json:"total_points"`
		TotalsLine   float64 `json:"totals_line,omitempty"`
		TotalsBook   string  `json:"totals_book,omitempty"`
		MarketH2HOdds float64 `json:"market_h2h_odds,omitempty"`
	} `json:"model_baseline"`
	Form struct {
		HomeWins10   int `json:"home_wins_last10"`
		HomeLosses10 int `json:"home_losses_last10"`
		AwayWins10   int `json:"away_wins_last10"`
		AwayLosses10 int `json:"away_losses_last10"`
	} `json:"form"`
}

// BuildPregameLLMPayload serializes fixture context and model baseline for Gemini.
func BuildPregameLLMPayload(game SquiggleFixture, report MatchReport) ([]byte, error) {
	ctx := report.Context
	var p PregameLLMPayload
	p.SquiggleID = game.ID
	p.Round = game.Round
	p.HomeTeam = string(ctx.HomeTeam)
	p.AwayTeam = string(ctx.AwayTeam)
	if game.Venue != "" {
		p.Venue = game.Venue
	} else if ctx.Venue.Name != "" {
		p.Venue = ctx.Venue.Name
	}
	if !ctx.Kickoff.IsZero() {
		p.KickoffUTC = ctx.Kickoff.UTC().Format(time.RFC3339)
	}
	p.Model.Winner = string(report.Score.PredictedWinner)
	p.Model.WinProbPct = report.WinnerWinProbability() * 100
	p.Model.HomeScore = report.Score.HomeScore
	p.Model.AwayScore = report.Score.AwayScore
	p.Model.Margin = report.Score.Margin
	p.Model.Total = report.Score.TotalScore
	if report.TotalsLine != nil {
		p.Model.TotalsLine = report.TotalsLine.Line
		p.Model.TotalsBook = report.TotalsLine.Bookmaker
	}
	if mo, ok := report.MarketOdds[report.Score.PredictedWinner]; ok && mo.DecimalOdds > 1 {
		p.Model.MarketH2HOdds = mo.DecimalOdds
	}
	h, a := ctx.HomeStats, ctx.AwayStats
	p.Form.HomeWins10 = h.FormWinsLast10
	p.Form.HomeLosses10 = h.FormLossesLast10
	p.Form.AwayWins10 = a.FormWinsLast10
	p.Form.AwayLosses10 = a.FormLossesLast10
	return json.Marshal(p)
}

// ParsePregameLLMResponse decodes JSON from Gemini (strips optional markdown fences).
func ParsePregameLLMResponse(raw string) (PregameLLMResponse, error) {
	raw = strings.TrimSpace(raw)
	raw = stripJSONFence(raw)
	var out PregameLLMResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return PregameLLMResponse{}, fmt.Errorf("parse pregame llm json: %w", err)
	}
	if strings.TrimSpace(out.MainBet.Selection) == "" {
		return PregameLLMResponse{}, fmt.Errorf("parse pregame llm json: missing main_bet.selection")
	}
	return out, nil
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	start := 1
	end := len(lines) - 1
	if strings.TrimSpace(lines[end]) == "```" {
		end--
	}
	return strings.Join(lines[start:end+1], "\n")
}

// RunPregameLLM calls Gemini with retries and returns parsed JSON.
func RunPregameLLM(ctx context.Context, cfg config.AFLConfig, game SquiggleFixture, report MatchReport) (PregameLLMResponse, error) {
	if cfg.GeminiAPIKey == "" {
		return PregameLLMResponse{}, fmt.Errorf("gemini_api_key not configured")
	}
	payload, err := BuildPregameLLMPayload(game, report)
	if err != nil {
		return PregameLLMResponse{}, err
	}

	retries := cfg.ResolvedPregameLLMRetries()
	client := gemini.NewClient(cfg.GeminiAPIKey, cfg.ResolvedGeminiModel())

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			slog.Warn("pregame gemini retry", "attempt", attempt, "squiggle_id", game.ID)
		}
		text, err := client.GenerateGrounded(ctx, PregameLLMSystemPrompt, string(payload))
		if err != nil {
			lastErr = err
			continue
		}
		parsed, err := ParsePregameLLMResponse(text)
		if err != nil {
			lastErr = err
			continue
		}
		return parsed, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("gemini failed after %d retries", retries)
	}
	return PregameLLMResponse{}, fmt.Errorf("pregame gemini: %w", lastErr)
}
