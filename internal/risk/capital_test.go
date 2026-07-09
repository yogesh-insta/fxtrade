package risk_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/risk"
)

func TestEffectiveCapital(t *testing.T) {
	if got := risk.EffectiveCapital(5000, 100000); got != 5000 {
		t.Fatalf("allocated: got %v want 5000", got)
	}
	if got := risk.EffectiveCapital(0, 100000); got != 100000 {
		t.Fatalf("fallback: got %v want 100000", got)
	}
}
