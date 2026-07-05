package btc_cfd

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

// runCycle polls on M5 boundaries: fetch candles and log; strategy signals are Phase 2.
func runCycle(ctx context.Context, client *oanda.Client, bc config.BtcCfdConfig, lastCycle *string, mu *sync.RWMutex) {
	wait := time.Until(nextM5Boundary(time.Now()))
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		if err := tick(ctx, client, bc, lastCycle, mu); err != nil && ctx.Err() == nil {
			slog.Warn("btc_cfd cycle", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func tick(ctx context.Context, client *oanda.Client, bc config.BtcCfdConfig, lastCycle *string, mu *sync.RWMutex) error {
	resp, err := client.Candles(ctx, bc.Instrument, bc.Granularity, bc.CandleCount)
	if err != nil {
		return err
	}
	n := len(resp.Candles)
	detail := ""
	if n > 0 {
		last := resp.Candles[n-1]
		detail = last.Time
	}
	slog.Info("btc_cfd cycle tick",
		"instrument", bc.Instrument,
		"granularity", bc.Granularity,
		"candles", n,
		"last_candle", detail,
	)
	mu.Lock()
	*lastCycle = fmtDetail(bc, n, detail)
	mu.Unlock()
	return nil
}

func fmtDetail(bc config.BtcCfdConfig, n int, lastTime string) string {
	if lastTime == "" {
		return bc.Instrument + " " + bc.Granularity + " (no candles)"
	}
	return bc.Instrument + " " + bc.Granularity + ": " + lastTime + " (" + strconv.Itoa(n) + " candles)"
}

func nextM5Boundary(now time.Time) time.Time {
	utc := now.UTC()
	trunc := utc.Truncate(5 * time.Minute)
	if trunc.Equal(utc) {
		return trunc
	}
	return trunc.Add(5 * time.Minute)
}
