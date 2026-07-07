package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

type countingNotifier struct {
	mu    sync.Mutex
	sends int
}

func (c *countingNotifier) Send(ctx context.Context, subject, body string) {
	_ = ctx
	_ = subject
	_ = body
	c.mu.Lock()
	c.sends++
	c.mu.Unlock()
}

func (c *countingNotifier) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sends
}

func TestSendDigestRateLimit(t *testing.T) {
	base := &countingNotifier{}
	n := &rateLimitedNotifier{
		inner:       base,
		minInterval: time.Hour,
	}

	ctx := context.Background()
	SendDigest(n, ctx, "sentiment:AUD_USD", "subject 1", "body")
	SendDigest(n, ctx, "sentiment:AUD_USD", "subject 2", "body")

	if got := base.count(); got != 1 {
		t.Fatalf("expected 1 digest email, got %d", got)
	}

	SendDigest(n, ctx, "strategy:AUD_USD", "subject 3", "body")
	if got := base.count(); got != 2 {
		t.Fatalf("expected 2 digest emails for different keys, got %d", got)
	}

	n.Send(ctx, "urgent", "always deliver")
	if got := base.count(); got != 3 {
		t.Fatalf("expected urgent email through, got %d", got)
	}
}

func TestNewAppliesRateLimitFromConfig(t *testing.T) {
	n := New(config.EmailConfig{MinIntervalMinutes: 120})
	if _, ok := n.(digestNotifier); !ok {
		t.Fatal("expected digest-capable notifier when min_interval_minutes is set")
	}
}

func TestTradeOnlySuppressesRoutineEmails(t *testing.T) {
	base := &countingNotifier{}
	n := WithTradeOnly(base, true)
	ctx := context.Background()

	SendDigest(n, ctx, "strategy:AUD_USD", "fxtrade: AUD_USD strategy — STAND_ASIDE", "body")
	SendDigest(n, ctx, "sentiment:AUD_USD", "fxtrade: AUD_USD sentiment LONG (70%)", "body")
	SendRoutine(n, ctx, "fxtrade: platform started", "startup")
	SendRoutine(n, ctx, "fxtrade: order rejected", "rejected")

	if got := base.count(); got != 0 {
		t.Fatalf("expected 0 routine emails, got %d", got)
	}

	n.Send(ctx, "fxtrade: OPEN AUD_USD LONG 1000 units", "fill confirmed")
	n.Send(ctx, "fxtrade: OPEN AUD_USD LONG 1000 units", "trade opened")
	n.Send(ctx, "fxtrade: CLOSED AUD_USD (+$10.00)", "trade closed")

	if got := base.count(); got != 3 {
		t.Fatalf("expected 3 trade emails, got %d", got)
	}
}

func TestTradeOnlyDisabledAllowsDigests(t *testing.T) {
	base := &countingNotifier{}
	n := WithTradeOnly(base, false)
	ctx := context.Background()

	SendDigest(n, ctx, "strategy:AUD_USD", "fxtrade: AUD_USD strategy — TREND", "body")
	if got := base.count(); got != 1 {
		t.Fatalf("expected digest through when trade-only disabled, got %d", got)
	}
}

func TestDailySummaryOnlySuppressesAllEmails(t *testing.T) {
	base := &countingNotifier{}
	n := WithDailySummaryOnly(base, true)
	ctx := context.Background()

	SendDigest(n, ctx, "strategy:AUD_USD", "fxtrade: AUD_USD strategy — STAND_ASIDE", "body")
	SendRoutine(n, ctx, "fxtrade: platform started", "startup")
	n.Send(ctx, "fxtrade: OPEN AUD_USD LONG 1000 units", "fill confirmed")
	n.Send(ctx, "fxtrade: CLOSED AUD_USD (+$10.00)", "trade closed")

	if got := base.count(); got != 0 {
		t.Fatalf("expected 0 emails with daily_summary_only, got %d", got)
	}
}
