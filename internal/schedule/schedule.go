package schedule

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// RunPeriodic invokes fn immediately, then on each tick. Cycles run in their
// own goroutine so a slow tick cannot block the ticker. Overlapping cycles are
// skipped with a warning. Panics are recovered and logged.
func RunPeriodic(ctx context.Context, name string, interval time.Duration, fn func(context.Context)) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	slog.Info(name+" started", "interval", interval)
	runSafe(ctx, name, interval, fn)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var mu sync.Mutex
	var running bool

	for {
		select {
		case <-ctx.Done():
			slog.Info(name + " stopping")
			return
		case <-ticker.C:
			mu.Lock()
			if running {
				slog.Warn(name+" cycle still running, skipping tick")
				mu.Unlock()
				continue
			}
			running = true
			mu.Unlock()

			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error(name+" cycle panic", "panic", r)
					}
					mu.Lock()
					running = false
					mu.Unlock()
				}()
				runSafe(ctx, name, interval, fn)
			}()
		}
	}
}

func runSafe(ctx context.Context, name string, interval time.Duration, fn func(context.Context)) {
	timeout := cycleTimeout(interval)
	cycleCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(cycleCtx)
	}()

	select {
	case <-done:
	case <-cycleCtx.Done():
		if ctx.Err() != nil {
			return
		}
		slog.Error(name+" cycle timed out", "timeout", timeout)
	}
}

func cycleTimeout(interval time.Duration) time.Duration {
	t := interval / 2
	if t < 5*time.Minute {
		return 5 * time.Minute
	}
	if t > 10*time.Minute {
		return 10 * time.Minute
	}
	return t
}
