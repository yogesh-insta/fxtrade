package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/schedule"
	"github.com/ym/fxtrade/internal/sentiment"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

type Engine struct {
	cfg        *config.Config
	instrument string
	client     *oanda.Client
	exec       *execution.Executor
	risk       *risk.Manager
	notify     notify.Notifier
	sentiment  *sentiment.Cache
	journal    *journal.Writer
	perfStore  *sqlite.Store
	botID      string
	modeMu           sync.RWMutex
	lastMode         string
	lastTrendAttempt time.Time
	cycleSummary     CycleSummary
}

func NewEngine(cfg *config.Config, instrument string, client *oanda.Client, exec *execution.Executor, rm *risk.Manager, n notify.Notifier, sc *sentiment.Cache) *Engine {
	if instrument == "" {
		instrument = oanda.DefaultInstrument
	}
	return &Engine{
		cfg:        cfg,
		instrument: instrument,
		client:     client,
		exec:       exec,
		risk:       rm,
		notify:     n,
		sentiment:  sc,
		journal:    journal.New(cfg.Strategy.JournalDir),
	}
}

func (e *Engine) Instrument() string {
	return e.instrument
}

// SetPerformanceStore records strategy decisions to SQLite for later analysis.
func (e *Engine) SetPerformanceStore(botID string, s *sqlite.Store) {
	e.botID = botID
	e.perfStore = s
}

// RunAll runs all engines on a shared ticker, cycling them sequentially each
// tick so account-wide checks (1 open position, correlation guard) cannot race.
// onTickOK is called when every engine completes a successful cycle in a tick.
func RunAll(ctx context.Context, engines []*Engine, cfg *config.Config, onTickOK func()) {
	if len(engines) == 0 {
		return
	}
	interval := time.Duration(cfg.Strategy.CycleMinutes) * time.Minute
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	instruments := make([]string, len(engines))
	for i, e := range engines {
		instruments[i] = e.instrument
	}
	slog.Info("strategy engines configured", "interval", interval, "enabled", cfg.Strategy.Enabled, "instruments", instruments)
	schedule.RunPeriodic(ctx, "strategy engines", interval, func(cycleCtx context.Context) {
		if runAllOnce(cycleCtx, engines) && onTickOK != nil {
			onTickOK()
		}
	})
}

func runAllOnce(ctx context.Context, engines []*Engine) bool {
	allOK := true
	for _, e := range engines {
		if ctx.Err() != nil {
			return false
		}
		if !e.runCycle(ctx) {
			allOK = false
		}
	}
	return allOK
}

func (e *Engine) RunOnce(ctx context.Context) bool {
	return e.runCycle(ctx)
}

func (e *Engine) runCycle(ctx context.Context) bool {
	if !e.cfg.Strategy.Enabled {
		return false
	}

	now := time.Now()
	e.cycleSummary = CycleSummary{
		Instrument:   e.instrument,
		Mode:         e.LastMode(),
		Action:       "no_trade",
		IntervalMins: e.cfg.Strategy.CycleMinutes,
	}
	if e.cycleSummary.IntervalMins <= 0 {
		e.cycleSummary.IntervalMins = 30
	}

	if e.risk.IsHalted() {
		e.setMode(ModeStandAside)
		e.cycleSummary.Mode = ModeStandAside
		e.cycleSummary.Reason = "kill switch active"
		e.logDecision(ModeStandAside, "no_trade", e.cycleSummary.Reason, nil)
		e.sendCycleReport(ctx, market.Snapshot{}, RangeBand{}, sentiment.SentimentSignal{}, false)
		return false
	}

	snap, err := market.LoadSnapshot(ctx, e.client, e.instrument)
	if err != nil {
		slog.Error("strategy snapshot", "instrument", e.instrument, "error", err)
		e.abortCycle(ctx, market.Snapshot{}, RangeBand{}, fmt.Sprintf("market data unavailable: %v", err))
		return false
	}

	sig, hasSig, history := e.currentSentiment(now)

	if ok, reason := CheckMarketConditions(snap, sig, hasSig, e.cfg.Risk, now); !ok {
		e.cycleSummary.Mode = e.LastMode()
		if e.cycleSummary.Mode == "" {
			e.cycleSummary.Mode = ModeStandAside
		}
		e.cycleSummary.Reason = reason
		e.logDecision(e.cycleSummary.Mode, "no_trade", reason, nil)
		band := DetectRange(snap, e.cfg.RangeMode)
		e.sendCycleReport(ctx, snap, band, sig, hasSig)
		return true
	}

	band := DetectRange(snap, e.cfg.RangeMode)
	mode := DetectMode(snap, band, e.cfg.TrendMode)
	if prev := e.LastMode(); prev != "" && prev != mode {
		e.cycleSummary.ModeChanged = fmt.Sprintf("%s → %s", prev, mode)
		e.logDecision(mode, "mode_change", fmt.Sprintf("%s -> %s", prev, mode), map[string]any{
			"range_valid": band.Valid,
			"width_pips":  band.WidthPips,
		})
		if e.cfg.RangeMode.CancelLimitsOnModeChange && mode != ModeRange {
			e.cancelAllPending(ctx, "mode_change")
		}
	}
	e.setMode(mode)
	e.cycleSummary.Mode = mode

	openTrades, pending, err := e.fetchState(ctx)
	if err != nil {
		slog.Error("strategy state", "instrument", e.instrument, "error", err)
		e.abortCycle(ctx, snap, band, fmt.Sprintf("could not read open trades/orders: %v", err))
		return false
	}

	ownTrades, _ := splitTradesByInstrument(openTrades, e.instrument)

	if len(ownTrades) > 0 {
		e.cycleSummary.Action = "manage_position"
		e.cycleSummary.Reason = fmt.Sprintf("monitoring %d open position(s)", len(ownTrades))
		e.handleOpenTrade(ctx, snap, band, mode, ownTrades, pending)
		e.sendCycleReport(ctx, snap, band, sig, hasSig)
		return true
	}

	// Per-bot allocation: do not block this engine on other bots' positions.
	openCount := len(ownTrades)

	switch mode {
	case ModeStandAside:
		e.cancelAllPending(ctx, "stand_aside")
		e.cycleSummary.Reason = "no valid range or trend"
		e.logDecision(mode, "no_trade", e.cycleSummary.Reason, map[string]any{"range_valid": band.Valid})
	case ModeRange:
		e.runRangeMode(ctx, snap, band, sig, hasSig, pending, openCount)
	case ModeTrend:
		e.cancelAllPending(ctx, "trend_mode")
		e.runTrendMode(ctx, snap, band, sig, hasSig, history, openCount)
	}

	e.sendCycleReport(ctx, snap, band, sig, hasSig)
	return true
}

func (e *Engine) sendCycleReport(ctx context.Context, snap market.Snapshot, band RangeBand, sig sentiment.SentimentSignal, hasSig bool) {
	subject := CycleEmailSubject(e.cycleSummary)
	body := FormatCycleEmail(snap, band, e.cfg, e.cycleSummary, sig, hasSig)
	notify.SendDigest(e.notify, ctx, "strategy:"+e.instrument, subject, body)
}

func (e *Engine) abortCycle(ctx context.Context, snap market.Snapshot, band RangeBand, reason string) {
	sig, hasSig, _ := e.currentSentiment(time.Now())
	e.cycleSummary.Reason = reason
	if e.cycleSummary.Mode == "" {
		e.cycleSummary.Mode = e.LastMode()
	}
	if e.cycleSummary.Mode == "" {
		e.cycleSummary.Mode = ModeStandAside
	}
	e.sendCycleReport(ctx, snap, band, sig, hasSig)
}

func (e *Engine) currentSentiment(now time.Time) (sentiment.SentimentSignal, bool, []sentiment.SentimentSignal) {
	if e.sentiment == nil {
		return sentiment.SentimentSignal{}, false, nil
	}
	sig, hasSig := e.sentiment.Current(now)
	return sig, hasSig, e.sentiment.History()
}

func (e *Engine) setMode(mode string) {
	e.modeMu.Lock()
	e.lastMode = mode
	e.modeMu.Unlock()
}

func (e *Engine) runRangeMode(ctx context.Context, snap market.Snapshot, band RangeBand, sig sentiment.SentimentSignal, hasSig bool, pending []oanda.PendingOrder, openCount int) {
	if !band.Valid {
		e.cancelAllPending(ctx, "invalid_range")
		e.cycleSummary.Reason = "range invalid"
		e.logDecision(ModeRange, "no_trade", e.cycleSummary.Reason, nil)
		return
	}
	if !e.cfg.RangeMode.PendingLimitsEnabled {
		e.cycleSummary.Reason = "pending limits disabled in config"
		e.logDecision(ModeRange, "no_trade", e.cycleSummary.Reason, nil)
		return
	}

	summary, err := e.client.AccountSummary(ctx)
	if err != nil {
		e.cycleSummary.Reason = fmt.Sprintf("account summary unavailable: %v", err)
		e.logDecision(ModeRange, "no_trade", e.cycleSummary.Reason, nil)
		return
	}
	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	balance = risk.EffectiveCapital(e.cfg.Risk.AllocatedCapitalUSD, balance)
	stopDist := snap.ATR14Daily * e.cfg.RangeMode.StopATRBeyondBoundary
	conf := SentimentConfidence("LONG", sig, hasSig, e.cfg.LLMGate)
	units, sizeErr := e.entryUnits(balance, stopDist, conf)
	if sizeErr != nil {
		e.cycleSummary.Reason = sizeErr.Error()
		e.logDecision(ModeRange, "no_trade", e.cycleSummary.Reason, nil)
		return
	}

	midTP := band.Mid
	hasBuy, hasSell := classifyPending(pending, e.instrument)

	// Correlation guard: block same-direction pending on another USD pair.
	longElsewhere, shortElsewhere := pendingDirectionsElsewhere(pending, e.instrument)

	if !hasBuy && longElsewhere {
		e.cycleSummary.Reason = "buy limit skipped — same-direction limit pending on another USD pair (correlation guard)"
		e.logDecision(ModeRange, "no_trade", e.cycleSummary.Reason, nil)
	}
	if !hasBuy && !longElsewhere && !SentimentVetoBuy(sig, hasSig, e.cfg.LLMGate) && snap.Ask > band.BuyLimitPrice {
		sl := band.Low - stopDist
		corr := fmt.Sprintf("range-buy-%s-%d", e.instrument, time.Now().UnixNano())
		req := risk.EntryRequest{
			CorrelationID:  corr,
			SpreadPips:     snap.SpreadPips,
			OpenPositions:  openCount,
			Confidence:     conf,
			AccountBalance: balance,
			StopDistance:   stopDist,
		}
		_, err := e.exec.PlaceLimit(ctx, req, execution.LimitOrderParams{
			Instrument:  e.instrument,
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
			e.cycleSummary.Action = "place_buy_limit"
			e.cycleSummary.Reason = fmt.Sprintf("buy limit at support %s", oanda.FormatPrice(band.BuyLimitPrice))
			e.logDecision(ModeRange, "place_buy_limit", "support limit placed", map[string]any{
				"price": band.BuyLimitPrice, "units": units,
			})
		}
	}

	if !hasSell && shortElsewhere {
		e.cycleSummary.Reason = "sell limit skipped — same-direction limit pending on another USD pair (correlation guard)"
		e.logDecision(ModeRange, "no_trade", e.cycleSummary.Reason, nil)
	}
	if !hasSell && !shortElsewhere && !SentimentVetoSell(sig, hasSig, e.cfg.LLMGate) && snap.Bid < band.SellLimitPrice {
		sl := band.High + stopDist
		corr := fmt.Sprintf("range-sell-%s-%d", e.instrument, time.Now().UnixNano())
		req := risk.EntryRequest{
			CorrelationID:  corr,
			SpreadPips:     snap.SpreadPips,
			OpenPositions:  openCount,
			Confidence:     SentimentConfidence("SHORT", sig, hasSig, e.cfg.LLMGate),
			AccountBalance: balance,
			StopDistance:   stopDist,
		}
		_, err := e.exec.PlaceLimit(ctx, req, execution.LimitOrderParams{
			Instrument:  e.instrument,
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
			e.cycleSummary.Action = "place_sell_limit"
			e.cycleSummary.Reason = fmt.Sprintf("sell limit at resistance %s", oanda.FormatPrice(band.SellLimitPrice))
			e.logDecision(ModeRange, "place_sell_limit", "resistance limit placed", map[string]any{
				"price": band.SellLimitPrice, "units": units,
			})
		}
	}

	if hasBuy || hasSell {
		if e.cycleSummary.Action == "no_trade" {
			e.cycleSummary.Action = "pending_limits"
			e.cycleSummary.Reason = "range limits already working"
		}
		e.logDecision(ModeRange, "pending_limits", "range limits active", map[string]any{
			"has_buy": hasBuy, "has_sell": hasSell,
		})
	} else if e.cycleSummary.Action == "no_trade" {
		e.cycleSummary.Reason = "RANGE mode — waiting for price at support/resistance before placing limits"
	}
}

func (e *Engine) runTrendMode(ctx context.Context, snap market.Snapshot, band RangeBand, sig sentiment.SentimentSignal, hasSig bool, history []sentiment.SentimentSignal, openCount int) {
	if time.Since(e.lastTrendAttempt) < time.Hour {
		e.cycleSummary.Reason = fmt.Sprintf("trend entry on cooldown (%s until next attempt)", (time.Hour - time.Since(e.lastTrendAttempt)).Round(time.Minute))
		return
	}
	direction, ok, reason := TrendEntry(snap, band, e.cfg.TrendMode)
	if !ok {
		e.cycleSummary.Reason = reason
		e.logDecision(ModeTrend, "no_trade", reason, nil)
		return
	}
	if hasSig && sig.Direction != "FLAT" && sig.Direction != direction {
		e.cycleSummary.Reason = "sentiment disagrees with trend direction"
		e.logDecision(ModeTrend, "no_trade", e.cycleSummary.Reason, map[string]any{"sentiment": sig.Direction})
		return
	}
	if okPersist, persistReason := SentimentPersistence(direction, history, e.cfg.LLMGate); !okPersist && hasSig {
		e.cycleSummary.Reason = persistReason
		e.logDecision(ModeTrend, "no_trade", persistReason, nil)
		return
	}

	summary, err := e.client.AccountSummary(ctx)
	if err != nil {
		e.cycleSummary.Reason = fmt.Sprintf("account summary unavailable: %v", err)
		e.logDecision(ModeTrend, "no_trade", e.cycleSummary.Reason, nil)
		return
	}
	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	balance = risk.EffectiveCapital(e.cfg.Risk.AllocatedCapitalUSD, balance)
	stopDist := snap.ATR14Daily * e.cfg.TrendMode.ATRStopMultiplier
	conf := SentimentConfidence(direction, sig, hasSig, e.cfg.LLMGate)
	units, sizeErr := e.entryUnits(balance, stopDist, conf)
	if sizeErr != nil {
		e.cycleSummary.Reason = sizeErr.Error()
		e.logDecision(ModeTrend, "no_trade", e.cycleSummary.Reason, nil)
		return
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

	corr := fmt.Sprintf("trend-%s-%s-%d", e.instrument, direction, time.Now().UnixNano())
	req := risk.EntryRequest{
		CorrelationID:  corr,
		SpreadPips:     snap.SpreadPips,
		OpenPositions:  openCount,
		Confidence:     conf,
		AccountBalance: balance,
		StopDistance:   stopDist,
	}
	result, err := e.exec.PlaceMarket(ctx, req, execution.MarketOrderParams{
		Instrument: e.instrument,
		Direction:  direction,
		Units:      units,
		StopLoss:   sl,
	})
	if err != nil {
		e.cycleSummary.Reason = fmt.Sprintf("market order rejected: %v", err)
		e.logDecision(ModeTrend, "market_rejected", err.Error(), nil)
		return
	}
	e.lastTrendAttempt = time.Now()
	e.cycleSummary.Action = "market_entry"
	e.cycleSummary.Reason = fmt.Sprintf("%s market entry, trade %s, %d units", direction, result.TradeID, units)
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
					corr := fmt.Sprintf("tp1-%s-%s", e.instrument, t.ID)
					pl, err := e.exec.CloseTradeUnits(ctx, t.ID, corr, half)
					if err == nil {
						e.logDecision(mode, "tp1_partial", "closed 50% at range midpoint", map[string]any{
							"trade_id": t.ID, "entry": entry, "mid": mid, "realized_pl": pl,
						})
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

func classifyPending(orders []oanda.PendingOrder, instrument string) (hasBuy, hasSell bool) {
	for _, o := range orders {
		if o.Instrument != instrument {
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

// pendingDirectionsElsewhere reports pending limit directions on instruments
// other than the given one (used by the correlation guard).
func pendingDirectionsElsewhere(orders []oanda.PendingOrder, instrument string) (hasLong, hasShort bool) {
	for _, o := range orders {
		if o.Instrument == instrument {
			continue
		}
		u, _ := strconv.ParseInt(o.Units, 10, 64)
		if u > 0 {
			hasLong = true
		}
		if u < 0 {
			hasShort = true
		}
	}
	return hasLong, hasShort
}

func splitTradesByInstrument(trades []oanda.Trade, instrument string) (own, other []oanda.Trade) {
	for _, t := range trades {
		if t.Instrument == instrument {
			own = append(own, t)
		} else {
			other = append(other, t)
		}
	}
	return own, other
}

func (e *Engine) CancelAllPendingNow(ctx context.Context, reason string) {
	e.cancelAllPending(ctx, reason)
}

func (e *Engine) entryUnits(balance, stopDist, conf float64) (int64, error) {
	if stopDist < oanda.MinStopDistance() {
		return 0, fmt.Errorf("stop distance %.5f below minimum %d pips", stopDist, oanda.MinStopPips)
	}
	units := e.risk.SizeUnits(balance, stopDist, conf)
	if units <= 0 {
		return 0, fmt.Errorf("position size zero — stop too wide or balance too low for risk settings")
	}
	return units, nil
}

func (e *Engine) cancelAllPending(ctx context.Context, reason string) {
	pending, err := e.client.PendingOrders(ctx)
	if err != nil {
		return
	}
	for _, o := range pending.Orders {
		if o.Instrument != e.instrument {
			continue
		}
		corr := fmt.Sprintf("cancel-%s", o.ID)
		if err := e.exec.CancelOrder(ctx, o.ID, corr); err != nil {
			slog.Warn("cancel pending", "instrument", e.instrument, "order_id", o.ID, "error", err)
			continue
		}
		e.logDecision(e.LastMode(), "cancel_limit", reason, map[string]any{"order_id": o.ID})
	}
}

func (e *Engine) logDecision(mode, action, reason string, details map[string]any) {
	slog.Info("strategy decision", "instrument", e.instrument, "mode", mode, "action", action, "reason", reason)
	_ = e.journal.Append(journal.Entry{
		Instrument: e.instrument,
		Mode:       mode,
		Action:     action,
		Reason:     reason,
		Details:    details,
	})
	if e.perfStore != nil && e.botID != "" {
		_ = e.perfStore.InsertSignal(sqlite.Signal{
			At:         time.Now().UTC(),
			BotID:      e.botID,
			Instrument: e.instrument,
			Mode:       mode,
			Action:     action,
			Reason:     reason,
			Details:    details,
		})
	}
}

func (e *Engine) LastMode() string {
	e.modeMu.RLock()
	defer e.modeMu.RUnlock()
	return e.lastMode
}
