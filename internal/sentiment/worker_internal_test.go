package sentiment

import (
	"testing"

	"github.com/ym/fxtrade/internal/config"
)

func TestEffectiveIntervalMinutes(t *testing.T) {
	cases := []struct {
		name       string
		configured int
		want       int
	}{
		{"below floor is bumped", 15, config.MinSentimentIntervalMinutes},
		{"zero is bumped", 0, config.MinSentimentIntervalMinutes},
		{"at floor kept", config.MinSentimentIntervalMinutes, config.MinSentimentIntervalMinutes},
		{"above floor kept", 60, 60},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &Worker{cfg: config.SentimentConfig{IntervalMinutes: tc.configured}}
			if got := w.effectiveIntervalMinutes(); got != tc.want {
				t.Fatalf("effectiveIntervalMinutes(%d) = %d, want %d", tc.configured, got, tc.want)
			}
		})
	}
}
