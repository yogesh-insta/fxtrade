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

// newGeminiClient builds the Gemini client for pregame LLM calls (overridable in tests).
var newGeminiClient = func(apiKey, model string) *gemini.Client {
	return gemini.NewClient(apiKey, model)
}

// PregameLLMSystemPrompt instructs Gemini to return structured JSON for T-45 emails.
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
    "weather_impact": "string — wind/rain and scoring/handling impact",
    "lineups": "confirmed | unconfirmed — plus brief note"
  },
  "main_bet": {
    "market": "H2H | Line | Total",
    "selection": "string — e.g. STK H2H or Over 168.5",
    "confidence": "high | medium",
    "odds_note": "string — live odds snapshot e.g. STK $1.65",
    "reasons": ["reason 1", "reason 2", "reason 3"]
  },
  "player_prop": {
    "player": "string",
    "market": "string — e.g. 25+ disposals",
    "reasons": ["reason 1", "reason 2"]
  },
  "risk_note": "string — structural anomalies or injury vulnerabilities",
  "model_agreement": "aligns | mixed | contradicts",
  "disclaimer": "Gambling involves risk. Bet responsibly."
}

Ground every field in live search results. Use the model baseline only as a cross-check.`

// PregameLLMResponse is the structured Gemini output for pregame emails.
type PregameLLMResponse struct {
	MatchDetails struct {
		LateChanges   string `json:"late_changes"`
		WeatherImpact string `json:"weather_impact"`
		Lineups       string `json:"lineups"`
	} `json:"match_details"`
	MainBet struct {
		Market     string   `json:"market"`
		Selection  string   `json:"selection"`
		Confidence string   `json:"confidence"`
		OddsNote   string   `json:"odds_note"`
		Reasons    []string `json:"reasons"`
	} `json:"main_bet"`
	PlayerProp struct {
		Player  string   `json:"player"`
		Market  string   `json:"market"`
		Reasons []string `json:"reasons"`
	} `json:"player_prop"`
	RiskNote        string `json:"risk_note"`
	ModelAgreement  string `json:"model_agreement"`
	Disclaimer      string `json:"disclaimer"`
}

// HasMainBet reports whether Gemini returned a usable main selection.
func (r PregameLLMResponse) HasMainBet() bool {
	return strings.TrimSpace(r.MainBet.Selection) != ""
}

// IsEmpty reports whether the response has no live-analytics content.
func (r PregameLLMResponse) IsEmpty() bool {
	if r.HasMainBet() {
		return false
	}
	if strings.TrimSpace(r.MatchDetails.LateChanges) != "" ||
		strings.TrimSpace(r.MatchDetails.WeatherImpact) != "" ||
		strings.TrimSpace(r.MatchDetails.Lineups) != "" {
		return false
	}
	if strings.TrimSpace(r.PlayerProp.Player) != "" ||
		strings.TrimSpace(r.RiskNote) != "" ||
		strings.TrimSpace(r.ModelAgreement) != "" ||
		strings.TrimSpace(r.MainBet.OddsNote) != "" {
		return false
	}
	return true
}

// Incomplete reports partial or missing live analytics (no main bet selection).
func (r PregameLLMResponse) Incomplete() bool {
	return !r.HasMainBet()
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
		Winner        string  `json:"predicted_winner"`
		WinProbPct    float64 `json:"win_prob_pct"`
		HomeScore     int     `json:"home_score"`
		AwayScore     int     `json:"away_score"`
		Margin        int     `json:"margin"`
		Total         int     `json:"total_points"`
		TotalsLine    float64 `json:"totals_line,omitempty"`
		TotalsBook    string  `json:"totals_book,omitempty"`
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

// ParsePregameLLMResponse decodes JSON from Gemini (strips markdown fences, maps alternate keys).
// Partial responses are accepted; callers should check Incomplete().
func ParsePregameLLMResponse(raw string) (PregameLLMResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return PregameLLMResponse{}, fmt.Errorf("parse pregame llm json: empty response")
	}
	payload, err := extractJSONObject(raw)
	if err != nil {
		return PregameLLMResponse{}, fmt.Errorf("parse pregame llm json: %w", err)
	}

	var out PregameLLMResponse
	if err := json.Unmarshal(payload, &out); err != nil {
		return PregameLLMResponse{}, fmt.Errorf("parse pregame llm json: %w", err)
	}
	normalizePregameLLM(&out, payload)
	return out, nil
}

// extractJSONObject pulls the first JSON object from text, tolerating markdown fences and prose.
func extractJSONObject(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = stripJSONFence(s)
	s = strings.TrimSpace(s)

	if json.Valid([]byte(s)) && strings.HasPrefix(s, "{") {
		return []byte(s), nil
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object found")
	}
	candidate := strings.TrimSpace(s[start : end+1])
	if !json.Valid([]byte(candidate)) {
		return nil, fmt.Errorf("invalid JSON object")
	}
	return []byte(candidate), nil
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
	// Drop opening ``` or ```json line.
	start := 1
	end := len(lines) - 1
	// Find closing fence (may not be the last line if trailing prose exists).
	for i := len(lines) - 1; i > start; i-- {
		if strings.TrimSpace(lines[i]) == "```" {
			end = i - 1
			break
		}
	}
	return strings.Join(lines[start:end+1], "\n")
}

// normalizePregameLLM fills canonical fields from alternate Gemini key names.
func normalizePregameLLM(out *PregameLLMResponse, payload []byte) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(payload, &root); err != nil {
		return
	}

	if details := firstObject(root, "match_details", "matchDetails", "details"); details != nil {
		if out.MatchDetails.LateChanges == "" {
			out.MatchDetails.LateChanges = firstString(details, "late_changes", "lateChanges", "late_change", "changes")
		}
		if out.MatchDetails.WeatherImpact == "" {
			out.MatchDetails.WeatherImpact = firstString(details, "weather_impact", "weatherImpact", "weather")
		}
		if out.MatchDetails.Lineups == "" {
			out.MatchDetails.Lineups = firstString(details, "lineups", "line_ups", "lineup_status", "lineupStatus")
		}
	}
	if out.MatchDetails.Lineups == "" {
		out.MatchDetails.Lineups = firstString(root, "lineups", "line_ups", "lineup_status")
	}

	if bet := firstObject(root, "main_bet", "mainBet", "bet", "primary_bet"); bet != nil {
		if out.MainBet.Market == "" {
			out.MainBet.Market = firstString(bet, "market", "type", "bet_type", "betType")
		}
		if out.MainBet.Selection == "" {
			out.MainBet.Selection = firstString(bet, "selection", "pick", "side", "team", "bet")
		}
		if out.MainBet.Confidence == "" {
			out.MainBet.Confidence = firstString(bet, "confidence", "conf")
		}
		if out.MainBet.OddsNote == "" {
			out.MainBet.OddsNote = firstString(bet, "odds_note", "oddsNote", "odds", "price", "odds_snapshot")
		}
		if len(out.MainBet.Reasons) == 0 {
			out.MainBet.Reasons = firstStringSlice(bet, "reasons", "rationale", "why", "bullets")
		}
	}

	if prop := firstObject(root, "player_prop", "playerProp", "prop"); prop != nil {
		if out.PlayerProp.Player == "" {
			out.PlayerProp.Player = firstString(prop, "player", "name")
		}
		if out.PlayerProp.Market == "" {
			out.PlayerProp.Market = firstString(prop, "market", "prop", "line")
		}
		if len(out.PlayerProp.Reasons) == 0 {
			out.PlayerProp.Reasons = firstStringSlice(prop, "reasons", "rationale", "why")
		}
	}

	if out.RiskNote == "" {
		out.RiskNote = firstString(root, "risk_note", "riskNote", "risks", "risk")
	}
	if out.ModelAgreement == "" {
		out.ModelAgreement = firstString(root, "model_agreement", "modelAgreement", "agreement", "vs_model")
	}
	if out.Disclaimer == "" {
		out.Disclaimer = firstString(root, "disclaimer")
	}

	out.MainBet.Market = strings.TrimSpace(out.MainBet.Market)
	out.MainBet.Selection = strings.TrimSpace(out.MainBet.Selection)
	out.MainBet.Confidence = strings.TrimSpace(out.MainBet.Confidence)
	out.MainBet.OddsNote = strings.TrimSpace(out.MainBet.OddsNote)
	out.MainBet.Reasons = trimNonEmpty(out.MainBet.Reasons)
	out.PlayerProp.Player = strings.TrimSpace(out.PlayerProp.Player)
	out.PlayerProp.Market = strings.TrimSpace(out.PlayerProp.Market)
	out.PlayerProp.Reasons = trimNonEmpty(out.PlayerProp.Reasons)
	out.MatchDetails.LateChanges = strings.TrimSpace(out.MatchDetails.LateChanges)
	out.MatchDetails.WeatherImpact = strings.TrimSpace(out.MatchDetails.WeatherImpact)
	out.MatchDetails.Lineups = strings.TrimSpace(out.MatchDetails.Lineups)
	out.RiskNote = strings.TrimSpace(out.RiskNote)
	out.ModelAgreement = strings.TrimSpace(out.ModelAgreement)
	out.Disclaimer = strings.TrimSpace(out.Disclaimer)
}

func firstObject(root map[string]json.RawMessage, keys ...string) map[string]json.RawMessage {
	for _, k := range keys {
		raw, ok := root[k]
		if !ok {
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err == nil {
			return obj
		}
	}
	return nil
}

func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if t := strings.TrimSpace(s); t != "" {
				return t
			}
			continue
		}
		// Accept non-string scalars as text (e.g. numbers).
		var v any
		if err := json.Unmarshal(raw, &v); err == nil && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

func firstStringSlice(m map[string]json.RawMessage, keys ...string) []string {
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		var ss []string
		if err := json.Unmarshal(raw, &ss); err == nil {
			return trimNonEmpty(ss)
		}
		// Single string instead of array.
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if t := strings.TrimSpace(s); t != "" {
				return []string{t}
			}
		}
	}
	return nil
}

func trimNonEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// RunPregameLLM calls Gemini with retries and returns parsed JSON.
// Partial JSON is returned (not an error) so callers can still email the model baseline.
func RunPregameLLM(ctx context.Context, cfg config.AFLConfig, game SquiggleFixture, report MatchReport) (PregameLLMResponse, error) {
	if cfg.GeminiAPIKey == "" {
		return PregameLLMResponse{}, fmt.Errorf("gemini_api_key not configured")
	}
	payload, err := BuildPregameLLMPayload(game, report)
	if err != nil {
		return PregameLLMResponse{}, err
	}

	retries := cfg.ResolvedPregameLLMRetries()
	client := newGeminiClient(cfg.GeminiAPIKey, cfg.ResolvedGeminiModel())

	var lastErr error
	var lastPartial PregameLLMResponse
	var hadPartial bool
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
			snippet := text
			if len(snippet) > 400 {
				snippet = snippet[:400] + "…"
			}
			slog.Warn("pregame gemini parse failed",
				"squiggle_id", game.ID,
				"attempt", attempt,
				"error", err,
				"raw_snippet", snippet,
			)
			lastErr = err
			continue
		}
		if parsed.HasMainBet() {
			return parsed, nil
		}
		// Keep partial content; retry in case a later attempt is complete.
		lastPartial = parsed
		hadPartial = true
		lastErr = fmt.Errorf("incomplete main_bet.selection")
		slog.Warn("pregame gemini incomplete analysis",
			"squiggle_id", game.ID,
			"attempt", attempt,
		)
	}
	if hadPartial {
		return lastPartial, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("gemini failed after %d retries", retries)
	}
	return PregameLLMResponse{}, fmt.Errorf("pregame gemini: %w", lastErr)
}
