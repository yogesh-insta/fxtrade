package afl

import (
	"testing"
)

func TestProjectScoreHomeFavorite(t *testing.T) {
	ctx := MatchDayContext{
		HomeTeam: "RICH",
		AwayTeam: "CARL",
		Venue: VenueProfile{
			ID: "MCG", Name: "Melbourne Cricket Ground", Dimension: VenueWide,
			PrimaryHomeTeam: "MEL",
			HomeWinRates:    map[TeamID]float64{"RICH": 0.55},
		},
		Weather: WeatherMetrics{TotalPointsFactor: 1.0, ContestFavorability: 0.2},
		HomeStats: TeamStats{
			TeamID: "RICH", FormWinsLast10: 7, FormLossesLast10: 3,
			Inside50Efficiency: 0.59, ClearanceRate: 0.55, ContestedPossessionRate: 0.54, DisposalRate: 0.56,
		},
		AwayStats: TeamStats{
			TeamID: "CARL", FormWinsLast10: 5, FormLossesLast10: 5,
			Inside50Efficiency: 0.50, ClearanceRate: 0.53, ContestedPossessionRate: 0.51, DisposalRate: 0.52,
		},
	}

	proj := ProjectScore(ctx, 0.62)
	if proj.PredictedWinner != "RICH" {
		t.Fatalf("expected RICH to win, got %s", proj.PredictedWinner)
	}
	if proj.HomeScore <= proj.AwayScore {
		t.Fatalf("home favorite should score more: %d-%d", proj.HomeScore, proj.AwayScore)
	}
	if proj.TotalScore != proj.HomeScore+proj.AwayScore {
		t.Fatalf("total mismatch: %d vs %d+%d", proj.TotalScore, proj.HomeScore, proj.AwayScore)
	}
	if proj.Margin != proj.HomeScore-proj.AwayScore {
		t.Fatalf("margin mismatch: %d vs %d", proj.Margin, proj.HomeScore-proj.AwayScore)
	}
	if proj.TotalScore < 140 || proj.TotalScore > 210 {
		t.Fatalf("total out of realistic range: %d", proj.TotalScore)
	}
}

func TestProjectScoreWeatherLowersTotal(t *testing.T) {
	base := MatchDayContext{
		HomeTeam: "BRI", AwayTeam: "COLL",
		Venue:     VenueProfile{ID: "GABBA", Dimension: VenueStandard},
		HomeStats: TeamStats{FormWinsLast10: 6, FormLossesLast10: 4, Inside50Efficiency: 0.55, ClearanceRate: 0.55},
		AwayStats: TeamStats{FormWinsLast10: 6, FormLossesLast10: 4, Inside50Efficiency: 0.55, ClearanceRate: 0.55},
		Weather:   WeatherMetrics{TotalPointsFactor: 1.0},
	}
	wet := base
	wet.Weather = WeatherMetrics{RainMM: 12, WindKPH: 40, TotalPointsFactor: 0.7}

	dry := ProjectScore(base, 0.5)
	rain := ProjectScore(wet, 0.5)
	if rain.TotalScore >= dry.TotalScore {
		t.Fatalf("wet weather should lower total: dry=%d rain=%d", dry.TotalScore, rain.TotalScore)
	}
}

func TestProjectScoreRespectsMinScores(t *testing.T) {
	ctx := MatchDayContext{
		HomeTeam: "GCS", AwayTeam: "NMFC",
		Venue:     VenueProfile{ID: "GABBA", Dimension: VenueStandard},
		HomeStats: TeamStats{FormWinsLast10: 1, FormLossesLast10: 9, Inside50Efficiency: 0.3, ClearanceRate: 0.3},
		AwayStats: TeamStats{FormWinsLast10: 9, FormLossesLast10: 1, Inside50Efficiency: 0.9, ClearanceRate: 0.9},
		Weather:   WeatherMetrics{TotalPointsFactor: 0.6},
	}
	proj := ProjectScore(ctx, 0.15)
	if proj.HomeScore < 30 || proj.AwayScore < 30 {
		t.Fatalf("scores below minimum: %d-%d", proj.HomeScore, proj.AwayScore)
	}
}

func TestFixturesFromOddsDedup(t *testing.T) {
	odds := []MarketOdds{
		{EventID: "e1", HomeTeam: "RICH", AwayTeam: "CARL", Team: "RICH", Bookmaker: "a"},
		{EventID: "e1", HomeTeam: "RICH", AwayTeam: "CARL", Team: "CARL", Bookmaker: "a"},
		{EventID: "e2", HomeTeam: "BRI", AwayTeam: "COLL", Team: "BRI", Bookmaker: "a"},
	}
	fixtures := FixturesFromOdds(odds)
	if len(fixtures) != 2 {
		t.Fatalf("expected 2 fixtures, got %d", len(fixtures))
	}
}
