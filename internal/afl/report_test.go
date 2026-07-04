package afl

import (
	"strings"
	"testing"
	"time"
)

func TestFormatMatchPredictionsRichCarl(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "RICH",
			AwayTeam: "CARL",
			Kickoff:  time.Date(2025, 4, 17, 19, 30, 0, 0, time.UTC),
		},
		HomeWinProb: 0.765,
		Score: ScoreProjection{
			HomeScore: 84, AwayScore: 72, TotalScore: 156, Margin: 12,
			PredictedWinner: "RICH", HomeWinProb: 0.765,
		},
	}

	body := FormatMatchPredictions(report)
	for _, want := range []string{
		"PREDICTION",
		"Winner:     Richmond (76%)",
		"Score:      Richmond 84 – Carlton 72  (margin 12)",
		"Total:      156 points",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in predictions:\n%s", want, body)
		}
	}

	block := FormatFixturePredictionBlock(report)
	for _, want := range []string{"RICH vs CARL", "PREDICTION", "CONTEXT", "────────────────"} {
		if !strings.Contains(block, want) {
			t.Fatalf("missing %q in block:\n%s", want, block)
		}
	}
}

func TestBestMarketOddsByTeam(t *testing.T) {
	h2h := []MarketOdds{
		{EventID: "e1", Team: "RICH", DecimalOdds: 1.55, Bookmaker: "a"},
		{EventID: "e1", Team: "RICH", DecimalOdds: 1.60, Bookmaker: "b"},
		{EventID: "e1", Team: "CARL", DecimalOdds: 2.40, Bookmaker: "a"},
		{EventID: "e2", Team: "RICH", DecimalOdds: 1.50, Bookmaker: "a"},
	}
	got := BestMarketOddsByTeam(h2h, "e1")
	if got["RICH"].DecimalOdds != 1.60 {
		t.Fatalf("expected best RICH odds 1.60, got %.2f", got["RICH"].DecimalOdds)
	}
	if got["CARL"].DecimalOdds != 2.40 {
		t.Fatalf("expected CARL odds 2.40, got %.2f", got["CARL"].DecimalOdds)
	}
}
