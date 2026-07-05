package afl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/afl/gemini"
	"github.com/ym/fxtrade/internal/config"
)

func TestParsePregameLLMResponse_MarkdownFences(t *testing.T) {
	raw := `{"match_details":{"lineups":"confirmed"},"main_bet":{"market":"H2H","selection":"STK","confidence":"high","odds_note":"STK $1.65","reasons":["a","b"]},"model_agreement":"aligns"}`
	fenced := "```json\n" + raw + "\n```"
	resp, err := ParsePregameLLMResponse(fenced)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "STK" {
		t.Fatalf("selection = %q", resp.MainBet.Selection)
	}

	prose := "Here is the analysis:\n```json\n" + raw + "\n```\nGood luck."
	resp2, err := ParsePregameLLMResponse(prose)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.MainBet.Selection != "STK" {
		t.Fatal("prose-wrapped fence strip failed")
	}
}

func TestParsePregameLLMResponse_PartialJSON(t *testing.T) {
	partial := `{"match_details":{"weather_impact":"light wind"},"main_bet":{}}`
	resp, err := ParsePregameLLMResponse(partial)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Incomplete() || resp.IsEmpty() {
		t.Fatalf("expected incomplete non-empty partial: incomplete=%v empty=%v", resp.Incomplete(), resp.IsEmpty())
	}
	if resp.MatchDetails.WeatherImpact != "light wind" {
		t.Fatalf("weather = %q", resp.MatchDetails.WeatherImpact)
	}
}

func TestParsePregameLLMResponse_AlternateFieldNames(t *testing.T) {
	raw := `{
  "mainBet": {
    "pick": "STK H2H",
    "type": "H2H",
    "conf": "medium",
    "odds": "STK $1.70",
    "why": ["form edge", "venue"]
  },
  "modelAgreement": "mixed",
  "risks": "injury cloud",
  "matchDetails": {"lineupStatus": "unconfirmed — late team news pending"}
}`
	resp, err := ParsePregameLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "STK H2H" || resp.MainBet.Market != "H2H" {
		t.Fatalf("main bet = %+v", resp.MainBet)
	}
	if len(resp.MainBet.Reasons) != 2 {
		t.Fatalf("reasons = %#v", resp.MainBet.Reasons)
	}
	if resp.ModelAgreement != "mixed" || resp.RiskNote != "injury cloud" {
		t.Fatalf("agreement/risk = %q / %q", resp.ModelAgreement, resp.RiskNote)
	}
	if resp.MatchDetails.Lineups == "" {
		t.Fatal("expected lineups from alternate key")
	}
}

func TestParsePregameLLMResponse_EmptyMainBet(t *testing.T) {
	raw := `{"match_details":{"weather_impact":"calm"},"main_bet":{"market":"H2H","confidence":"medium","reasons":[]},"risk_note":"none"}`
	resp, err := ParsePregameLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.HasMainBet() {
		t.Fatal("empty selection must not count as main bet")
	}
	if !resp.Incomplete() {
		t.Fatal("expected incomplete when selection missing")
	}
	if resp.IsEmpty() {
		t.Fatal("partial weather/risk should not be fully empty")
	}

	game := SquiggleFixture{ID: 1}
	report := MatchReport{
		Context: MatchDayContext{HomeTeam: "PORT", AwayTeam: "NMFC"},
		Score:   ScoreProjection{PredictedWinner: "PORT", HomeScore: 90, AwayScore: 80, Margin: 10, TotalScore: 170},
	}
	body := FormatPregameEmail(game, report, resp, 45)
	if strings.Contains(body, "[]") {
		t.Fatalf("email must not print empty slice for empty main bet:\n%s", body)
	}
	if !strings.Contains(body, "Main bet: unavailable") {
		t.Fatalf("expected unavailable main bet:\n%s", body)
	}
}

func TestRunPregameLLM_Gemini403ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"PERMISSION_DENIED","code":403}}`))
	}))
	defer srv.Close()

	oldFactory := newGeminiClient
	newGeminiClient = func(apiKey, model string) *gemini.Client {
		return gemini.NewClient(apiKey, model).WithBaseURL(srv.URL + "/v1beta")
	}
	defer func() { newGeminiClient = oldFactory }()

	game := SquiggleFixture{ID: 7, HomeTeam: "Essendon", AwayTeam: "St Kilda"}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC),
		},
		Score: ScoreProjection{PredictedWinner: "STK", HomeScore: 80, AwayScore: 90, Margin: 10, TotalScore: 170},
	}
	cfg := config.AFLConfig{GeminiAPIKey: "test-key", PregameLLMRetries: 0}

	_, err := RunPregameLLM(context.Background(), cfg, game, report)
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 in error, got: %v", err)
	}
}

func TestRunPregameLLM_ValidJSONReturnsSelection(t *testing.T) {
	validJSON := `{"main_bet":{"market":"H2H","selection":"STK","confidence":"high","odds_note":"STK $1.65","reasons":["form"]},"model_agreement":"aligns"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]string{{"text": validJSON}},
				},
			}},
		})
	}))
	defer srv.Close()

	oldFactory := newGeminiClient
	newGeminiClient = func(apiKey, model string) *gemini.Client {
		return gemini.NewClient(apiKey, model).WithBaseURL(srv.URL + "/v1beta")
	}
	defer func() { newGeminiClient = oldFactory }()

	game := SquiggleFixture{ID: 8}
	report := MatchReport{
		Context: MatchDayContext{HomeTeam: "ESS", AwayTeam: "STK"},
		Score:   ScoreProjection{PredictedWinner: "STK", HomeScore: 80, AwayScore: 90, Margin: 10, TotalScore: 170},
	}
	cfg := config.AFLConfig{GeminiAPIKey: "test-key", PregameLLMRetries: 0}

	resp, err := RunPregameLLM(context.Background(), cfg, game, report)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "STK" {
		t.Fatalf("selection = %q", resp.MainBet.Selection)
	}

	body := FormatPregameEmail(game, report, resp, 45)
	if !strings.Contains(body, "Main bet: STK H2H (high)") {
		t.Fatalf("email missing selection:\n%s", body)
	}
}
