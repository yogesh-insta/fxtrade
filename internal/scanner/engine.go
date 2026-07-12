package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/store"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

type Engine struct {
	cfg      *config.Config
	client   *oanda.Client
	exec     *execution.Executor
	rm       *risk.Manager
	scanner  *Scanner
	notify   *Notifier
	universe Universe
	botID    string

	mu               sync.Mutex
	lastDailyNotify  time.Time
	lastWeeklyNotify time.Time
	tradesToday      int
	dayKey           string
	sessionEntered   map[string]time.Time
	perfStore        *sqlite.Store
	metaStore        *store.MetaStore
	onCycleOK        func()
}

func NewEngine(cfg *config.Config, client *oanda.Client, exec *execution.Executor, rm *risk.Manager, n *Notifier, universe Universe) *Engine {
	return &Engine{
		cfg:            cfg,
		client:         client,
		exec:           exec,
		rm:             rm,
		scanner:        NewScanner(cfg, client, universe),
		notify:         n,
		universe:       universe,
		sessionEntered: make(map[string]time.Time),
	}
}

func (e *Engine) SetBotID(id string) {
	e.botID = id
}

func (e *Engine) SetPerformanceStore(s *sqlite.Store, meta *store.MetaStore) {
	e.perfStore = s
	e.metaStore = meta
}

func (e *Engine) SetOnCycleOK(fn func()) {
	e.onCycleOK = fn
}

func (e *Engine) Run(ctx context.Context) {
	interval := time.Duration(e.cfg.Scanner.PollSeconds) * time.Second
	if interval <= 0 {
		interval = 10 * time.Second
	}
	e.notify.DaemonStarted(ctx, e.universe.Symbols, e.cfg.Scanner.PollSeconds)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	e.cycle(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.cycle(ctx)
		}
	}
}

func (e *Engine) cycle(ctx context.Context) {
	if e.rm.IsHalted() {
		return
	}

	cycleOK := false
	defer func() {
		if cycleOK && e.onCycleOK != nil {
			e.onCycleOK()
		}
	}()

	now := time.Now().UTC()
	e.mu.Lock()
	if e.dayKey != now.Format("2006-01-02") {
		e.dayKey = now.Format("2006-01-02")
		e.tradesToday = 0
	}
	e.mu.Unlock()

	balance, err := AccountBalance(ctx, e.cfg, e.client)
	if err != nil {
		slog.Warn("account balance", "error", err)
		return
	}

	e.checkWeeklyProfitTarget(ctx, balance)
	e.maybeScheduledSummaries(ctx, balance)

	if err := e.maybeForceFlat(ctx); err != nil {
		slog.Warn("force flat", "error", err)
	}

	open, err := e.client.OpenTrades(ctx)
	if err != nil {
		slog.Warn("open trades", "error", err)
		return
	}
	cycleOK = true
	if e.hasScannerOpenTrade(open.Trades) {
		return
	}

	setups := e.scanner.ScanAll(ctx, e.universe.Symbols)
	ranked := RankSetups(setups, e.cfg.Scanner.MinSetupScore)
	if len(ranked) == 0 {
		slog.Debug("scanner: no setups above threshold")
		e.recordSignal("no_setup", "", "", 0, nil)
		return
	}

	top := ranked[0]
	if top.BreakoutDirection == "" {
		prices := e.scanner.PricingMap(ctx, []string{top.Instrument})
		tick, ok := prices[top.Instrument]
		if !ok {
			return
		}
		top.BreakoutDirection = DetectBreakout(top, tick.Bid, tick.Ask)
	}
	if top.BreakoutDirection == "" {
		e.recordSignal("await_breakout", top.Instrument, "", top.Score, map[string]any{
			"setup_score": top.Score,
		})
		return
	}

	if e.cfg.Scanner.RequireTrendAlignment && !BreakoutAlignedWithTrend(top.BreakoutDirection, top.TrendBias) {
		e.recordSignal("counter_trend_breakout", top.Instrument, top.BreakoutDirection, top.Score, map[string]any{
			"trend_bias": top.TrendBias,
		})
		return
	}

	e.mu.Lock()
	if at, ok := e.sessionEntered[top.Instrument]; ok && at.Equal(top.Range.SessionOpen) {
		e.mu.Unlock()
		e.recordSignal("session_already_traded", top.Instrument, top.BreakoutDirection, top.Score, nil)
		return
	}
	e.mu.Unlock()

	if err := e.enter(ctx, top, ranked[1:], balance); err != nil {
		slog.Warn("entry failed", "instrument", top.Instrument, "error", err)
		return
	}

	e.mu.Lock()
	e.sessionEntered[top.Instrument] = top.Range.SessionOpen
	e.tradesToday++
	e.mu.Unlock()
}

func (e *Engine) enter(ctx context.Context, setup Setup, runnersUp []Setup, balance float64) error {
	meta := e.universe.Meta[setup.Instrument]
	pipSize := oanda.InstrumentPipSize(setup.Instrument, meta.PipLocation)
	stopDistance := stopDistanceForClass(e.cfg.Scanner, setup.Class, pipSize)
	if stopDistance <= 0 {
		return fmt.Errorf("invalid stop distance")
	}

	prices := e.scanner.PricingMap(ctx, []string{setup.Instrument})
	tick, ok := prices[setup.Instrument]
	if !ok {
		return fmt.Errorf("no live price")
	}

	entry := tick.Ask
	if setup.BreakoutDirection == execution.DirectionShort {
		entry = tick.Bid
	}

	var stop, tp float64
	switch setup.BreakoutDirection {
	case execution.DirectionLong:
		stop = entry - stopDistance
		tp = entry + stopDistance*e.cfg.Scanner.TakeProfitRR
	case execution.DirectionShort:
		stop = entry + stopDistance
		tp = entry - stopDistance*e.cfg.Scanner.TakeProfitRR
	default:
		return fmt.Errorf("unknown direction %q", setup.BreakoutDirection)
	}

	units := sizeUnits(balance, e.cfg.Scanner.RiskPerTradePct, stopDistance)
	if units <= 0 {
		return fmt.Errorf("size units zero")
	}

	open, _ := e.client.OpenTrades(ctx)
	openCount := 0
	if open != nil && e.hasScannerOpenTrade(open.Trades) {
		openCount = 1
	}

	corrID := fmt.Sprintf("scan-%s-%d", setup.Instrument, time.Now().UnixNano())
	if e.botID != "" {
		corrID = e.botID + ":" + corrID
	}
	req := risk.EntryRequest{
		CorrelationID:  corrID,
		SpreadPips:     setup.SpreadPips,
		OpenPositions:  openCount,
		Confidence:     1.0,
		AccountBalance: balance,
		StopDistance:   stopDistance,
	}

	e.notify.TradeEntry(ctx, setup, runnersUp, balance, units, stop, tp)

	result, err := e.exec.PlaceMarket(ctx, req, execution.MarketOrderParams{
		Instrument:     setup.Instrument,
		Direction:      setup.BreakoutDirection,
		Units:          units,
		StopLoss:       stop,
		TakeProfit:     &tp,
		ClientOrderID:  corrID,
		ClientOrderTag: e.botID,
	})
	if err != nil {
		return err
	}
	if e.metaStore != nil && result.TradeID != "" {
		e.metaStore.Put(result.TradeID, store.TradeMeta{
			Instrument:    setup.Instrument,
			CorrelationID: corrID,
			Direction:     setup.BreakoutDirection,
			SignalPrice:   entry,
			FillPrice:     result.FillPrice,
			StopLoss:      stop,
			TakeProfit:    tp,
			Units:         result.Units,
			SetupScore:    setup.Score,
			OpenedAt:      time.Now().UTC(),
		})
	}
	e.recordSignal("entry_taken", setup.Instrument, setup.BreakoutDirection, setup.Score, map[string]any{
		"correlation_id": corrID,
		"trade_id":       result.TradeID,
		"units":          result.Units,
		"stop_loss":      stop,
		"take_profit":    tp,
		"runners_up":     len(runnersUp),
	})
	return nil
}

func (e *Engine) maybeForceFlat(ctx context.Context) error {
	trades, err := e.client.OpenTrades(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, t := range trades.Trades {
		meta, ok := e.universe.Meta[t.Instrument]
		if !ok {
			continue
		}
		if !ShouldForceFlat(e.cfg.Scanner, t.Instrument, meta.Type, now) {
			continue
		}
		_, err := e.exec.CloseTrade(ctx, t.ID, "force_flat_"+t.Instrument)
		if err != nil {
			slog.Warn("force flat close failed", "trade", t.ID, "error", err)
			continue
		}
		slog.Info("force flat", "instrument", t.Instrument, "trade", t.ID)
	}
	return nil
}

func (e *Engine) checkWeeklyProfitTarget(ctx context.Context, balance float64) {
	target := e.cfg.Scanner.WeeklyProfitTargetPct
	if target <= 0 || e.rm.WeeklyProfitNotified() {
		return
	}
	snap := e.rm.Snapshot()
	if snap.WeeklyPnL >= balance*target/100 {
		e.notify.WeeklyTargetReached(ctx, snap.WeeklyPnL, target, balance)
		e.rm.MarkWeeklyProfitNotified()
	}
}

func (e *Engine) maybeScheduledSummaries(ctx context.Context, balance float64) {
	snap := e.rm.Snapshot()
	now := time.Now().UTC()
	window := 2 * time.Minute

	if shouldFireScheduled(e.lastDailyNotify, e.cfg.Notifications.DailySummaryUTC, now, window) {
		e.notify.DailySummary(ctx, snap.DailyPnL, snap.WeeklyPnL, balance, e.tradesToday)
		e.lastDailyNotify = now
	}
	if shouldFireScheduled(e.lastWeeklyNotify, e.cfg.Notifications.WeeklySummaryUTC, now, window) {
		e.notify.WeeklySummary(ctx, snap.WeeklyPnL, balance, e.tradesToday)
		e.lastWeeklyNotify = now
	}
}

func (e *Engine) recordSignal(action, instrument, direction string, score float64, details map[string]any) {
	if e.perfStore == nil || e.botID == "" {
		return
	}
	_ = e.perfStore.InsertSignal(sqlite.Signal{
		At:         time.Now().UTC(),
		BotID:      e.botID,
		Instrument: instrument,
		Action:     action,
		Direction:  direction,
		SetupScore: score,
		Details:    details,
	})
}

func stopDistanceForClass(cfg config.ScannerConfig, cls string, pipSize float64) float64 {
	switch cls {
	case "CRYPTO":
		return cfg.StopLossPointsCrypto * pipSize
	case "INDEX", "ENERGY":
		return cfg.StopLossPointsIndex * pipSize
	default:
		return cfg.StopLossPipsFX * pipSize
	}
}

func sizeUnits(balance, riskPct, stopDistance float64) int64 {
	if balance <= 0 || stopDistance <= 0 {
		return 0
	}
	riskAmount := balance * riskPct / 100
	units := riskAmount / stopDistance
	if units < 1 {
		return 0
	}
	return int64(math.Floor(units))
}

func AccountBalance(ctx context.Context, cfg *config.Config, client *oanda.Client) (float64, error) {
	switch cfg.Scanner.BalanceSource {
	case "config":
		if cfg.Scanner.AccountBalanceUSD > 0 {
			return cfg.Scanner.AccountBalanceUSD, nil
		}
	}

	summary, err := client.AccountSummary(ctx)
	if err != nil {
		if cfg.Scanner.AccountBalanceUSD > 0 {
			return cfg.Scanner.AccountBalanceUSD, nil
		}
		return 0, err
	}

	field := summary.Account.NAV
	if cfg.Scanner.BalanceSource == "oanda_balance" {
		field = summary.Account.Balance
	}
	v, err := oanda.ParsePrice(field)
	if err != nil {
		return cfg.Scanner.AccountBalanceUSD, err
	}
	if v <= 0 && cfg.Scanner.AccountBalanceUSD > 0 {
		return cfg.Scanner.AccountBalanceUSD, nil
	}
	return v, nil
}

// DryRun performs one scan cycle without placing orders.
func (e *Engine) DryRun(ctx context.Context) []Setup {
	setups := e.scanner.ScanAll(ctx, e.universe.Symbols)
	return RankSetups(setups, e.cfg.Scanner.MinSetupScore)
}

func (e *Engine) hasScannerOpenTrade(trades []oanda.Trade) bool {
	for _, t := range trades {
		if t.ClientExtensions != nil && t.ClientExtensions.Tag == e.botID && e.botID != "" {
			return true
		}
	}
	return false
}
