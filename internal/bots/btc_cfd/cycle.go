package btc_cfd

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
)

type cycleEngine struct {
	client    *oanda.Client
	bc        config.BtcCfdConfig
	exec      *execution.Executor
	rm        *risk.Manager
	lastCycle *string
	mu        *sync.RWMutex

	reconciled  bool
	dayKey      string
	tradesToday int
}

func runCycle(ctx context.Context, e *cycleEngine) {
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
		if err := e.tick(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("btc_cfd cycle", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (e *cycleEngine) tick(ctx context.Context) error {
	if e.rm.IsHalted() {
		e.setDetail("halted")
		return nil
	}

	e.rollDay()

	if !e.reconciled {
		if err := e.reconcileStartup(ctx); err != nil {
			return err
		}
	}

	resp, err := e.client.Candles(ctx, e.bc.Instrument, e.bc.Granularity, e.bc.CandleCount)
	if err != nil {
		return err
	}

	ind, err := ComputeIndicators(resp.Candles, e.bc)
	if err != nil {
		e.setDetail(fmt.Sprintf("%s: %v", e.bc.Instrument, err))
		slog.Debug("btc_cfd indicators", "error", err)
		return nil
	}

	openTrades, err := e.openInstrumentTrades(ctx)
	if err != nil {
		return err
	}
	hasOpen := len(openTrades) > 0

	pricing, err := e.client.Pricing(ctx, e.bc.Instrument)
	if err != nil {
		return err
	}
	if len(pricing.Prices) == 0 {
		return fmt.Errorf("no pricing for %s", e.bc.Instrument)
	}
	tick, err := pricing.Prices[0].ToUpdate()
	if err != nil {
		return err
	}
	bid, ask, spread := tick.Bid, tick.Ask, tick.Spread

	summary, err := e.client.AccountSummary(ctx)
	if err != nil {
		return err
	}
	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	riskSnap := e.rm.Snapshot()

	sig := EvaluateSignal(ind, e.bc, EntryLimits{
		HasOpenPosition: hasOpen,
		TradesToday:     e.tradesToday,
		DailyPnL:        riskSnap.DailyPnL,
		AccountBalance:  balance,
		SpreadUSD:       spread,
	})

	detail := fmt.Sprintf("%s RSI=%.1f dev=%.2f ATR=%.2f spread=%.2f | %s",
		e.bc.Instrument, ind.RSI, ind.Deviation, ind.ATR, spread, sig.Reason)
	e.setDetail(detail)

	slog.Info("btc_cfd cycle",
		"instrument", e.bc.Instrument,
		"rsi", ind.RSI,
		"prev_rsi", ind.PrevRSI,
		"deviation", ind.Deviation,
		"atr", ind.ATR,
		"spread", spread,
		"signal", sig.Direction,
		"reason", sig.Reason,
	)

	if sig.Direction == "" {
		return nil
	}

	entry := ask
	if sig.Direction == execution.DirectionShort {
		entry = bid
	}
	if StaleSignal(ind.SignalPrice, entry, e.bc.MaxSlippagePct) {
		slog.Info("btc_cfd stale signal", "signal_price", ind.SignalPrice, "live", entry)
		return nil
	}

	stop, tp, ok, tpReason := StopTakeProfit(sig.Direction, entry, ind.ATR, e.bc)
	if !ok {
		slog.Info("btc_cfd skip entry", "reason", tpReason)
		return nil
	}

	stopDist := ind.ATR * e.bc.SLATRMultiple
	units := e.rm.SizeUnits(balance, stopDist, 1.0)
	if units <= 0 {
		slog.Info("btc_cfd skip entry", "reason", "position size below minimum")
		return nil
	}

	corrID := fmt.Sprintf("%s:%s:%s:%d", Meta.ID, e.bc.Instrument, sig.Direction, time.Now().UnixNano())
	req := risk.EntryRequest{
		CorrelationID:  corrID,
		SpreadPips:     0,
		OpenPositions:  len(openTrades),
		Confidence:     1.0,
		AccountBalance: balance,
		StopDistance:   stopDist,
	}

	result, err := e.exec.PlaceMarket(ctx, req, execution.MarketOrderParams{
		Instrument: e.bc.Instrument,
		Direction:  sig.Direction,
		Units:      units,
		StopLoss:   stop,
		TakeProfit: &tp,
	})
	if err != nil {
		return fmt.Errorf("place market: %w", err)
	}

	e.tradesToday++
	slog.Info("btc_cfd entry filled",
		"direction", sig.Direction,
		"trade_id", result.TradeID,
		"units", result.Units,
		"fill", result.FillPrice,
		"stop", stop,
		"tp", tp,
		"reason", sig.Reason,
	)
	e.setDetail(fmt.Sprintf("%s %s trade %s %d units", e.bc.Instrument, sig.Direction, result.TradeID, units))
	return nil
}

func (e *cycleEngine) reconcileStartup(ctx context.Context) error {
	open, err := e.openInstrumentTrades(ctx)
	if err != nil {
		return fmt.Errorf("startup reconcile: %w", err)
	}
	e.reconciled = true
	if len(open) > 0 {
		t := open[0]
		slog.Info("btc_cfd startup reconcile: existing position",
			"trade_id", t.ID,
			"units", t.CurrentUnits,
			"price", t.Price,
		)
		e.setDetail(fmt.Sprintf("%s: managing open trade %s", e.bc.Instrument, t.ID))
	} else {
		slog.Info("btc_cfd startup reconcile: flat")
	}
	return nil
}

func (e *cycleEngine) openInstrumentTrades(ctx context.Context) ([]oanda.Trade, error) {
	resp, err := e.client.OpenTrades(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]oanda.Trade, 0, len(resp.Trades))
	for _, t := range resp.Trades {
		if t.Instrument == e.bc.Instrument {
			out = append(out, t)
		}
	}
	return out, nil
}

func (e *cycleEngine) rollDay() {
	now := time.Now().UTC().Format("2006-01-02")
	if e.dayKey != now {
		e.dayKey = now
		e.tradesToday = 0
	}
}

func (e *cycleEngine) setDetail(s string) {
	e.mu.Lock()
	*e.lastCycle = s
	e.mu.Unlock()
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
