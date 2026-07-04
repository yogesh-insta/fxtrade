package afl

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Predictor returns home-team win probability from a feature vector.
type Predictor interface {
	Predict(fv FeatureVector) (winProbability float64, err error)
}

// MatrixModel is the JSON export format from offline Python training.
type MatrixModel struct {
	Bias         float64   `json:"bias"`
	Coefficients []float64 `json:"coefficients"`
}

// MatrixPredictor applies a linear model: sigmoid(bias + dot(coefficients, features)).
type MatrixPredictor struct {
	bias float64
	w    []float64
}

func NewMatrixPredictor(path string) (*MatrixPredictor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read model %s: %w", path, err)
	}
	var m MatrixModel
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse model %s: %w", path, err)
	}
	if len(m.Coefficients) != FeatureCount {
		return nil, fmt.Errorf("model coefficients length %d != FeatureCount %d", len(m.Coefficients), FeatureCount)
	}
	return &MatrixPredictor{bias: m.Bias, w: m.Coefficients}, nil
}

func NewMatrixPredictorFromModel(m MatrixModel) (*MatrixPredictor, error) {
	if len(m.Coefficients) != FeatureCount {
		return nil, fmt.Errorf("model coefficients length %d != FeatureCount %d", len(m.Coefficients), FeatureCount)
	}
	return &MatrixPredictor{bias: m.Bias, w: m.Coefficients}, nil
}

func (p *MatrixPredictor) Predict(fv FeatureVector) (float64, error) {
	if len(fv.Values) != FeatureCount {
		return 0, fmt.Errorf("feature vector length %d != %d", len(fv.Values), FeatureCount)
	}
	z := p.bias
	for i, x := range fv.Values {
		z += p.w[i] * x
	}
	return sigmoid(z), nil
}

func sigmoid(z float64) float64 {
	if z > 20 {
		return 1
	}
	if z < -20 {
		return 0
	}
	return 1.0 / (1.0 + math.Exp(-z))
}

// ONNXPredictor loads an ONNX model for inference (requires onnx build tag for full impl).
type ONNXPredictor struct {
	path string
}

func NewONNXPredictor(path string) *ONNXPredictor {
	return &ONNXPredictor{path: path}
}

func (p *ONNXPredictor) Predict(fv FeatureVector) (float64, error) {
	if p.path == "" {
		return 0, fmt.Errorf("onnx model path is empty")
	}
	return 0, fmt.Errorf("ONNX inference not enabled in this build; use predictor_type=matrix or build with -tags onnx")
}

// NewPredictor selects matrix or onnx predictor from config.
func NewPredictor(predictorType, matrixPath, onnxPath string) (Predictor, error) {
	switch predictorType {
	case "onnx":
		return NewONNXPredictor(onnxPath), nil
	default:
		return NewMatrixPredictor(matrixPath)
	}
}
