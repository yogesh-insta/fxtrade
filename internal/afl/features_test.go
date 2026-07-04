package afl

import "testing"

func TestBuildHomeFeatureVectorLength(t *testing.T) {
	ctx := MatchDayContext{
		HomeTeam: "BRI",
		AwayTeam: "COLL",
		Venue: VenueProfile{
			ID: "GABBA", Name: "The Gabba", Dimension: VenueStandard,
			PrimaryHomeTeam: "BRI",
		},
		HomeStats: TeamStats{FormWinsLast10: 7, FormLossesLast10: 3, Inside50Efficiency: 0.55},
		AwayStats: TeamStats{FormWinsLast10: 5, FormLossesLast10: 5, Inside50Efficiency: 0.50},
		Weather:   WeatherMetrics{ContestFavorability: 0.2, TotalPointsFactor: 1.0},
	}
	fv := BuildHomeFeatureVector(ctx)
	if len(fv.Values) != FeatureCount {
		t.Fatalf("expected %d features, got %d", FeatureCount, len(fv.Values))
	}
	for i, v := range fv.Values {
		if v < -1.1 || v > 1.5 {
			t.Fatalf("feature %d out of expected range: %.3f", i, v)
		}
	}
}

func TestFeatureCountStable(t *testing.T) {
	if FeatureCount != 26 {
		t.Fatalf("FeatureCount changed — update model export and tests; got %d", FeatureCount)
	}
}
