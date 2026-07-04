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

func TestAwayTravelKMStKToMCG(t *testing.T) {
	stkBase := VenueProfile{
		ID: "DOCKLANDS", Name: "Marvel Stadium",
		Latitude: -37.8165, Longitude: 144.9475,
	}
	mcg := VenueProfile{
		ID: "MCG", Name: "Melbourne Cricket Ground",
		Latitude: -37.8199, Longitude: 144.9834,
	}
	km := AwayTravelKM(stkBase, mcg)
	if km >= MinMeaningfulTravelKM {
		t.Fatalf("STK→MCG should be intra-Melbourne, got %.1f km", km)
	}
	if km > 10 {
		t.Fatalf("STK→MCG expected <10 km, got %.1f km", km)
	}
	if MeaningfulTravel(km) {
		t.Fatalf("STK→MCG travel should not be meaningful, got %.1f km", km)
	}
	if TravelFatigue("STK", mcg, km) != 0 {
		t.Fatalf("expected zero travel fatigue for short intra-city trip")
	}
}

func TestAwayTravelKMInterstate(t *testing.T) {
	melBase := VenueProfile{ID: "MCG", Latitude: -37.8199, Longitude: 144.9834}
	gabba := VenueProfile{ID: "GABBA", Latitude: -27.4858, Longitude: 153.0381, Interstate: true}
	km := AwayTravelKM(melBase, gabba)
	if !MeaningfulTravel(km) {
		t.Fatalf("Melbourne→Brisbane should be meaningful travel, got %.0f km", km)
	}
	if km < 1000 {
		t.Fatalf("Melbourne→Brisbane expected >1000 km, got %.0f km", km)
	}
}

func TestMatchWeatherClosedVenueNeutralizesImpact(t *testing.T) {
	outdoor := WeatherMetrics{RainMM: 8, WindKPH: 30, ContestFavorability: 0.7, TotalPointsFactor: 0.82}
	closed := VenueProfile{ID: "DOCKLANDS", Dimension: VenueClosed}
	got := MatchWeather(closed, outdoor)
	if got.TotalPointsFactor != 1.0 {
		t.Fatalf("closed venue total factor = %.2f, want 1.0", got.TotalPointsFactor)
	}
	if got.ContestFavorability != 0.2 {
		t.Fatalf("closed venue contest favorability = %.2f, want 0.2", got.ContestFavorability)
	}
	if got.RainMM != outdoor.RainMM || got.WindKPH != outdoor.WindKPH {
		t.Fatal("raw rain/wind should be preserved for display")
	}
	open := VenueProfile{ID: "MCG", Dimension: VenueWide}
	if MatchWeather(open, outdoor) != outdoor {
		t.Fatal("open venue should pass weather through unchanged")
	}
}

func TestVenueStyleFitClosedUsesBalancedProfile(t *testing.T) {
	team := TeamStats{
		Inside50Efficiency:      0.50,
		ClearanceRate:           0.40,
		ContestedPossessionRate: 0.60,
		DisposalRate:            0.90,
	}
	wide := VenueStyleFit(team, VenueProfile{Dimension: VenueWide})
	closed := VenueStyleFit(team, VenueProfile{Dimension: VenueClosed})
	if closed >= wide {
		t.Fatalf("closed fit %.3f should be below wide disposal-heavy fit %.3f", closed, wide)
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
