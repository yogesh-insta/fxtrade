package sentiment

import (
	"strings"
	"testing"
)

func TestSystemPromptFor(t *testing.T) {
	aud := SystemPromptFor("AUD_USD")
	for _, want := range []string{"AUD/USD", "AUD_USD", "base_bias", "RBA policy"} {
		if !strings.Contains(aud, want) {
			t.Fatalf("AUD_USD prompt missing %q", want)
		}
	}

	eur := SystemPromptFor("EUR_USD")
	for _, want := range []string{"EUR/USD", "EUR_USD", "base_bias", "ECB policy"} {
		if !strings.Contains(eur, want) {
			t.Fatalf("EUR_USD prompt missing %q", want)
		}
	}
	if strings.Contains(eur, "RBA") {
		t.Fatal("EUR_USD prompt should not mention RBA")
	}
}
