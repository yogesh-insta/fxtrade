package bot_test

import (
	"testing"

	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/config"

	_ "github.com/ym/fxtrade/internal/bots"
)

func TestRegisteredBots(t *testing.T) {
	ids := bot.RegisteredIDs()
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 bots, got %v", ids)
	}
	meta := bot.AllMeta()
	for _, m := range meta {
		if m.ID == "" || m.Name == "" {
			t.Fatalf("incomplete meta: %+v", m)
		}
	}
}

func TestDescribe(t *testing.T) {
	m := bot.Describe(config.BotUniverseScanner)
	if m.ID != config.BotUniverseScanner {
		t.Fatalf("got %+v", m)
	}
	if m.Name == "" {
		t.Fatal("expected name")
	}
}
