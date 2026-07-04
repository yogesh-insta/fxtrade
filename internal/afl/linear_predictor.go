package afl

import (
	"encoding/json"
	"fmt"
	"os"
)

// LinearModel is a bias + dot-product model for continuous targets (match totals).
type LinearModel struct {
	Bias         float64   `json:"bias"`
	Coefficients []float64 `json:"coefficients"`
}

// LinearPredictor predicts match total points from a feature vector.
type LinearPredictor struct {
	bias float64
	w    []float64
}

func NewLinearPredictor(path string) (*LinearPredictor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m LinearModel
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse totals model %s: %w", path, err)
	}
	if len(m.Coefficients) != TotalsFeatureCount {
		return nil, fmt.Errorf("totals coefficients length %d != %d", len(m.Coefficients), TotalsFeatureCount)
	}
	return &LinearPredictor{bias: m.Bias, w: m.Coefficients}, nil
}

func NewLinearPredictorFromModel(m LinearModel) (*LinearPredictor, error) {
	if len(m.Coefficients) != TotalsFeatureCount {
		return nil, fmt.Errorf("totals coefficients length %d != %d", len(m.Coefficients), TotalsFeatureCount)
	}
	return &LinearPredictor{bias: m.Bias, w: m.Coefficients}, nil
}

func (p *LinearPredictor) Predict(fv FeatureVector) (float64, error) {
	if len(fv.Values) != TotalsFeatureCount {
		return 0, fmt.Errorf("totals feature length %d != %d", len(fv.Values), TotalsFeatureCount)
	}
	z := p.bias
	for i, x := range fv.Values {
		z += p.w[i] * x
	}
	return z, nil
}

// SaveLinearModel writes coefficients to JSON.
func SaveLinearModel(path string, m LinearModel) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
