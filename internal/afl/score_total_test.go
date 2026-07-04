package afl

import (
	"testing"
)

func TestProjectTotalFromScoringRates(t *testing.T) {
	ctx := MatchDayContext{
		HomeTeam: "RICH",
		AwayTeam: "CARL",
		Venue: VenueProfile{
			ID: "MCG", Name: "Melbourne Cricket Ground", Dimension: VenueWide,
			AvgTotalScore: 168,
		},
		Weather: WeatherMetrics{TotalPointsFactor: 1.0},
		HomeStats: TeamStats{
			PointsForPerGame: 65, PointsAgainstPerGame: 107,
		},
		AwayStats: TeamStats{
			PointsForPerGame: 81, PointsAgainstPerGame: 89,
		},
	}

	bd := ProjectTotal(ctx)
	// home exp = (65+89)/2 = 77, away = (81+107)/2 = 94, team = 171
	// blended = 0.6*171 + 0.4*168 = 169.8
	if bd.TeamBasedTotal < 165 || bd.TeamBasedTotal > 175 {
		t.Fatalf("unexpected team total %.1f", bd.TeamBasedTotal)
	}
	if bd.AdjustedTotal >= 170 {
		t.Fatalf("low-scoring richmond profile should keep total modest, got %.1f", bd.AdjustedTotal)
	}
	if bd.HomeExpected != 77 || bd.AwayExpected != 94 {
		t.Fatalf("unexpected expectations: %.0f %.0f", bd.HomeExpected, bd.AwayExpected)
	}
}

func TestProjectTotalClosedVenueIgnoresWeather(t *testing.T) {
	wet := MatchDayContext{
		HomeTeam: "ESS", AwayTeam: "STK",
		Venue: VenueProfile{
			ID: "DOCKLANDS", Name: "Marvel Stadium", Dimension: VenueClosed,
			AvgTotalScore: 179,
		},
		HomeStats: TeamStats{PointsForPerGame: 82, PointsAgainstPerGame: 88},
		AwayStats: TeamStats{PointsForPerGame: 86, PointsAgainstPerGame: 84},
		Weather:   WeatherMetrics{RainMM: 8, WindKPH: 30, TotalPointsFactor: 0.94},
	}
	openWet := wet
	openWet.Venue.Dimension = VenueWide

	closed := ProjectTotal(wet)
	open := ProjectTotal(openWet)
	if closed.WeatherFactor != 1.0 {
		t.Fatalf("closed venue weather factor = %.2f, want 1.0", closed.WeatherFactor)
	}
	if open.WeatherFactor >= 1.0 {
		t.Fatalf("open wet venue should reduce total, factor=%.2f", open.WeatherFactor)
	}
	if closed.AdjustedTotal <= open.AdjustedTotal {
		t.Fatalf("closed should score higher than open-wet: closed=%.0f open=%.0f",
			closed.AdjustedTotal, open.AdjustedTotal)
	}
}

func TestProjectTotalWeatherLowers(t *testing.T) {
	base := MatchDayContext{
		HomeTeam: "BRI", AwayTeam: "COLL",
		Venue:     VenueProfile{ID: "GABBA", AvgTotalScore: 170},
		HomeStats: TeamStats{PointsForPerGame: 90, PointsAgainstPerGame: 85},
		AwayStats: TeamStats{PointsForPerGame: 88, PointsAgainstPerGame: 87},
		Weather:   WeatherMetrics{TotalPointsFactor: 1.0},
	}
	wet := base
	wet.Weather = WeatherMetrics{RainMM: 12, WindKPH: 40, TotalPointsFactor: 0.7}

	dry := ProjectTotal(base)
	rain := ProjectTotal(wet)
	if rain.AdjustedTotal >= dry.AdjustedTotal {
		t.Fatalf("wet weather should lower total: dry=%.0f rain=%.0f", dry.AdjustedTotal, rain.AdjustedTotal)
	}
}

func TestProjectScoreDecouplesTotalAndMargin(t *testing.T) {
	ctx := MatchDayContext{
		HomeTeam: "RICH",
		AwayTeam: "CARL",
		Venue: VenueProfile{
			ID: "MCG", Dimension: VenueWide, AvgTotalScore: 168,
		},
		Weather: WeatherMetrics{TotalPointsFactor: 1.0},
		HomeStats: TeamStats{
			FormWinsLast10: 2, FormLossesLast10: 8,
			Inside50Efficiency: 0.38, ClearanceRate: 0.40,
			PointsForPerGame: 65, PointsAgainstPerGame: 107,
		},
		AwayStats: TeamStats{
			FormWinsLast10: 6, FormLossesLast10: 4,
			Inside50Efficiency: 0.55, ClearanceRate: 0.53,
			PointsForPerGame: 81, PointsAgainstPerGame: 89,
		},
	}

	lowProb := ProjectScore(ctx, 0.35)  // away favorite
	highProb := ProjectScore(ctx, 0.75) // home favorite

	diff := abs(float64(lowProb.TotalScore - highProb.TotalScore))
	if diff > 8 {
		t.Fatalf("total should not swing much with win prob: %d vs %d", lowProb.TotalScore, highProb.TotalScore)
	}
	if lowProb.Margin == highProb.Margin && lowProb.PredictedWinner == highProb.PredictedWinner {
		t.Fatal("margin/winner should still respond to win probability")
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
