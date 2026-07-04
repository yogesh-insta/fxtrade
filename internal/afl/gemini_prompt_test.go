package afl

import (
	"strings"
	"testing"
)

func TestBuildGeminiAnalyticsUserPromptIncludesModelBaseline(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS",
			AwayTeam: "STK",
			Venue:    VenueProfile{Name: "Marvel Stadium"},
			HomeStats: TeamStats{FormWinsLast10: 0, FormLossesLast10: 10},
			AwayStats: TeamStats{FormWinsLast10: 4, FormLossesLast10: 6},
		},
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			HomeScore: 74, AwayScore: 82, TotalScore: 156, Margin: 8,
			PredictedWinner: "STK",
		},
		MarketOdds: map[TeamID]MarketOdds{
			"STK": {DecimalOdds: 1.20},
		},
	}
	prompt := BuildGeminiAnalyticsUserPrompt(17, report)
	for _, want := range []string{
		"Round 17",
		"Essendon vs St Kilda",
		"AFLPulse model baseline",
		"St Kilda (62%)",
		"Marvel Stadium",
		"0–10",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt:\n%s", want, prompt)
		}
	}
}

func TestFormatRoundReportEmailIncludesLLMAnalyticsAfterModel(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS",
			AwayTeam: "STK",
			Venue:    VenueProfile{Name: "Marvel Stadium"},
		},
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			HomeScore: 74, AwayScore: 82, TotalScore: 156, Margin: 8,
			PredictedWinner: "STK",
		},
		LLMAnalytics: "Subject Line: [AFL Round 17 Analytics] - Essendon vs St Kilda\nMain High-Confidence Bet Selection: St Kilda H2H",
	}
	body := FormatRoundReportEmail([]MatchReport{report}, nil)

	predIdx := strings.Index(body, "PREDICTION")
	llmIdx := strings.Index(body, "LIVE ANALYTICS (Gemini + Google Search)")
	if predIdx < 0 || llmIdx < predIdx {
		t.Fatalf("LLM section should follow model prediction:\n%s", body)
	}
	for _, want := range []string{
		"model prediction above unchanged",
		"Essendon vs St Kilda",
		"Main High-Confidence Bet Selection",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}
