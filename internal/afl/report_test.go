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
		"Winner: RICH (76% probability)",
		"Total score: 156 points",
		"Team scores: RICH 84 – CARL 72",
		"Winning margin: 12 points (RICH)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in predictions:\n%s", want, body)
		}
	}

	block := FormatFixturePredictionBlock(report)
	if !strings.Contains(block, "RICH vs CARL") {
		t.Fatalf("missing fixture header in block:\n%s", block)
	}
}
