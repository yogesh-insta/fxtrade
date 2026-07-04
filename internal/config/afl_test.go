package config

import "testing"

func TestDefaultAFLConfig(t *testing.T) {
	def := DefaultAFLConfig()
	if def.MinEVThreshold != 0.05 {
		t.Fatalf("unexpected min ev: %v", def.MinEVThreshold)
	}
	if def.OddsSportKey != "aussierules_afl" {
		t.Fatalf("unexpected sport key: %s", def.OddsSportKey)
	}
}

func TestAFLConfigValidateRequiresKey(t *testing.T) {
	cfg := DefaultAFLConfig()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error without api key")
	}
	cfg.OddsAPIKey = "test"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAFLConfigLLMAnalyticsEnabled(t *testing.T) {
	cfg := DefaultAFLConfig()
	if cfg.AnalyticsEnabled() {
		t.Fatal("should be disabled without api key")
	}
	cfg.GeminiAPIKey = "key"
	if cfg.AnalyticsEnabled() {
		t.Fatal("weekly LLM should be opt-in (default off)")
	}
	enabled := true
	cfg.LLMAnalyticsEnabled = &enabled
	if !cfg.AnalyticsEnabled() {
		t.Fatal("should enable when explicitly set")
	}
	disabled := false
	cfg.LLMAnalyticsEnabled = &disabled
	if cfg.AnalyticsEnabled() {
		t.Fatal("should respect explicit false")
	}
	if cfg.ResolvedGeminiModel() != "gemini-2.5-flash-lite" {
		t.Fatalf("model = %q", cfg.ResolvedGeminiModel())
	}
}

func TestAFLConfigPregameDefaults(t *testing.T) {
	cfg := DefaultAFLConfig()
	if cfg.PregameLeadMinutes != 30 {
		t.Fatalf("lead = %d", cfg.PregameLeadMinutes)
	}
	if cfg.PregamePollWindowMinutes != 5 {
		t.Fatalf("window = %d", cfg.PregamePollWindowMinutes)
	}
	if cfg.PregameLLMRetries != 2 {
		t.Fatalf("retries = %d", cfg.PregameLLMRetries)
	}
	if !cfg.PregameLLMRequiredEnabled() {
		t.Fatal("pregame LLM should be required by default")
	}
	if cfg.ResolvedPregameStatePath() != "data/afl/pregame-sent.json" {
		t.Fatalf("state path = %q", cfg.ResolvedPregameStatePath())
	}
}

func TestEffectiveAFLPrefix(t *testing.T) {
	n := NotificationsConfig{AFLPrefix: "[AFLPulse TEST]"}
	if n.EffectiveAFLPrefix() != "[AFLPulse TEST]" {
		t.Fatal("prefix mismatch")
	}
}
