package afl

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

// ContextBuilder enriches raw odds with match-day context from stats and weather.
type ContextBuilder interface {
	Build(ctx context.Context, odds MarketOdds) (MatchDayContext, error)
}

// RepositoryContextBuilder builds match context from a stats repository.
type RepositoryContextBuilder struct {
	Repo           StatsSource
	Weather        WeatherSource
	InjuriesFile   string
	TravelKMTable  map[string]float64 // "HOME-AWAY" -> km, optional
}

// StatsSource provides team/venue data.
type StatsSource interface {
	ResolveTeam(name string) (TeamID, error)
	Team(id TeamID) (TeamStats, bool)
	Venue(id VenueID) (VenueProfile, bool)
	DefaultVenueForTeam(team TeamID) VenueProfile
	PlayerMatrix(home, away TeamID, injuriesPath string) PlayerAvailabilityMatrix
}

// WeatherSource fetches weather for a venue at kickoff.
type WeatherSource interface {
	FetchForVenue(ctx context.Context, venue VenueProfile, kickoff time.Time) (WeatherMetrics, error)
}

func (b *RepositoryContextBuilder) Build(ctx context.Context, odds MarketOdds) (MatchDayContext, error) {
	homeStats, ok := b.Repo.Team(odds.HomeTeam)
	if !ok {
		homeStats = TeamStats{TeamID: odds.HomeTeam}
	}
	awayStats, ok := b.Repo.Team(odds.AwayTeam)
	if !ok {
		awayStats = TeamStats{TeamID: odds.AwayTeam}
	}
	venue := b.Repo.DefaultVenueForTeam(odds.HomeTeam)
	if v, ok := b.Repo.Venue(venue.ID); ok {
		venue = v
	}

	travelKM := estimateTravelKM(odds.HomeTeam, odds.AwayTeam, venue)
	if b.TravelKMTable != nil {
		key := string(odds.HomeTeam) + "-" + string(odds.AwayTeam)
		if km, ok := b.TravelKMTable[key]; ok {
			travelKM = km
		}
	}

	weather, err := b.Weather.FetchForVenue(ctx, venue, odds.Kickoff)
	if err != nil {
		weather = WeatherMetrics{ContestFavorability: 0.2, TotalPointsFactor: 1.0}
	}

	players := b.Repo.PlayerMatrix(odds.HomeTeam, odds.AwayTeam, b.InjuriesFile)

	return MatchDayContext{
		EventID:       odds.EventID,
		HomeTeam:      odds.HomeTeam,
		AwayTeam:      odds.AwayTeam,
		Venue:         venue,
		Kickoff:       odds.Kickoff,
		TravelKM:      travelKM,
		TravelHours:   travelKM / 800.0,
		CrowdEstimate: 40000,
		Weather:       weather,
		Players:       players,
		HomeStats:     homeStats,
		AwayStats:     awayStats,
	}, nil
}

func estimateTravelKM(home, away TeamID, venue VenueProfile) float64 {
	if venue.Interstate {
		return 2500
	}
	if home != away {
		return 800
	}
	return 0
}

// Evaluator processes market odds concurrently and finds value bets.
type Evaluator struct {
	cfg       config.AFLConfig
	predictor Predictor
	builder   ContextBuilder
}

func NewEvaluator(cfg config.AFLConfig, predictor Predictor, builder ContextBuilder) *Evaluator {
	return &Evaluator{cfg: cfg, predictor: predictor, builder: builder}
}

// Evaluate runs the pipeline on all market odds and returns deduplicated value bets.
func (e *Evaluator) Evaluate(ctx context.Context, odds []MarketOdds) ([]ValueBet, error) {
	if len(odds) == 0 {
		return nil, nil
	}

	maxAge := time.Duration(e.cfg.MaxOddsAgeMinutes) * time.Minute
	now := time.Now().UTC()

	jobs := make(chan MarketOdds, len(odds))
	results := make(chan ValueBet, len(odds))
	errs := make(chan error, e.cfg.Concurrency)

	var wg sync.WaitGroup
	workers := e.cfg.Concurrency
	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mo := range jobs {
				if err := mo.Validate(); err != nil {
					errs <- err
					continue
				}
				if maxAge > 0 && !mo.UpdatedAt.IsZero() && now.Sub(mo.UpdatedAt) > maxAge {
					continue
				}
				matchCtx, err := e.builder.Build(ctx, mo)
				if err != nil {
					errs <- err
					continue
				}
				vb, ok, err := e.evaluateOne(ctx, matchCtx, mo)
				if err != nil {
					errs <- err
					continue
				}
				if ok {
					results <- vb
				}
			}
		}()
	}

	go func() {
		for _, o := range odds {
			jobs <- o
		}
		close(jobs)
		wg.Wait()
		close(results)
		close(errs)
	}()

	var evalErr error
	for err := range errs {
		if evalErr == nil {
			evalErr = err
		}
	}

	best := make(map[string]ValueBet) // key: event|team
	for vb := range results {
		key := vb.EventID + "|" + string(vb.Team)
		if cur, ok := best[key]; !ok || vb.EV > cur.EV {
			best[key] = vb
		}
	}

	out := make([]ValueBet, 0, len(best))
	for _, vb := range best {
		out = append(out, vb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EV > out[j].EV })

	if len(out) == 0 && evalErr != nil {
		return nil, evalErr
	}
	return out, nil
}

func (e *Evaluator) evaluateOne(ctx context.Context, matchCtx MatchDayContext, mo MarketOdds) (ValueBet, bool, error) {
	_ = ctx
	fv := BuildHomeFeatureVector(matchCtx)
	homeProb, err := e.predictor.Predict(fv)
	if err != nil {
		return ValueBet{}, false, err
	}
	homeProb = Clamp01(homeProb)
	awayProb := Clamp01(1.0 - homeProb)

	var modelProb float64
	isHome := mo.Team == mo.HomeTeam
	if isHome {
		modelProb = homeProb
	} else {
		modelProb = awayProb
	}

	ev := ComputeEV(modelProb, mo.DecimalOdds)
	if ev <= e.cfg.MinEVThreshold {
		return ValueBet{}, false, nil
	}

	implied := mo.ImpliedProb()
	vb := ValueBet{
		EventID:     mo.EventID,
		HomeTeam:    mo.HomeTeam,
		AwayTeam:    mo.AwayTeam,
		Team:        mo.Team,
		Bookmaker:   mo.Bookmaker,
		ModelProb:   modelProb,
		ImpliedProb: implied,
		DecimalOdds: mo.DecimalOdds,
		EV:          ev,
		EdgePct:     ev,
		Kickoff:     mo.Kickoff,
		IsHomePick:  isHome,
	}
	vb.Reasons = BuildValueBetReasons(matchCtx, vb)
	return vb, true, nil
}

// TopN returns the first n value bets (already sorted by EV).
func TopN(bets []ValueBet, n int) []ValueBet {
	if n <= 0 || len(bets) == 0 {
		return nil
	}
	if len(bets) <= n {
		return append([]ValueBet(nil), bets...)
	}
	return append([]ValueBet(nil), bets[:n]...)
}

// FormatEvaluateSummary returns a log-friendly summary.
func FormatEvaluateSummary(bets []ValueBet) string {
	if len(bets) == 0 {
		return "no value bets found"
	}
	return fmt.Sprintf("%d value bet(s); best EV %.1f%% on %s", len(bets), bets[0].EV*100, bets[0].Team)
}
