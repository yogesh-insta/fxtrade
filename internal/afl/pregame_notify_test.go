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
	body := FormatPregameEmail(game, report, PregameLLMResponse{}, false)

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

	body := FormatPregameEmail(game, report, llm, false)
	for _, want := range []string{
		"T-30",
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
}

func TestFormatPregameEmail_ModelFallbackAfterGeminiFailure(t *testing.T) {
	game := SquiggleFixture{ID: 38636, Venue: "Adelaide Oval"}
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "PORT", AwayTeam: "NMFC",
			Kickoff: time.Date(2026, 7, 5, 6, 40, 0, 0, time.UTC),
			Venue:   VenueProfile{Name: "Adelaide Oval"},
			HomeStats: TeamStats{LadderPosition: 15},
			AwayStats: TeamStats{LadderPosition: 11},
		},
		HomeWinProb: 0.60,
		Score: ScoreProjection{
			PredictedWinner: "PORT", HomeScore: 83, AwayScore: 79, Margin: 4, TotalScore: 162,
		},
		MarketOdds: map[TeamID]MarketOdds{
			"PORT": {DecimalOdds: 1.72},
		},
	}
	fallback := BuildPregameModelFallback(report)
	merged, used := MergePregameLLMWithFallback(PregameLLMResponse{}, fallback)
	if !used || !merged.HasMainBet() {
		t.Fatalf("expected model fallback main bet: used=%v merged=%+v", used, merged)
	}

	body := FormatPregameEmail(game, report, merged, used)
	for _, want := range []string{
		"PORT vs NMFC",
		"MODEL BASELINE",
		"Port Adelaide (60%)",
		"LIVE ANALYTICS (Gemini)",
		"live search unavailable — main bet from AFLPulse model baseline",
		"Main bet: Port Adelaide H2H (medium)",
		"Why:",
		"Squiggle game ID: 38636",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Live analytics: unavailable") {
		t.Fatalf("model fallback should not show fully unavailable:\n%s", body)
	}
}

func TestPregameOrchestration_Gemini403(t *testing.T) {
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

	fallback := BuildPregameModelFallback(report)
	merged, used := MergePregameLLMWithFallback(PregameLLMResponse{}, fallback)
	body := FormatPregameEmail(game, report, merged, used)
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
	if !strings.Contains(body, "Main bet: St Kilda H2H") {
		t.Fatalf("model fallback main bet missing after 403:\n%s", body)
	}
	if !st.WasLLMAttempted(game.ID) {
		t.Fatal("LLM attempt should be recorded to avoid retry budget burn")
	}
}
