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

func TestEffectiveAFLPrefix(t *testing.T) {
	n := NotificationsConfig{AFLPrefix: "[AFLPulse TEST]"}
	if n.EffectiveAFLPrefix() != "[AFLPulse TEST]" {
		t.Fatal("prefix mismatch")
	}
}
