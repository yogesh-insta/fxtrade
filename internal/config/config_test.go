package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ym/fxtrade/internal/config"
)

func TestLoadPracticeConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	content := `{
  "oanda": {
    "account_id": "101-001-1",
    "token": "test-token",
    "environment": "practice"
  },
  "risk": {
    "max_spread_pips": 2.5
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OANDA.RESTBaseURL() != "https://api-fxpractice.oanda.com" {
		t.Fatalf("unexpected rest url: %s", cfg.OANDA.RESTBaseURL())
	}
	if cfg.OANDA.StreamBaseURL() != "https://stream-fxpractice.oanda.com" {
		t.Fatalf("unexpected stream url: %s", cfg.OANDA.StreamBaseURL())
	}
}

func TestLoadMissingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	content := `{"oanda":{"account_id":"101-001-1"}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected error for missing token")
	}
}
