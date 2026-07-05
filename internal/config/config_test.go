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
	if cfg.Notifications.NSEPrefix != "[NiftyPulse]" {
		t.Fatalf("nse_prefix = %q", cfg.Notifications.NSEPrefix)
	}
	if cfg.Notifications.FXPrefix != "[FXPulse]" {
		t.Fatalf("fx_prefix = %q", cfg.Notifications.FXPrefix)
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

func TestNotificationsLegacyPrefixMigratesToFXPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".credentials")
	content := `{
  "oanda": {
    "account_id": "101-001-1",
    "token": "test-token",
    "environment": "practice"
  },
  "notifications": {
    "prefix": "[fxtrade DEMO]"
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notifications.FXPrefix != "[fxtrade DEMO]" {
		t.Fatalf("fx_prefix = %q, want legacy prefix", cfg.Notifications.FXPrefix)
	}
	if cfg.Notifications.NSEPrefix != "[NiftyPulse]" {
		t.Fatalf("nse_prefix = %q", cfg.Notifications.NSEPrefix)
	}
}

func TestEffectiveFXPrefixDryRunDefault(t *testing.T) {
	n := config.NotificationsConfig{}
	if got := n.EffectiveFXPrefix(true); got != "[FXPulse PRACTICE]" {
		t.Fatalf("dry-run default = %q", got)
	}
	if got := n.EffectiveFXPrefix(false); got != "[FXPulse]" {
		t.Fatalf("live default = %q", got)
	}
}

func TestEffectiveNSEPrefixFallback(t *testing.T) {
	n := config.NotificationsConfig{}
	if got := n.EffectiveNSEPrefix(); got != "[NiftyPulse]" {
		t.Fatalf("nse default = %q", got)
	}
}

func TestTradeOnlyEmailDefaultTrue(t *testing.T) {
	n := config.NotificationsConfig{}
	if !n.TradeOnlyEmail() {
		t.Fatal("expected trade-only email default true")
	}
}

func TestTradeOnlyEmailExplicitFalse(t *testing.T) {
	f := false
	n := config.NotificationsConfig{EmailOnTradeOnly: &f}
	if n.TradeOnlyEmail() {
		t.Fatal("expected trade-only email false when explicitly set")
	}
}
