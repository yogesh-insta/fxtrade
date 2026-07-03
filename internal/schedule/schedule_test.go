package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunPeriodicInvokesFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var runs atomic.Int32
	done := make(chan struct{}, 1)
	go RunPeriodic(ctx, "test", time.Hour, func(context.Context) {
		if runs.Add(1) == 1 {
			done <- struct{}{}
		}
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("initial cycle did not run")
	}
	cancel()
}

func TestCycleTimeout(t *testing.T) {
	if cycleTimeout(30*time.Minute) != 10*time.Minute {
		t.Fatalf("30m interval: got %v, want 10m", cycleTimeout(30*time.Minute))
	}
	if cycleTimeout(2*time.Minute) != 5*time.Minute {
		t.Fatalf("2m interval: got %v, want 5m", cycleTimeout(2*time.Minute))
	}
}
