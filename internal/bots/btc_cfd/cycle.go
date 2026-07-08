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
	"github.com/ym/fxtrade/internal/monitor"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
)

type cycleEngine struct {
	client    *oanda.Client
	bc        config.BtcCfdConfig
	exec      *execution.Executor
	rm        *risk.Manager
	notifier  notify.Notifier
	posMon    *monitor.PositionMonitor
	meta      *tradeMetaStore
	lastCycle *string
	mu        *sync.RWMutex

	reconciled       bool
	apiFailures      int
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
			e.recordAPIFailure(ctx, err)
		} else if err == nil {
			e.apiFailures = 0
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

	if !e.reconciled {
		if err := e.reconcileStartup(ctx); err != nil {
			return err
		}
	}

	resp, err := e.client.Candles(ctx, e.bc.Instrument, e.bc.Granularity, candlesRequestCount(e.bc.CandleCount))
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

	if err := e.enforceMaxHold(ctx, openTrades); err != nil {
		slog.Warn("btc_cfd max-hold enforcement", "error", err)
	}

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
	balance = risk.EffectiveCapital(e.bc.AllocatedCapitalUSD, balance)
	riskSnap := e.rm.Snapshot()

	lim := EntryLimits{
		HasOpenPosition: hasOpen,
		TradesToday:     riskSnap.TradesToday,
		DailyPnL:        riskSnap.DailyPnL,
		AccountBalance:  balance,
		SpreadUSD:       spread,
		Tradeable:       tick.Tradeable,
	}
	if e.bc.M15Confirmation {
		if m15Close, m15EMA, err := e.m15Bias(ctx); err != nil {
			slog.Debug("btc_cfd m15 confirmation skipped", "error", err)
		} else {
			lim.M15Close = m15Close
			lim.M15EMA200 = m15EMA
		}
	}

	sig := EvaluateSignal(ind, e.bc, lim)

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
	units := PositionUnits(balance, stopDist, e.bc.PerTradeRiskPct)
	if units <= 0 {
		slog.Info("btc_cfd skip entry", "reason", "position size below minimum")
		return nil
	}

	clientOrderID := fmt.Sprintf("bc-%d", time.Now().UnixNano())
	if len(clientOrderID) > 50 {
		clientOrderID = clientOrderID[:50]
	}
	corrID := fmt.Sprintf("%s:%s:%s:%s", Meta.ID, e.bc.Instrument, sig.Direction, clientOrderID)
	req := risk.EntryRequest{
		CorrelationID:  corrID,
		SpreadPips:     0,
		OpenPositions:  len(openTrades),
		Confidence:     1.0,
		AccountBalance: balance,
		StopDistance:   stopDist,
	}

	result, err := e.exec.PlaceMarket(ctx, req, execution.MarketOrderParams{
		Instrument:     e.bc.Instrument,
		Direction:      sig.Direction,
		Units:          units,
		StopLoss:       stop,
		TakeProfit:     &tp,
		ClientOrderID:  clientOrderID,
		ClientOrderTag: Meta.ID,
	})
	if err != nil {
		return fmt.Errorf("place market: %w", err)
	}

	e.meta.Put(result.TradeID, tradeMeta{
		CorrelationID: corrID,
		Direction:     sig.Direction,
		SignalPrice:   ind.SignalPrice,
		FillPrice:     result.FillPrice,
		StopLoss:      stop,
		TakeProfit:    tp,
		Units:         result.Units,
		OpenedAt:      time.Now().UTC(),
	})

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

func (e *cycleEngine) m15Bias(ctx context.Context) (closePx, ema200 float64, err error) {
	resp, err := e.client.Candles(ctx, e.bc.Instrument, "M15", candlesRequestCount(e.bc.CandleCount))
	if err != nil {
		return 0, 0, err
	}
	ind, err := ComputeIndicators(resp.Candles, e.bc)
	if err != nil {
		return 0, 0, err
	}
	return ind.SignalPrice, ind.EMA200, nil
}

func (e *cycleEngine) enforceMaxHold(ctx context.Context, open []oanda.Trade) error {
	if e.bc.MaxHoldHours <= 0 || len(open) == 0 {
		return nil
	}
	maxAge := time.Duration(e.bc.MaxHoldHours * float64(time.Hour))
	for _, t := range open {
		openTime, err := time.Parse(time.RFC3339, t.OpenTime)
		if err != nil {
			continue
		}
		if time.Since(openTime) < maxAge {
			continue
		}
		corrID := Meta.ID + ":max-hold:" + t.ID
		slog.Info("btc_cfd max-hold close", "trade_id", t.ID, "open_time", t.OpenTime)
		e.notifier.Send(ctx, "fxtrade: max-hold close",
			fmt.Sprintf("trade_id=%s\ninstrument=%s\nmax_hold_hours=%.1f\n", t.ID, t.Instrument, e.bc.MaxHoldHours))
		if _, err := e.exec.CloseTrade(ctx, t.ID, corrID); err != nil {
			return err
		}
	}
	return nil
}

func (e *cycleEngine) reconcileStartup(ctx context.Context) error {
	open, err := e.openInstrumentTrades(ctx)
	if err != nil {
		return fmt.Errorf("startup reconcile: %w", err)
	}
	e.reconciled = true
	if len(open) > 1 {
		msg := fmt.Sprintf("btc_cfd startup: %d open %s trades (expected 0-1)", len(open), e.bc.Instrument)
		slog.Warn(msg)
		notify.SendRoutine(e.notifier, ctx, "fxtrade: reconciliation mismatch", msg)
	}
	if e.posMon != nil {
		e.posMon.SeedOpenTrades(open)
	}
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

func (e *cycleEngine) recordAPIFailure(ctx context.Context, err error) {
	e.apiFailures++
	slog.Warn("btc_cfd cycle", "error", err, "consecutive_failures", e.apiFailures)
	if e.bc.APIFailureAlertAfter > 0 && e.apiFailures >= e.bc.APIFailureAlertAfter {
		notify.SendRoutine(e.notifier, ctx, "fxtrade: repeated API failures",
			fmt.Sprintf("bot=%s\nfailures=%d\nlast_error=%v\n", Meta.ID, e.apiFailures, err))
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

// candlesRequestCount returns how many bars to fetch from OANDA. The latest
// candle in the response is usually the still-forming bar (incomplete), so we
// request one extra to ensure CandleCount complete bars for indicators.
func candlesRequestCount(complete int) int {
	return complete + 1
}

func nextM5Boundary(now time.Time) time.Time {
	utc := now.UTC()
	trunc := utc.Truncate(5 * time.Minute)
	if trunc.Equal(utc) {
		return trunc
	}
	return trunc.Add(5 * time.Minute)
}
