package afl

import (
	"strings"
	"testing"
	"time"
)

func TestFormatPregameEmail_EmptyLLMResponse(t *testing.T) {
	game := SquiggleFixture{ID: 38629}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: time.Date(2026, 7, 5, 5, 15, 0, 0, time.UTC),
			Venue:   VenueProfile{Name: "Docklands"},
		},
		Score: ScoreProjection{PredictedWinner: "STK", HomeScore: 79, AwayScore: 88, Margin: 9, TotalScore: 167},
	}
	body := FormatPregameEmail(game, report, PregameLLMResponse{})

	if strings.Contains(body, "[]") {
		t.Fatalf("must not print [] for empty LLM:\n%s", body)
	}
	if !strings.Contains(body, "Gemini returned incomplete analysis") {
		t.Fatal("expected incomplete note")
	}
	if !strings.Contains(body, "Live analytics: unavailable") {
		t.Fatal("expected unavailable line")
	}
	if !strings.Contains(body, "MODEL BASELINE") || !strings.Contains(body, "St Kilda") {
		t.Fatalf("model baseline missing:\n%s", body)
	}
	if strings.Contains(body, "Main bet (medium confidence):") {
		t.Fatal("old empty main-bet format must not appear")
	}
}

func TestFormatPregameEmail_FullLLMResponse(t *testing.T) {
	game := SquiggleFixture{ID: 99, Venue: "M.C.G."}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC),
			Venue:   VenueProfile{Name: "Melbourne Cricket Ground"},
		},
		Score: ScoreProjection{PredictedWinner: "STK", HomeScore: 80, AwayScore: 90, Margin: 10, TotalScore: 170},
	}
	llm := PregameLLMResponse{}
	llm.MatchDetails.LateChanges = "No late changes"
	llm.MatchDetails.Lineups = "confirmed"
	llm.MainBet.Market = "H2H"
	llm.MainBet.Selection = "STK"
	llm.MainBet.Confidence = "high"
	llm.MainBet.OddsNote = "STK $1.65"
	llm.MainBet.Reasons = []string{"form", "lineups", "odds"}
	llm.PlayerProp.Player = "Nasiah Wanganeen-Milera"
	llm.PlayerProp.Market = "25+ disposals"
	llm.RiskNote = "Essendon midfield rotation"
	llm.ModelAgreement = "aligns"

	body := FormatPregameEmail(game, report, llm)
	for _, want := range []string{
		"T-45",
		"MODEL BASELINE",
		"LIVE ANALYTICS (Gemini)",
		"Main bet: STK H2H (high)",
		"Odds note: STK $1.65",
		"Why:",
		"form",
		"Player prop: Nasiah Wanganeen-Milera — 25+ disposals",
		"Risks: Essendon midfield rotation",
		"Model agreement: aligns",
		"Lineups: confirmed",
		"Squiggle game ID: 99",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
	if strings.Contains(body, "[]") {
		t.Fatalf("must not print empty slice: %s", body)
	}
}

func TestFormatPregameEmail_ModelOnlyAfterGeminiFailure(t *testing.T) {
	// Zero-value PregameLLMResponse after Gemini 403/parse failure — email must stay readable.
	game := SquiggleFixture{ID: 55, Venue: "Docklands"}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS", AwayTeam: "STK",
			Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC),
			Venue:   VenueProfile{Name: "Marvel Stadium"},
		},
		Score: ScoreProjection{PredictedWinner: "STK", HomeScore: 78, AwayScore: 92, Margin: 14, TotalScore: 170},
	}
	body := FormatPregameEmail(game, report, PregameLLMResponse{})

	if strings.Contains(body, "[]") {
		t.Fatalf("model-only email must not print Go zero values:\n%s", body)
	}
	for _, want := range []string{
		"MODEL BASELINE",
		"LIVE ANALYTICS (Gemini)",
		"Gemini returned incomplete analysis",
		"Live analytics: unavailable",
		"Gambling involves risk",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in model-only body:\n%s", want, body)
		}
	}
	for _, bad := range []string{
		"Main bet (medium confidence):",
		"Main bet:  ",
		"Why:\n    • ",
	} {
		if strings.Contains(body, bad) {
			t.Fatalf("unexpected broken section %q in:\n%s", bad, body)
		}
	}
}

func TestPregameOrchestration_Gemini403(t *testing.T) {
	// Table-driven simulation of cmd/afl-pulse-pregame when RunPregameLLM fails.
	game := SquiggleFixture{
		ID: 55, Kickoff: time.Date(2026, 7, 5, 8, 20, 0, 0, time.UTC), Venue: "Docklands",
	}
	report := MatchReport{
		Context: MatchDayContext{HomeTeam: "ESS", AwayTeam: "STK"},
		Score:   ScoreProjection{PredictedWinner: "STK", HomeScore: 80, AwayScore: 90, Margin: 10, TotalScore: 170},
	}
	llmErr := errTest("gemini status 403 Forbidden: PERMISSION_DENIED")

	st := &PregameState{Sent: make(map[string]time.Time), LLMAttempted: make(map[string]time.Time)}
	st.MarkLLMAttempted(game.ID)

	body := FormatPregameEmail(game, report, PregameLLMResponse{})
	alertBody := FormatPregameFailureAlert(game, "ESS", "STK", game.Venue, llmErr, "/opt/fxtrade/logs/afl-pulse-pregame.log")
	subject := FormatPregameFailureSubject("[AFLPulse PRE]", "ESS", "STK")

	if strings.Contains(body, "[]") {
		t.Fatalf("pregame body must not contain [] after 403:\n%s", body)
	}
	if subject != "[AFLPulse PRE] ALERT · Gemini failed · ESS vs STK" {
		t.Fatalf("failure subject = %q", subject)
	}
	for _, want := range []string{"Gemini failed", "403", "PERMISSION_DENIED", "Squiggle ID: 55"} {
		if !strings.Contains(alertBody, want) {
			t.Fatalf("failure alert missing %q:\n%s", want, alertBody)
		}
	}
	if !strings.Contains(body, "MODEL BASELINE") {
		t.Fatal("model baseline must still render after Gemini failure")
	}
	if st.WasSent(game.ID) {
		t.Fatal("MarkSent must not run before email send completes")
	}
	st.MarkSent(game.ID)
	if !st.WasSent(game.ID) {
		t.Fatal("MarkSent after successful model-only email")
	}
	if !st.WasLLMAttempted(game.ID) {
		t.Fatal("LLM attempt should be recorded to avoid retry budget burn")
	}
}
