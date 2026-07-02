package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
)

type Engine struct {
	cfg      *config.Config
	client   *oanda.Client
	exec     *execution.Executor
	risk     *risk.Manager
	notify   notify.Notifier
	sentiment *sentiment.Cache
	journal  *journal.Writer
	lastMode string
	lastTrendAttempt time.Time
}

func NewEngine(cfg *config.Config, client *oanda.Client, exec *execution.Executor, rm *risk.Manager, n notify.Notifier, sc *sentiment.Cache) *Engine {
	return &Engine{
		cfg:       cfg,
		client:    client,
		exec:      exec,
		risk:      rm,
		notify:    n,
		sentiment: sc,
		journal:   journal.New(cfg.Strategy.JournalDir),
	}
}

func (e *Engine) Run(ctx context.Context) {
	interval := time.Duration(e.cfg.Strategy.CycleMinutes) * time.Minute
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	slog.Info("strategy engine started", "interval", interval, "enabled", e.cfg.Strategy.Enabled)
	e.runCycle(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runCycle(ctx)
		}
	}
}

func (e *Engine) RunOnce(ctx context.Context) {
	e.runCycle(ctx)
}

func (e *Engine) runCycle(ctx context.Context) {
	if !e.cfg.Strategy.Enabled {
		return
	}
	if e.risk.IsHalted() {
		e.logDecision(ModeStandAside, "no_trade", "kill switch active", nil)
		return
	}

	now := time.Now()
	snap, err := market.LoadSnapshot(ctx, e.client, oanda.DefaultInstrument)
	if err != nil {
		slog.Error("strategy snapshot", "error", err)
		return
	}

	sig, hasSig := e.sentiment.Current(now)
	history := e.sentiment.History()

	if ok, reason := CheckMarketConditions(snap, sig, hasSig, e.cfg.Risk, now); !ok {
		e.logDecision(e.lastMode, "no_trade", reason, nil)
		e.notify.Send(ctx, "fxtrade: no trade", "reason="+reason+"\n")
		return
	}

	band := DetectRange(snap, e.cfg.RangeMode)
	mode := DetectMode(snap, band, e.cfg.TrendMode)
	if e.lastMode != "" && e.lastMode != mode {
		e.logDecision(mode, "mode_change", fmt.Sprintf("%s -> %s", e.lastMode, mode), map[string]any{
			"range_valid": band.Valid,
			"width_pips":  band.WidthPips,
		})
		e.notify.Send(ctx, "fxtrade: mode change", fmt.Sprintf("from=%s\nto=%s\n", e.lastMode, mode))
		if e.cfg.RangeMode.CancelLimitsOnModeChange && mode != ModeRange {
			e.cancelAllPending(ctx, "mode_change")
		}
	}
	e.lastMode = mode

	openTrades, pending, err := e.fetchState(ctx)
	if err != nil {
		slog.Error("strategy state", "error", err)
		return
	}

	if len(openTrades) > 0 {
		e.handleOpenTrade(ctx, snap, band, mode, openTrades, pending)
		return
	}

	if len(openTrades) >= e.cfg.Risk.MaxOpenPositions {
		return
	}

	switch mode {
	case ModeStandAside:
		e.cancelAllPending(ctx, "stand_aside")
		e.logDecision(mode, "no_trade", "no valid range or trend", map[string]any{"range_valid": band.Valid})
		e.notify.Send(ctx, "fxtrade: stand aside", "no valid range or trend\n")
	case ModeRange:
		e.runRangeMode(ctx, snap, band, sig, hasSig, pending)
	case ModeTrend:
		e.cancelAllPending(ctx, "trend_mode")
		e.runTrendMode(ctx, snap, band, sig, hasSig, history)
	}
}

func (e *Engine) runRangeMode(ctx context.Context, snap market.Snapshot, band RangeBand, sig sentiment.SentimentSignal, hasSig bool, pending []oanda.PendingOrder) {
	if !band.Valid {
		e.cancelAllPending(ctx, "invalid_range")
		e.logDecision(ModeRange, "no_trade", "range invalid", nil)
		return
	}
	if !e.cfg.RangeMode.PendingLimitsEnabled {
		e.logDecision(ModeRange, "no_trade", "pending limits disabled in config", nil)
		return
	}

	summary, err := e.client.AccountSummary(ctx)
	if err != nil {
		return
	}
	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	stopDist := snap.ATR14Daily * e.cfg.RangeMode.StopATRBeyondBoundary
	conf := SentimentConfidence("LONG", sig, hasSig, e.cfg.LLMGate)
	units := e.risk.SizeUnits(balance, stopDist, conf)
	if units < 100 {
		units = 100
	}

	midTP := band.Mid
	hasBuy, hasSell := classifyPending(pending)

	if !hasBuy && !SentimentVetoBuy(sig, hasSig, e.cfg.LLMGate) && snap.Ask > band.BuyLimitPrice {
		sl := band.Low - stopDist
		corr := fmt.Sprintf("range-buy-%d", time.Now().UnixNano())
		req := risk.EntryRequest{
			CorrelationID:  corr,
			SpreadPips:     snap.SpreadPips,
			OpenPositions:  0,
			Confidence:     conf,
			AccountBalance: balance,
			StopDistance:   stopDist,
		}
		_, err := e.exec.PlaceLimit(ctx, req, execution.LimitOrderParams{
			Instrument:  oanda.DefaultInstrument,
			Direction:   execution.DirectionLong,
			Units:       units,
			Price:       band.BuyLimitPrice,
			StopLoss:    sl,
			TakeProfit:  &midTP,
			TimeInForce: e.cfg.RangeMode.PendingLimitTimeInForce,
		})
		if err != nil {
			e.logDecision(ModeRange, "limit_rejected", err.Error(), nil)
		} else {
			e.logDecision(ModeRange, "place_buy_limit", "support limit placed", map[string]any{
				"price": band.BuyLimitPrice, "units": units,
			})
		}
	}

	if !hasSell && !SentimentVetoSell(sig, hasSig, e.cfg.LLMGate) && snap.Bid < band.SellLimitPrice {
		sl := band.High + stopDist
		corr := fmt.Sprintf("range-sell-%d", time.Now().UnixNano())
		req := risk.EntryRequest{
			CorrelationID:  corr,
			SpreadPips:     snap.SpreadPips,
			OpenPositions:  0,
			Confidence:     SentimentConfidence("SHORT", sig, hasSig, e.cfg.LLMGate),
			AccountBalance: balance,
			StopDistance:   stopDist,
		}
		_, err := e.exec.PlaceLimit(ctx, req, execution.LimitOrderParams{
			Instrument:  oanda.DefaultInstrument,
			Direction:   execution.DirectionShort,
			Units:       units,
			Price:       band.SellLimitPrice,
			StopLoss:    sl,
			TakeProfit:  &midTP,
			TimeInForce: e.cfg.RangeMode.PendingLimitTimeInForce,
		})
		if err != nil {
			e.logDecision(ModeRange, "limit_rejected", err.Error(), nil)
		} else {
			e.logDecision(ModeRange, "place_sell_limit", "resistance limit placed", map[string]any{
				"price": band.SellLimitPrice, "units": units,
			})
		}
	}

	if hasBuy || hasSell {
		e.logDecision(ModeRange, "pending_limits", "range limits active", map[string]any{
			"has_buy": hasBuy, "has_sell": hasSell,
		})
	}
}

func (e *Engine) runTrendMode(ctx context.Context, snap market.Snapshot, band RangeBand, sig sentiment.SentimentSignal, hasSig bool, history []sentiment.SentimentSignal) {
	if time.Since(e.lastTrendAttempt) < time.Hour {
		return
	}
	direction, ok, reason := TrendEntry(snap, band, e.cfg.TrendMode)
	if !ok {
		e.logDecision(ModeTrend, "no_trade", reason, nil)
		e.notify.Send(ctx, "fxtrade: no trade (trend)", "reason="+reason+"\n")
		return
	}
	if hasSig && sig.Direction != "FLAT" && sig.Direction != direction {
		e.logDecision(ModeTrend, "no_trade", "sentiment disagrees with trend direction", map[string]any{"sentiment": sig.Direction})
		return
	}
	if okPersist, persistReason := SentimentPersistence(direction, history, e.cfg.LLMGate); !okPersist && hasSig {
		e.logDecision(ModeTrend, "no_trade", persistReason, nil)
		return
	}

	summary, _ := e.client.AccountSummary(ctx)
	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	stopDist := snap.ATR14Daily * e.cfg.TrendMode.ATRStopMultiplier
	conf := SentimentConfidence(direction, sig, hasSig, e.cfg.LLMGate)
	units := e.risk.SizeUnits(balance, stopDist, conf)
	if units < 100 {
		units = 100
	}

	var entry, sl float64
	switch direction {
	case execution.DirectionLong:
		entry = snap.Ask
		sl = entry - stopDist
	default:
		entry = snap.Bid
		sl = entry + stopDist
	}

	corr := fmt.Sprintf("trend-%s-%d", direction, time.Now().UnixNano())
	req := risk.EntryRequest{
		CorrelationID:  corr,
		SpreadPips:     snap.SpreadPips,
		OpenPositions:  0,
		Confidence:     conf,
		AccountBalance: balance,
		StopDistance:   stopDist,
	}
	result, err := e.exec.PlaceMarket(ctx, req, execution.MarketOrderParams{
		Instrument: oanda.DefaultInstrument,
		Direction:  direction,
		Units:      units,
		StopLoss:   sl,
	})
	if err != nil {
		e.logDecision(ModeTrend, "market_rejected", err.Error(), nil)
		return
	}
	e.lastTrendAttempt = time.Now()
	e.logDecision(ModeTrend, "market_entry", "trend entry", map[string]any{
		"direction": direction, "trade_id": result.TradeID, "units": units,
	})
}

func (e *Engine) handleOpenTrade(ctx context.Context, snap market.Snapshot, band RangeBand, mode string, trades []oanda.Trade, pending []oanda.PendingOrder) {
	if e.cfg.RangeMode.CancelOppositeOnFill && len(trades) > 0 && len(pending) > 0 {
		e.cancelAllPending(ctx, "position_open")
	}

	// Range TP1: close half at midpoint when price crosses
	if mode == ModeRange && band.Valid {
		for _, t := range trades {
			units, _ := strconv.ParseInt(t.CurrentUnits, 10, 64)
			if units == 0 {
				continue
			}
			entry, _ := oanda.ParsePrice(t.Price)
			mid := band.Mid
			long := units > 0
			crossed := (long && snap.Mid >= mid) || (!long && snap.Mid <= mid)
			absUnits := units
			if absUnits < 0 {
				absUnits = -absUnits
			}
			if crossed && absUnits > 100 {
				half := strconv.FormatInt(absUnits/2, 10)
				if half != "0" {
					_, err := e.client.CloseTrade(ctx, t.ID, half)
					if err == nil {
						e.logDecision(mode, "tp1_partial", "closed 50% at range midpoint", map[string]any{
							"trade_id": t.ID, "entry": entry, "mid": mid,
						})
						e.notify.Send(ctx, "fxtrade: TP1 partial close", fmt.Sprintf("trade_id=%s\nmid=%s\n", t.ID, oanda.FormatPrice(mid)))
					}
				}
			}
		}
	}
}

func (e *Engine) fetchState(ctx context.Context) ([]oanda.Trade, []oanda.PendingOrder, error) {
	trades, err := e.client.OpenTrades(ctx)
	if err != nil {
		return nil, nil, err
	}
	pending, err := e.client.PendingOrders(ctx)
	if err != nil {
		return nil, nil, err
	}
	return trades.Trades, pending.Orders, nil
}

func classifyPending(orders []oanda.PendingOrder) (hasBuy, hasSell bool) {
	for _, o := range orders {
		if o.Instrument != oanda.DefaultInstrument {
			continue
		}
		u, _ := strconv.ParseInt(o.Units, 10, 64)
		if u > 0 {
			hasBuy = true
		}
		if u < 0 {
			hasSell = true
		}
	}
	return hasBuy, hasSell
}

func (e *Engine) cancelAllPending(ctx context.Context, reason string) {
	pending, err := e.client.PendingOrders(ctx)
	if err != nil {
		return
	}
	for _, o := range pending.Orders {
		if o.Instrument != oanda.DefaultInstrument {
			continue
		}
		corr := fmt.Sprintf("cancel-%s", o.ID)
		if err := e.exec.CancelOrder(ctx, o.ID, corr); err != nil {
			slog.Warn("cancel pending", "order_id", o.ID, "error", err)
			continue
		}
		e.logDecision(e.lastMode, "cancel_limit", reason, map[string]any{"order_id": o.ID})
	}
}

func (e *Engine) logDecision(mode, action, reason string, details map[string]any) {
	slog.Info("strategy decision", "mode", mode, "action", action, "reason", reason)
	_ = e.journal.Append(journal.Entry{
		Mode:    mode,
		Action:  action,
		Reason:  reason,
		Details: details,
	})
}

func (e *Engine) LastMode() string {
	return e.lastMode
}
