package config_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/config"
)

func TestEnabledBotsExplicit(t *testing.T) {
	cfg := &config.Config{
		Bots: config.BotsConfig{Enabled: []string{config.BotUniverseScanner, config.BotRangeTrend}},
	}
	got := cfg.EnabledBots()
	if len(got) != 2 || got[0] != config.BotUniverseScanner {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestEnabledBotsLegacyScannerMode(t *testing.T) {
	cfg := &config.Config{
		Strategy: config.StrategyConfig{Mode: config.StrategyModeUniverseScanner},
	}
	got := cfg.EnabledBots()
	if len(got) != 1 || got[0] != config.BotUniverseScanner {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestStateFileForBot(t *testing.T) {
	got := config.StateFileForBot("data/state.json", config.BotUniverseScanner)
	want := "data/state-universe_scanner.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHaltFileForBot(t *testing.T) {
	got := config.HaltFileForBot(".halt", config.BotUniverseScanner)
	if got != ".halt.universe_scanner" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateBotsUnknown(t *testing.T) {
	cfg := &config.Config{
		Bots: config.BotsConfig{Enabled: []string{"unknown_bot"}},
	}
	if err := cfg.ValidateBots(); err == nil {
		t.Fatal("expected error for unknown bot")
	}
}
