package afl

import (
	"context"
	"testing"
	"time"
)

type stubStatsSource struct {
	matchVenue VenueProfile
	hasMatch   bool
	defaultVen VenueProfile
}

func (s stubStatsSource) ResolveTeam(string) (TeamID, error) { return "", nil }
func (s stubStatsSource) Team(TeamID) (TeamStats, bool)      { return TeamStats{}, false }
func (s stubStatsSource) Venue(VenueID) (VenueProfile, bool) {
	if s.hasMatch {
		return s.matchVenue, true
	}
	return s.defaultVen, true
}
func (s stubStatsSource) MatchVenue(home, away TeamID) (VenueProfile, bool) {
	if s.hasMatch && home == "ESS" && away == "STK" {
		return s.matchVenue, true
	}
	return VenueProfile{}, false
}
func (s stubStatsSource) DefaultVenueForTeam(TeamID) VenueProfile { return s.defaultVen }
func (s stubStatsSource) PlayerMatrix(TeamID, TeamID, string) PlayerAvailabilityMatrix {
	return PlayerAvailabilityMatrix{}
}
func (s stubStatsSource) HeadToHead(TeamID, TeamID) (int, int) { return 0, 0 }

type noopWeather struct{}

func (noopWeather) FetchForVenue(context.Context, VenueProfile, time.Time) (WeatherMetrics, error) {
	return WeatherMetrics{ContestFavorability: 0.2, TotalPointsFactor: 1.0}, nil
}

func TestRepositoryContextBuilderUsesFixtureVenue(t *testing.T) {
	mcg := VenueProfile{ID: "MCG", Name: "Melbourne Cricket Ground", Dimension: VenueWide}
	dock := VenueProfile{ID: "DOCKLANDS", Name: "Marvel Stadium", Dimension: VenueClosed}
	builder := &RepositoryContextBuilder{
		Repo: stubStatsSource{
			defaultVen: mcg,
			matchVenue: dock,
			hasMatch:   true,
		},
		Weather: noopWeather{},
	}
	ctx, err := builder.Build(context.Background(), MarketOdds{
		EventID: "ev1", HomeTeam: "ESS", AwayTeam: "STK", Kickoff: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Venue.ID != "DOCKLANDS" {
		t.Fatalf("venue = %s, want DOCKLANDS (Marvel Stadium)", ctx.Venue.ID)
	}
}
