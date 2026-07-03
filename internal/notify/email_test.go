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
