package afl

import (
	"testing"
)

func TestBuildPregameModelFallback_PORTNMFC(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "PORT", AwayTeam: "NMFC",
			HomeStats: TeamStats{FormWinsLast10: 3, FormLossesLast10: 7},
			AwayStats: TeamStats{FormWinsLast10: 5, FormLossesLast10: 5},
		},
		HomeWinProb: 0.60,
		Score: ScoreProjection{
			PredictedWinner: "PORT", HomeScore: 83, AwayScore: 79, Margin: 4, TotalScore: 162,
		},
		MarketOdds: map[TeamID]MarketOdds{
			"PORT": {DecimalOdds: 1.72},
		},
	}
	fb := BuildPregameModelFallback(report)
	if fb.MainBet.Selection != "Port Adelaide" {
		t.Fatalf("selection = %q", fb.MainBet.Selection)
	}
	if len(fb.MainBet.Reasons) == 0 {
		t.Fatal("expected model reasons")
	}
	if fb.MainBet.OddsNote != "PORT $1.72" {
		t.Fatalf("odds = %q", fb.MainBet.OddsNote)
	}
}

func TestMergePregameLLMWithFallback_PartialLiveData(t *testing.T) {
	live := PregameLLMResponse{}
	live.MatchDetails.WeatherImpact = "15 km/h wind, light rain"
	live.MatchDetails.Lineups = "confirmed"
	fallback := PregameLLMResponse{}
	fallback.MainBet.Selection = "Port Adelaide"
	fallback.MainBet.Market = "H2H"
	fallback.MainBet.Confidence = "medium"
	fallback.MainBet.Reasons = []string{"model form edge"}

	merged, used := MergePregameLLMWithFallback(live, fallback)
	if !used || !merged.HasMainBet() {
		t.Fatalf("merge failed: used=%v merged=%+v", used, merged)
	}
	if merged.MatchDetails.WeatherImpact != "15 km/h wind, light rain" {
		t.Fatal("live weather should be preserved")
	}
	if merged.MainBet.Selection != "Port Adelaide" {
		t.Fatalf("main bet = %q", merged.MainBet.Selection)
	}
	if merged.ModelAgreement != "mixed" {
		t.Fatalf("agreement = %q", merged.ModelAgreement)
	}
}

func TestParsePregameLLMResponse_NestedSelectionObject(t *testing.T) {
	raw := `{"main_bet":{"market":"H2H","selection":{"team":"Port Adelaide","pick":"H2H"},"confidence":"high","reasons":["a","b","c"]}}`
	resp, err := ParsePregameLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if resp.MainBet.Selection != "Port Adelaide H2H" {
		t.Fatalf("selection = %q", resp.MainBet.Selection)
	}
}
