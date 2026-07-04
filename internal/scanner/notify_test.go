package scanner

import (
	"testing"

	"github.com/ym/fxtrade/internal/config"
)

func TestNotifierPrefixSubject(t *testing.T) {
	n := &Notifier{
		cfg: config.NotificationsConfig{FXPrefix: "[FXPulse]"},
	}
	got := n.prefixSubject("ORB LONG EUR_USD")
	want := "[FXPulse] ORB LONG EUR_USD"
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}

func TestNotifierPrefixSubjectDryRunDefault(t *testing.T) {
	n := &Notifier{cfg: config.NotificationsConfig{}, dryRun: true}
	got := n.prefixSubject("daemon started")
	want := "[FXPulse PRACTICE] daemon started"
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}

func TestNotifierTradeEntrySubjectDryRun(t *testing.T) {
	n := &Notifier{
		cfg:    config.NotificationsConfig{FXPrefix: "[FXPulse]", Enabled: true, OnTradeEntry: true},
		dryRun: true,
	}
	got := n.prefixSubject("[PAPER] ORB LONG EUR_USD")
	want := "[FXPulse] [PAPER] ORB LONG EUR_USD"
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}

func TestNotifierLegacyPrefix(t *testing.T) {
	n := &Notifier{
		cfg: config.NotificationsConfig{Prefix: "[fxtrade DEMO]"},
	}
	got := n.prefixSubject("Daily P&L summary")
	want := "[fxtrade DEMO] Daily P&L summary"
	if got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}
