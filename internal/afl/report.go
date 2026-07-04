package afl

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// TotalsOdds is a bookmaker over/under line for match total points.
type TotalsOdds struct {
	EventID   string
	HomeTeam  TeamID
	AwayTeam  TeamID
	Bookmaker string
	Line      float64
	OverPrice float64
	UnderPrice float64
	UpdatedAt time.Time
	Kickoff   time.Time
}

// MatchReport is a full prediction for one fixture.
type MatchReport struct {
	Context     MatchDayContext
	HomeWinProb float64
	Score       ScoreProjection
	ValueBets   []ValueBet
	TotalsLine  *TotalsOdds
}

// Fixture is a deduplicated upcoming match from odds events.
type Fixture struct {
	EventID  string
	HomeTeam TeamID
	AwayTeam TeamID
	Kickoff  time.Time
}

// FixturesFromOdds deduplicates market odds into upcoming fixtures.
func FixturesFromOdds(odds []MarketOdds) []Fixture {
	seen := make(map[string]Fixture)
	for _, mo := range odds {
		if _, ok := seen[mo.EventID]; ok {
			continue
		}
		seen[mo.EventID] = Fixture{
			EventID:  mo.EventID,
			HomeTeam: mo.HomeTeam,
			AwayTeam: mo.AwayTeam,
			Kickoff:  mo.Kickoff,
		}
	}
	out := make([]Fixture, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kickoff.Equal(out[j].Kickoff) {
			return out[i].EventID < out[j].EventID
		}
		return out[i].Kickoff.Before(out[j].Kickoff)
	})
	return out
}

// BestTotalsLine picks the first available totals line per event (prefer lowest line count bookmaker key).
func BestTotalsLine(totals []TotalsOdds) map[string]TotalsOdds {
	best := make(map[string]TotalsOdds)
	for _, t := range totals {
		cur, ok := best[t.EventID]
		if !ok || t.Line > 0 && (cur.Line == 0 || t.Bookmaker < cur.Bookmaker) {
			best[t.EventID] = t
		}
	}
	return best
}

// ValueBetsByEvent groups value bets by event id.
func ValueBetsByEvent(bets []ValueBet) map[string][]ValueBet {
	out := make(map[string][]ValueBet)
	for _, vb := range bets {
		out[vb.EventID] = append(out[vb.EventID], vb)
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i].EV > out[id][j].EV })
	}
	return out
}

// BuildRoundReports produces full match reports for every fixture.
func (e *Evaluator) BuildRoundReports(ctx context.Context, fixtures []Fixture, h2h []MarketOdds, totals []TotalsOdds) ([]MatchReport, []ValueBet, error) {
	valueBets, err := e.Evaluate(ctx, h2h)
	if err != nil {
		// Continue with partial reports when evaluation had non-fatal errors.
	}
	byEvent := ValueBetsByEvent(valueBets)
	totalsByEvent := BestTotalsLine(totals)

	reports := make([]MatchReport, 0, len(fixtures))
	var mu sync.Mutex
	var wg sync.WaitGroup
	errs := make(chan error, len(fixtures))

	for _, fix := range fixtures {
		wg.Add(1)
		go func(fix Fixture) {
			defer wg.Done()
			probe := MarketOdds{
				EventID:  fix.EventID,
				HomeTeam: fix.HomeTeam,
				AwayTeam: fix.AwayTeam,
				Kickoff:  fix.Kickoff,
				Team:     fix.HomeTeam,
			}
			matchCtx, err := e.builder.Build(ctx, probe)
			if err != nil {
				errs <- fmt.Errorf("event %s: %w", fix.EventID, err)
				return
			}
			fv := BuildHomeFeatureVector(matchCtx)
			homeProb, err := e.predictor.Predict(fv)
			if err != nil {
				errs <- fmt.Errorf("event %s predict: %w", fix.EventID, err)
				return
			}
			homeProb = Clamp01(homeProb)
			score := ProjectScore(matchCtx, homeProb)

			report := MatchReport{
				Context:     matchCtx,
				HomeWinProb: homeProb,
				Score:       score,
				ValueBets:   append([]ValueBet(nil), byEvent[fix.EventID]...),
			}
			if t, ok := totalsByEvent[fix.EventID]; ok {
				tcopy := t
				report.TotalsLine = &tcopy
			}

			mu.Lock()
			reports = append(reports, report)
			mu.Unlock()
		}(fix)
	}
	wg.Wait()
	close(errs)

	var buildErr error
	for err := range errs {
		if buildErr == nil {
			buildErr = err
		}
	}

	sort.Slice(reports, func(i, j int) bool {
		if reports[i].Context.Kickoff.Equal(reports[j].Context.Kickoff) {
			return reports[i].Context.EventID < reports[j].Context.EventID
		}
		return reports[i].Context.Kickoff.Before(reports[j].Context.Kickoff)
	})

	if len(reports) == 0 && buildErr != nil {
		return nil, valueBets, buildErr
	}
	return reports, valueBets, buildErr
}

// FormatRoundReportSummary returns a log-friendly one-liner.
func FormatRoundReportSummary(reports []MatchReport, valueBets []ValueBet) string {
	if len(reports) == 0 {
		return "no upcoming fixtures"
	}
	return fmt.Sprintf("%d fixture(s), %d value bet(s)", len(reports), len(valueBets))
}

// WinnerWinProbability returns the model probability for the predicted winner.
func (r MatchReport) WinnerWinProbability() float64 {
	if r.Score.PredictedWinner == r.Context.HomeTeam {
		return r.HomeWinProb
	}
	return 1 - r.HomeWinProb
}

// FormatMatchPredictions returns four labeled prediction lines for one fixture.
func FormatMatchPredictions(r MatchReport) string {
	ctx := r.Context
	var b strings.Builder
	fmt.Fprintf(&b, "  Winner: %s (%.0f%% probability)\n", r.Score.PredictedWinner, r.WinnerWinProbability()*100)
	fmt.Fprintf(&b, "  Total score: %d points\n", r.Score.TotalScore)
	fmt.Fprintf(&b, "  Team scores: %s %d – %s %d\n", ctx.HomeTeam, r.Score.HomeScore, ctx.AwayTeam, r.Score.AwayScore)
	fmt.Fprintf(&b, "  Winning margin: %d points (%s)", r.Score.Margin, r.Score.PredictedWinner)
	return b.String()
}

// FormatFixturePredictionBlock returns a log-friendly fixture header plus labeled predictions.
func FormatFixturePredictionBlock(r MatchReport) string {
	ctx := r.Context
	var b strings.Builder
	fmt.Fprintf(&b, "▸ %s vs %s", ctx.HomeTeam, ctx.AwayTeam)
	if !ctx.Kickoff.IsZero() {
		fmt.Fprintf(&b, " · %s", ctx.Kickoff.Format(time.RFC1123))
	}
	b.WriteString("\n")
	b.WriteString(FormatMatchPredictions(r))
	return b.String()
}
