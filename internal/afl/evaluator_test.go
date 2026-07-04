package afl

import (
	"context"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

type stubPredictor struct {
	homeProb float64
}

func (p stubPredictor) Predict(fv FeatureVector) (float64, error) {
	return p.homeProb, nil
}

type stubBuilder struct {
	ctx MatchDayContext
}

func (b stubBuilder) Build(ctx context.Context, odds MarketOdds) (MatchDayContext, error) {
	_ = ctx
	c := b.ctx
	c.EventID = odds.EventID
	c.HomeTeam = odds.HomeTeam
	c.AwayTeam = odds.AwayTeam
	c.Kickoff = odds.Kickoff
	return c, nil
}

func testAFLConfig() config.AFLConfig {
	return config.AFLConfig{
		MinEVThreshold:    0.05,
		Concurrency:       2,
		MaxOddsAgeMinutes: 60,
	}
}

func TestEvaluatorFindsValueBet(t *testing.T) {
	cfg := testAFLConfig()

	matchCtx := MatchDayContext{
		Venue:     VenueProfile{ID: "GABBA", Name: "Gabba", Dimension: VenueStandard, PrimaryHomeTeam: "BRI"},
		HomeStats: TeamStats{FormWinsLast10: 8, FormLossesLast10: 2},
		AwayStats: TeamStats{FormWinsLast10: 3, FormLossesLast10: 7},
		Weather:   WeatherMetrics{ContestFavorability: 0.2, TotalPointsFactor: 1.0},
	}

	eval := NewEvaluator(cfg, stubPredictor{homeProb: 0.62}, stubBuilder{ctx: matchCtx})
	now := time.Now().UTC()
	odds := []MarketOdds{
		{
			EventID: "ev1", HomeTeam: "BRI", AwayTeam: "COLL", Team: "BRI",
			Bookmaker: "sportsbet", DecimalOdds: 2.10, UpdatedAt: now, Kickoff: now.Add(24 * time.Hour),
		},
	}

	bets, err := eval.Evaluate(context.Background(), odds)
	if err != nil {
		t.Fatal(err)
	}
	if len(bets) != 1 {
		t.Fatalf("expected 1 value bet, got %d", len(bets))
	}
	if bets[0].EV <= cfg.MinEVThreshold {
		t.Fatalf("expected EV above threshold, got %.4f", bets[0].EV)
	}
}

func TestEvaluatorRejectsLowEV(t *testing.T) {
	cfg := testAFLConfig()
	eval := NewEvaluator(cfg, stubPredictor{homeProb: 0.45}, stubBuilder{ctx: MatchDayContext{
		Venue: VenueProfile{ID: "MCG", Dimension: VenueWide},
	}})
	now := time.Now().UTC()
	odds := []MarketOdds{{
		EventID: "ev1", HomeTeam: "MEL", AwayTeam: "COLL", Team: "MEL",
		Bookmaker: "tab", DecimalOdds: 1.90, UpdatedAt: now, Kickoff: now,
	}}
	bets, err := eval.Evaluate(context.Background(), odds)
	if err != nil {
		t.Fatal(err)
	}
	if len(bets) != 0 {
		t.Fatalf("expected no value bets, got %d", len(bets))
	}
}

func TestEvaluatorDedupBestBookmaker(t *testing.T) {
	cfg := testAFLConfig()
	cfg.MinEVThreshold = 0.01
	eval := NewEvaluator(cfg, stubPredictor{homeProb: 0.60}, stubBuilder{ctx: MatchDayContext{
		Venue: VenueProfile{ID: "GABBA", Dimension: VenueStandard},
	}})
	now := time.Now().UTC()
	odds := []MarketOdds{
		{EventID: "ev1", HomeTeam: "BRI", AwayTeam: "COLL", Team: "BRI", Bookmaker: "a", DecimalOdds: 2.0, UpdatedAt: now, Kickoff: now},
		{EventID: "ev1", HomeTeam: "BRI", AwayTeam: "COLL", Team: "BRI", Bookmaker: "b", DecimalOdds: 2.2, UpdatedAt: now, Kickoff: now},
	}
	bets, err := eval.Evaluate(context.Background(), odds)
	if err != nil {
		t.Fatal(err)
	}
	if len(bets) != 1 {
		t.Fatalf("expected deduped to 1 bet, got %d", len(bets))
	}
	if bets[0].DecimalOdds != 2.2 {
		t.Fatalf("expected best odds 2.2, got %.2f", bets[0].DecimalOdds)
	}
}

func TestComputeEVThreshold(t *testing.T) {
	ev := ComputeEV(0.55, 2.0)
	if ev < 0.09 || ev > 0.11 {
		t.Fatalf("unexpected EV %.4f", ev)
	}
}
