package train

import (
	"context"
	"fmt"
	"sort"

	"github.com/ym/fxtrade/internal/afl"
	"github.com/ym/fxtrade/internal/afl/stats"
)

// Example is one historical match with features and labels.
type Example struct {
	Year     int
	Round    int
	HomeTeam afl.TeamID
	AwayTeam afl.TeamID
	HomeWin  float64
	TotalPts float64
	WinFV    afl.FeatureVector
	TotalsFV afl.FeatureVector
}

// BuildExamples fetches Squiggle history and builds walk-forward training rows.
func BuildExamples(ctx context.Context, repo *stats.Repository, client *stats.SquiggleClient, fromYear, toYear int) ([]Example, error) {
	var all []stats.HistoricalGame
	for year := fromYear; year <= toYear; year++ {
		games, err := client.FetchHistoricalGames(ctx, year)
		if err != nil {
			return nil, fmt.Errorf("year %d: %w", year, err)
		}
		all = append(all, games...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Year != all[j].Year {
			return all[i].Year < all[j].Year
		}
		if all[i].Round != all[j].Round {
			return all[i].Round < all[j].Round
		}
		return all[i].Date < all[j].Date
	})

	venueTotals := make(map[afl.VenueID]struct{ sum float64; n int })
	seasonTrackers := make(map[int]*stats.RollingTracker)

	var out []Example
	for _, g := range all {
		if g.Complete < 100 {
			continue
		}
		homeID, err := repo.ResolveTeam(g.HTeam)
		if err != nil {
			continue
		}
		awayID, err := repo.ResolveTeam(g.ATeam)
		if err != nil {
			continue
		}

		if seasonTrackers[g.Year] == nil {
			seasonTrackers[g.Year] = stats.NewRollingTracker()
		}
		st := seasonTrackers[g.Year]

		homeStats, awayStats, _, _ := st.Snapshot(homeID, awayID)
		venue := repo.DefaultVenueForTeam(homeID)
		if vid, ok := repo.ResolveVenue(g.Venue); ok {
			if v, ok2 := repo.Venue(vid); ok2 {
				venue = v
			}
		}
		if agg, ok := venueTotals[venue.ID]; ok && agg.n > 0 {
			venue.AvgTotalScore = agg.sum / float64(agg.n)
		}

		matchCtx := afl.MatchDayContext{
			HomeTeam:  homeID,
			AwayTeam:  awayID,
			Venue:     venue,
			Weather:   afl.WeatherMetrics{ContestFavorability: 0.2, TotalPointsFactor: 1.0},
			HomeStats: homeStats,
			AwayStats: awayStats,
		}

		homeWin := 0.0
		if g.HScore > g.AScore {
			homeWin = 1
		} else if g.HScore == g.AScore {
			homeWin = 0.5
		}

		out = append(out, Example{
			Year:     g.Year,
			Round:    g.Round,
			HomeTeam: homeID,
			AwayTeam: awayID,
			HomeWin:  homeWin,
			TotalPts: float64(g.HScore + g.AScore),
			WinFV:    afl.BuildHomeFeatureVector(matchCtx),
			TotalsFV: afl.BuildTotalsFeatureVector(matchCtx),
		})

		st.RecordGame(homeID, awayID, g.HScore, g.AScore, g.HScore > g.AScore)
		if vid, ok := repo.ResolveVenue(g.Venue); ok {
			agg := venueTotals[vid]
			agg.sum += float64(g.HScore + g.AScore)
			agg.n++
			venueTotals[vid] = agg
		}
	}
	return out, nil
}

// SplitHoldout returns train and test sets; test is exactly holdoutYear fixtures.
func SplitHoldout(examples []Example, holdoutYear int) (train, test []Example) {
	for _, ex := range examples {
		if ex.Year == holdoutYear {
			test = append(test, ex)
		} else {
			train = append(train, ex)
		}
	}
	return train, test
}

func winRows(examples []Example) ([][]float64, []float64) {
	x := make([][]float64, len(examples))
	y := make([]float64, len(examples))
	for i, ex := range examples {
		x[i] = append([]float64(nil), ex.WinFV.Values...)
		y[i] = ex.HomeWin
	}
	return x, y
}

func totalsRows(examples []Example) ([][]float64, []float64) {
	x := make([][]float64, len(examples))
	y := make([]float64, len(examples))
	for i, ex := range examples {
		x[i] = append([]float64(nil), ex.TotalsFV.Values...)
		y[i] = ex.TotalPts
	}
	return x, y
}

// TrainWinModel fits logistic regression on training examples.
func TrainWinModel(train []Example) afl.MatrixModel {
	x, y := winRows(train)
	ShufflePairs(x, y, 42)
	bias, w := TrainLogistic(x, y, 1200, 0.1)
	return afl.MatrixModel{Bias: bias, Coefficients: w}
}

// TrainTotalsModel fits linear regression on training examples.
func TrainTotalsModel(train []Example) afl.LinearModel {
	x, y := totalsRows(train)
	bias, w := TrainLinear(x, y, 2.0)
	return afl.LinearModel{Bias: bias, Coefficients: w}
}

// SaveModels writes win and totals coefficient files.
func SaveModels(winPath, totalsPath string, win afl.MatrixModel, totals afl.LinearModel) error {
	if err := saveMatrix(winPath, win); err != nil {
		return err
	}
	return afl.SaveLinearModel(totalsPath, totals)
}

func saveMatrix(path string, m afl.MatrixModel) error {
	return afl.SaveMatrixModel(path, m)
}
