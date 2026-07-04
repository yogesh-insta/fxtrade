package afl

import (
	"testing"
)

func TestTravelFatigueIncreasesWithDistance(t *testing.T) {
	venue := VenueProfile{Interstate: true}
	low := TravelFatigue("MEL", venue, 500)
	high := TravelFatigue("MEL", venue, 3000)
	if high <= low {
		t.Fatalf("expected higher fatigue for longer travel: low=%.3f high=%.3f", low, high)
	}
	if high > 1 || low < 0 {
		t.Fatalf("fatigue out of range: low=%.3f high=%.3f", low, high)
	}
}

func TestCrowdBiasHomeVenue(t *testing.T) {
	venue := VenueProfile{
		PrimaryHomeTeam: "GEE",
		HomeWinRates:    map[TeamID]float64{"GEE": 0.68},
		Interstate:      false,
	}
	bias := CrowdBias("GEE", "COLL", venue)
	if bias < 1.0 {
		t.Fatalf("expected home bias >= 1.0, got %.3f", bias)
	}
}

func TestWeatherDownWeightsDisposalTeam(t *testing.T) {
	weather := WeatherMetrics{RainMM: 10, WindKPH: 35, TotalPointsFactor: 0.75}
	disposalHeavy := TeamStats{DisposalRate: 0.9, ContestedPossessionRate: 0.3}
	contestHeavy := TeamStats{DisposalRate: 0.3, ContestedPossessionRate: 0.9}
	dAdj := WeatherProfileAdjust(weather, disposalHeavy)
	cAdj := WeatherProfileAdjust(weather, contestHeavy)
	if dAdj.TeamStrengthFactor >= cAdj.TeamStrengthFactor {
		t.Fatalf("disposal team should be down-weighted more in wet weather: d=%.3f c=%.3f",
			dAdj.TeamStrengthFactor, cAdj.TeamStrengthFactor)
	}
}

func TestPlayerAvailabilityAdjust(t *testing.T) {
	impacts := []PlayerImpact{
		{Role: RoleMidfielder, ImpactScore: 0.9, Available: false},
		{Role: RoleKeyForward, ImpactScore: 0.8, Available: false},
	}
	strength := PlayerAvailabilityAdjust(impacts)
	if strength >= 1.0 {
		t.Fatalf("expected reduced strength, got %.3f", strength)
	}
}

func TestComputeEV(t *testing.T) {
	ev := ComputeEV(0.55, 2.0)
	if ev < 0.099 || ev > 0.101 {
		t.Fatalf("expected EV ~0.10, got %.4f", ev)
	}
	if ComputeEV(0.40, 2.0) > 0 {
		t.Fatal("expected negative EV")
	}
}

func TestFormScore(t *testing.T) {
	if FormScore(7, 3) != 0.7 {
		t.Fatal("form score mismatch")
	}
	if FormScore(0, 0) != 0.5 {
		t.Fatal("empty form should be 0.5")
	}
}
