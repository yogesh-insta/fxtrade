package health

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateHealthy(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	st := Status{
		StreamConnected: true,
		LastTickAt:      now.Add(-2 * time.Minute),
		StartedAt:       now.Add(-1 * time.Hour),
		Bots:            []BotStatus{{ID: "universe_scanner", Running: true}},
	}
	ok, issues := Evaluate(st, now, DefaultStaleTickAge)
	if !ok || len(issues) != 0 {
		t.Fatalf("expected healthy, got ok=%v issues=%v", ok, issues)
	}
}

func TestEvaluateStaleTicks(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	st := Status{
		StreamConnected: true,
		LastTickAt:      now.Add(-15 * time.Minute),
		StartedAt:       now.Add(-1 * time.Hour),
	}
	ok, issues := Evaluate(st, now, DefaultStaleTickAge)
	if ok || len(issues) != 1 {
		t.Fatalf("expected stale tick issue, got ok=%v issues=%v", ok, issues)
	}
}

func TestEvaluateSkipsStaleOnWeekend(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC) // Saturday
	st := Status{
		StreamConnected: true,
		LastTickAt:      now.Add(-24 * time.Hour),
		StartedAt:       now.Add(-48 * time.Hour),
	}
	ok, issues := Evaluate(st, now, DefaultStaleTickAge)
	if !ok || len(issues) != 0 {
		t.Fatalf("expected no stale alert on Saturday, got ok=%v issues=%v", ok, issues)
	}
}

func TestEvaluateHaltedAndBotDown(t *testing.T) {
	now := time.Now()
	st := Status{
		Halted:          true,
		StreamConnected: false,
		Bots:            []BotStatus{{ID: "universe_scanner", Running: false}},
	}
	ok, issues := Evaluate(st, now, DefaultStaleTickAge)
	if ok || len(issues) < 3 {
		t.Fatalf("expected multiple issues, got ok=%v issues=%v", ok, issues)
	}
}

func TestEvaluateStaleLastCycleOKAt(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	st := Status{
		StreamConnected: true,
		LastTickAt:      now.Add(-2 * time.Minute),
		StartedAt:       now.Add(-1 * time.Hour),
		Bots: []BotStatus{{
			ID:            "btc_cfd",
			Running:       true,
			LastCycleOKAt: now.Add(-20 * time.Minute),
		}},
	}
	ok, issues := Evaluate(st, now, DefaultStaleTickAge)
	if ok || len(issues) != 1 {
		t.Fatalf("expected stale cycle issue, got ok=%v issues=%v", ok, issues)
	}
	if !strings.Contains(issues[0], "btc_cfd stale cycle") {
		t.Fatalf("unexpected issue: %q", issues[0])
	}
}

func TestEvaluateSkipsStaleCycleOnWeekend(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC) // Saturday
	st := Status{
		StreamConnected: true,
		Bots: []BotStatus{{
			ID:            "universe_scanner",
			Running:       true,
			LastCycleOKAt: now.Add(-2 * time.Hour),
		}},
	}
	ok, issues := Evaluate(st, now, DefaultStaleTickAge)
	if !ok || len(issues) != 0 {
		t.Fatalf("expected no FX stale cycle alert on Saturday, got ok=%v issues=%v", ok, issues)
	}
}
