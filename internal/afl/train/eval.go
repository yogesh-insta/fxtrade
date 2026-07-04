package train

import (
	"fmt"
	"math"

	"github.com/ym/fxtrade/internal/afl"
)

// WinMetrics holds holdout evaluation for the H2H model.
type WinMetrics struct {
	Games       int
	Accuracy    float64
	LogLoss     float64
	Brier       float64
}

// TotalsMetrics holds holdout evaluation for the totals model.
type TotalsMetrics struct {
	Games int
	MAE   float64
	RMSE  float64
	Bias  float64
}

// EvaluateWinModel scores a fitted matrix model on holdout examples.
func EvaluateWinModel(m afl.MatrixModel, test []Example) WinMetrics {
	pred, err := afl.NewMatrixPredictorFromModel(m)
	if err != nil {
		return WinMetrics{}
	}
	var correct, logLoss, brier float64
	for _, ex := range test {
		p, err := pred.Predict(ex.WinFV)
		if err != nil {
			continue
		}
		p = clamp(p, 1e-6, 1-1e-6)
		y := ex.HomeWin
		if (p >= 0.5 && y >= 0.5) || (p < 0.5 && y < 0.5) {
			correct++
		}
		logLoss += -(y*math.Log(p) + (1-y)*math.Log(1-p))
		brier += (p - y) * (p - y)
	}
	n := float64(len(test))
	if n == 0 {
		return WinMetrics{}
	}
	return WinMetrics{
		Games:    len(test),
		Accuracy: correct / n,
		LogLoss:  logLoss / n,
		Brier:    brier / n,
	}
}

// EvaluateTotalsModel scores a fitted linear model on holdout examples.
func EvaluateTotalsModel(m afl.LinearModel, test []Example) TotalsMetrics {
	pred, err := afl.NewLinearPredictorFromModel(m)
	if err != nil {
		return TotalsMetrics{}
	}
	var mae, rmse, bias float64
	for _, ex := range test {
		p, err := pred.Predict(ex.TotalsFV)
		if err != nil {
			continue
		}
		errVal := p - ex.TotalPts
		mae += math.Abs(errVal)
		rmse += errVal * errVal
		bias += errVal
	}
	n := float64(len(test))
	if n == 0 {
		return TotalsMetrics{}
	}
	return TotalsMetrics{
		Games: len(test),
		MAE:   mae / n,
		RMSE:  math.Sqrt(rmse / n),
		Bias:  bias / n,
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// FormatReport returns a human-readable training summary.
func FormatReport(holdout int, win WinMetrics, totals TotalsMetrics) string {
	return fmt.Sprintf(
		"holdout %d: H2H accuracy %.1f%% logloss %.3f | totals MAE %.1f RMSE %.1f bias %+.1f (%d games)",
		holdout, win.Accuracy*100, win.LogLoss, totals.MAE, totals.RMSE, totals.Bias, totals.Games,
	)
}
