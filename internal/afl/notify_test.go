package afl

import (
	"strings"
	"testing"
	"time"
)

func TestFormatAlertEmail(t *testing.T) {
	bet := ValueBet{
		HomeTeam:    "BRI",
		AwayTeam:    "COLL",
		Team:        "BRI",
		Bookmaker:   "sportsbet",
		DecimalOdds: 2.10,
		ModelProb:   0.58,
		ImpliedProb: 0.476,
		EV:          0.218,
		Kickoff:     time.Date(2025, 4, 17, 19, 30, 0, 0, time.UTC),
		IsHomePick:  true,
		Reasons:     []string{"Model edge 10.4%"},
	}
	body := FormatAlertEmail(bet, []ValueBet{bet})
	for _, want := range []string{
		"BRI vs COLL",
		"Why this bet:",
		"Manual execution required",
		"AFLPulse does NOT place bets",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}

func TestMatrixPredictor(t *testing.T) {
	coeffs := make([]float64, FeatureCount)
	for i := range coeffs {
		coeffs[i] = 0.05
	}
	p, err := NewMatrixPredictorFromModel(MatrixModel{Bias: 0, Coefficients: coeffs})
	if err != nil {
		t.Fatal(err)
	}
	fv := FeatureVector{Values: make([]float64, FeatureCount)}
	for i := range fv.Values {
		fv.Values[i] = 0.5
	}
	prob, err := p.Predict(fv)
	if err != nil {
		t.Fatal(err)
	}
	if prob <= 0 || prob >= 1 {
		t.Fatalf("expected probability in (0,1), got %.4f", prob)
	}
}
