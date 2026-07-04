package train

import (
	"math"
	"math/rand"
)

// TrainLogistic fits weights with gradient descent on log-loss.
func TrainLogistic(x [][]float64, y []float64, epochs int, lr float64) (bias float64, weights []float64) {
	if len(x) == 0 {
		return 0, nil
	}
	d := len(x[0])
	weights = make([]float64, d)
	if lr <= 0 {
		lr = 0.08
	}
	if epochs <= 0 {
		epochs = 800
	}
	for ep := 0; ep < epochs; ep++ {
		var gb float64
		gw := make([]float64, d)
		for i, row := range x {
			p := sigmoid(bias + dot(weights, row))
			err := p - y[i]
			gb += err
			for j := range weights {
				gw[j] += err * row[j]
			}
		}
		n := float64(len(x))
		bias -= lr * gb / n
		for j := range weights {
			weights[j] -= lr * gw[j] / n
		}
	}
	return bias, weights
}

// TrainLinear fits a linear model with ridge-regularised normal equations (closed form).
func TrainLinear(x [][]float64, y []float64, ridge float64) (bias float64, weights []float64) {
	if len(x) == 0 {
		return 0, nil
	}
	n := len(x)
	d := len(x[0])
	if ridge <= 0 {
		ridge = 1.0
	}

	// Augment with bias column.
	xt := make([][]float64, d+1)
	for j := 0; j <= d; j++ {
		xt[j] = make([]float64, n)
	}
	for i, row := range x {
		xt[0][i] = 1
		for j, v := range row {
			xt[j+1][i] = v
		}
	}

	// XTX + ridge*I
	dim := d + 1
	xtx := make([][]float64, dim)
	for i := range xtx {
		xtx[i] = make([]float64, dim)
	}
	for i := 0; i < dim; i++ {
		for j := 0; j < dim; j++ {
			var s float64
			for k := 0; k < n; k++ {
				s += xt[i][k] * xt[j][k]
			}
			xtx[i][j] = s
		}
		if i > 0 {
			xtx[i][i] += ridge
		}
	}

	xty := make([]float64, dim)
	for i := 0; i < dim; i++ {
		var s float64
		for k := 0; k < n; k++ {
			s += xt[i][k] * y[k]
		}
		xty[i] = s
	}

	beta := solveSymmetric(xtx, xty)
	return beta[0], beta[1:]
}

func dot(w, x []float64) float64 {
	var s float64
	for i := range w {
		s += w[i] * x[i]
	}
	return s
}

func sigmoid(z float64) float64 {
	if z > 20 {
		return 1
	}
	if z < -20 {
		return 0
	}
	return 1 / (1 + math.Exp(-z))
}

// solveSymmetric solves A x = b for small dense symmetric A (Gaussian elimination).
func solveSymmetric(a [][]float64, b []float64) []float64 {
	n := len(b)
	aug := make([][]float64, n)
	for i := range aug {
		aug[i] = make([]float64, n+1)
		copy(aug[i], a[i])
		aug[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(aug[r][col]) > math.Abs(aug[pivot][col]) {
				pivot = r
			}
		}
		aug[col], aug[pivot] = aug[pivot], aug[col]
		div := aug[col][col]
		if math.Abs(div) < 1e-12 {
			continue
		}
		for j := col; j <= n; j++ {
			aug[col][j] /= div
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := aug[r][col]
			for j := col; j <= n; j++ {
				aug[r][j] -= f * aug[col][j]
			}
		}
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = aug[i][n]
	}
	return out
}

// ShufflePairs shuffles feature rows and labels together.
func ShufflePairs(x [][]float64, y []float64, seed int64) {
	r := rand.New(rand.NewSource(seed))
	for i := len(x) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		x[i], x[j] = x[j], x[i]
		y[i], y[j] = y[j], y[i]
	}
}
