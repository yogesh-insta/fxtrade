package sentiment

import "testing"

func TestFormatEmailBody(t *testing.T) {
	hold := "awaiting NFP"
	body := FormatEmailBody(SentimentSignal{
		Instrument: "EUR_USD",
		Direction:  "LONG",
		Confidence: 0.8,
		BaseBias:   "bullish",
		EventRisk:  "low",
		Drivers:    []string{"driver one", "driver two"},
		Risks:      []string{"risk one"},
		HoldReason: &hold,
	}, 30)
	if body == "" {
		t.Fatal("expected body")
	}
	for _, want := range []string{"EUR_USD sentiment update", "does NOT place a trade", "• driver one", "• risk one", "Hold reason: awaiting NFP"} {
		if !contains(body, want) {
			t.Fatalf("missing %q in body:\n%s", want, body)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
