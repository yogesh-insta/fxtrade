package afl

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMatchPredictionReasonsNoTravelForIntraMelbourne(t *testing.T) {
	r := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "ESS",
			AwayTeam: "STK",
			Venue: VenueProfile{
				Name: "Melbourne Cricket Ground", Dimension: VenueWide,
				Latitude: -37.8199, Longitude: 144.9834,
				AvgTotalScore: 168, PrimaryHomeTeam: "MEL",
			},
			TravelKM: 3,
			Weather:  WeatherMetrics{RainMM: 0.5, WindKPH: 15, TotalPointsFactor: 0.95},
			HomeStats: TeamStats{
				FormWinsLast10: 0, FormLossesLast10: 10,
				PointsForPerGame: 73, PointsAgainstPerGame: 102,
				RecentPointsForPerGame: 59, ScoringTrendFor: -14,
			},
			AwayStats: TeamStats{
				FormWinsLast10: 4, FormLossesLast10: 6,
				PointsForPerGame: 89, PointsAgainstPerGame: 88,
				RecentPointsForPerGame: 80, ScoringTrendFor: -9,
			},
		},
		HomeWinProb: 0.38,
		Score: ScoreProjection{
			HomeScore: 74, AwayScore: 82, TotalScore: 156, Margin: 8,
			PredictedWinner: "STK",
			Breakdown: ScoreBreakdown{
				HomeExpected: 80, AwayExpected: 85, TeamBasedTotal: 165,
				WeatherFactor: 0.93, AdjustedTotal: 156, RawMargin: -8.4,
				ProfileEdge: -0.25, WinEdge: -0.23,
			},
		},
		TotalsLine: &TotalsOdds{Line: 168.5, Bookmaker: "sportsbet"},
	}
	reasons := BuildMatchPredictionReasons(r)
	if len(reasons) < 4 {
		t.Fatalf("expected several reasons, got %d: %v", len(reasons), reasons)
	}
	joined := strings.Join(reasons, "\n")
	for _, want := range []string{"STK", "Form edge", "Scoring slump", "UNDER", "Margin"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Away travel") {
		t.Fatalf("intra-Melbourne fixture should not mention travel:\n%s", joined)
	}
}

func TestFormatRoundReportEmailIncludesReasons(t *testing.T) {
	report := MatchReport{
		Context: MatchDayContext{
			HomeTeam: "RICH",
			AwayTeam: "CARL",
			Venue:    VenueProfile{Name: "Melbourne Cricket Ground", Dimension: VenueWide, AvgTotalScore: 170},
			Weather:  WeatherMetrics{RainMM: 0, WindKPH: 12, TotalPointsFactor: 1.0},
			Kickoff:  timeNow(),
			HomeStats: TeamStats{
				FormWinsLast10: 2, FormLossesLast10: 8,
				PointsForPerGame: 66, RecentPointsForPerGame: 62, ScoringTrendFor: -4,
			},
			AwayStats: TeamStats{
				FormWinsLast10: 6, FormLossesLast10: 4,
				PointsForPerGame: 84, RecentPointsForPerGame: 78, ScoringTrendFor: -6,
			},
		},
		HomeWinProb: 0.32,
		Score: ScoreProjection{
			HomeScore: 74, AwayScore: 82, TotalScore: 156, Margin: 8,
			PredictedWinner: "CARL",
			Breakdown: ScoreBreakdown{
				HomeExpected: 77, AwayExpected: 79, TeamBasedTotal: 156,
				WeatherFactor: 0.97, AdjustedTotal: 151, RawMargin: -10,
			},
		},
		TotalsLine: &TotalsOdds{Line: 168.5, Bookmaker: "sportsbet"},
	}
	body := FormatRoundReportEmail([]MatchReport{report}, nil)
	if !strings.Contains(body, "WHY\n") {
		t.Fatalf("missing reasoning section:\n%s", body)
	}
}

func timeNow() time.Time {
	return time.Date(2025, 4, 17, 19, 30, 0, 0, time.UTC)
}
