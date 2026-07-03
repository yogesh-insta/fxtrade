package sentiment

import "testing"

func TestNormalizeConfidence(t *testing.T) {
	tests := []struct {
		in, want float64
	}{
		{0.8, 0.8},
		{80, 0.8},
		{-1, 0},
		{150, 1},
	}
	for _, tc := range tests {
		if got := normalizeConfidence(tc.in); got != tc.want {
			t.Fatalf("normalizeConfidence(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
