package monitor

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
)

type PositionMonitor struct {
	client         *oanda.Client
	notify         notify.Notifier
	risk           *risk.Manager
	interval       time.Duration
	mu             sync.RWMutex
	known          map[string]oanda.Trade
	recordedOpens  map[string]struct{}
	recordedCloses map[string]struct{}
	onTradeOpened  func(oanda.Trade)
	onTradeClosed  func(tradeID, correlationID string, pl float64)
	instruments    map[string]struct{}
}

func New(client *oanda.Client, rm *risk.Manager, n notify.Notifier, interval time.Duration, instruments ...string) *PositionMonitor {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	filter := make(map[string]struct{}, len(instruments))
	for _, inst := range instruments {
		inst = strings.ToUpper(strings.TrimSpace(inst))
		if inst == "" {
			continue
		}
		filter[inst] = struct{}{}
	}
	return &PositionMonitor{
		client:         client,
		notify:         n,
		risk:           rm,
		interval:       interval,
		known:          make(map[string]oanda.Trade),
		recordedOpens:  make(map[string]struct{}),
		recordedCloses: make(map[string]struct{}),
		instruments:    filter,
	}
}

// SetOnTradeOpened is called when a new open trade is detected (e.g. cancel
// opposite pending limits immediately).
func (m *PositionMonitor) SetOnTradeOpened(fn func(oanda.Trade)) {
	m.onTradeOpened = fn
}

// SetOnTradeClosed is called after realized P&L is recorded (journal/state).
func (m *PositionMonitor) SetOnTradeClosed(fn func(tradeID, correlationID string, pl float64)) {
	m.onTradeClosed = fn
}

// NoteTradeOpened records a trade open once (idempotent). Called by the
// executor on immediate fills so the monitor does not double-count.
// Seeds known with a stub so SL/TP before the next poll is still detected.
func (m *PositionMonitor) NoteTradeOpened(tradeID string) {
	if tradeID == "" {
		return
	}
	m.SeedOpenedTrade(oanda.Trade{ID: tradeID})
}

// SeedOpenedTrade registers a filled trade in known + recordedOpens so a
// fast SL/TP (open+close between polls) still triggers NoteTradeClosed.
// Idempotent: risk counters and onTradeOpened run at most once per trade ID.
func (m *PositionMonitor) SeedOpenedTrade(t oanda.Trade) {
	if t.ID == "" {
		return
	}
	m.mu.Lock()
	_, already := m.recordedOpens[t.ID]
	if !already {
		m.recordedOpens[t.ID] = struct{}{}
	}
	// Prefer richer snapshots (instrument/units/price) over ID-only stubs.
	if prev, ok := m.known[t.ID]; !ok || prev.Instrument == "" {
		m.known[t.ID] = t
	} else if t.Instrument != "" {
		m.known[t.ID] = t
	}
	m.mu.Unlock()

	if already {
		return
	}
	if m.risk != nil {
		m.risk.RecordTradeOpened()
	}
	if m.onTradeOpened != nil && t.Instrument != "" {
		m.onTradeOpened(t)
	}
}

// NoteTradeClosed records realized P&L once (idempotent).
func (m *PositionMonitor) NoteTradeClosed(tradeID string, pl float64) {
	if tradeID == "" || m.risk == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.recordedCloses[tradeID]; ok {
		return
	}
	m.recordedCloses[tradeID] = struct{}{}
	delete(m.recordedOpens, tradeID)
	m.risk.RecordTradeClosed(pl)
	if m.onTradeClosed != nil {
		m.onTradeClosed(tradeID, "", pl)
	}
}

// RecordPartialPL books realized P&L from a partial close without marking the
// trade as fully closed (the position may still be open).
func (m *PositionMonitor) RecordPartialPL(pl float64) {
	if m.risk == nil || pl == 0 {
		return
	}
	m.risk.RecordTradeClosed(pl)
}

// TradeSnapshot returns the last known open trade state (for close emails).
func (m *PositionMonitor) TradeSnapshot(tradeID string) (oanda.Trade, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.known[tradeID]
	return t, ok
}

func (m *PositionMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *PositionMonitor) poll(ctx context.Context) {
	resp, err := m.client.OpenTrades(ctx)
	if err != nil {
		slog.Warn("position monitor", "error", err)
		return
	}

	current := make(map[string]oanda.Trade, len(resp.Trades))
	for _, t := range resp.Trades {
		if !m.trackInstrument(t.Instrument) {
			continue
		}
		current[t.ID] = t
		m.mu.RLock()
		_, known := m.known[t.ID]
		_, alreadyOpened := m.recordedOpens[t.ID]
		m.mu.RUnlock()
		if !known && !alreadyOpened {
			units, _ := strconv.ParseInt(strings.TrimSpace(t.CurrentUnits), 10, 64)
			entry, _ := oanda.ParsePrice(t.Price)
			open := notify.TradeOpen{
				Instrument: t.Instrument,
				Units:      units,
				FillPrice:  entry,
				TradeID:    t.ID,
			}
			slog.Info("position opened",
				"trade_id", t.ID,
				"instrument", t.Instrument,
				"units", t.CurrentUnits,
				"price", t.Price,
			)
			m.notify.Send(ctx,
				notify.TradeOpenSubject(t.Instrument, oanda.TradeDirection(units), units),
				notify.FormatTradeOpen(open))
			// Seed before onTradeOpened so risk is counted once; SeedOpenedTrade
			// invokes onTradeOpened when instrument is set.
			m.SeedOpenedTrade(t)
		} else if alreadyOpened {
			// Refresh snapshot for an executor-seeded fill still open.
			m.mu.Lock()
			m.known[t.ID] = t
			m.mu.Unlock()
		}
	}

	candidates := m.closeCandidates(current)
	for id, prev := range candidates {
		pl, found, exitPrice, closedUnits := m.lookupClosedTrade(ctx, id)
		entry, _ := oanda.ParsePrice(prev.Price)
		openUnits, _ := strconv.ParseInt(strings.TrimSpace(prev.CurrentUnits), 10, 64)
		closeUnits := openUnits
		if closedUnits > 0 {
			closeUnits = closedUnits
		}
		if closeUnits < 0 {
			closeUnits = -closeUnits
		}

		closeEv := notify.TradeClose{
			Instrument: prev.Instrument,
			Direction:  oanda.TradeDirection(openUnits),
			Units:      closeUnits,
			EntryPrice: entry,
			ExitPrice:  exitPrice,
			RealizedPL: pl,
			PLKnown:    found,
			Reason:     "closed externally (stop loss / take profit / manual)",
			TradeID:    id,
		}
		if found {
			slog.Info("position closed externally", "trade_id", id, "instrument", prev.Instrument, "realized_pl", pl)
		} else {
			slog.Warn("position closed externally: realized P/L unavailable", "trade_id", id, "instrument", prev.Instrument)
		}
		m.notify.Send(ctx,
			notify.TradeCloseSubject(prev.Instrument, pl, found, false),
			notify.FormatTradeClose(closeEv))
		m.NoteTradeClosed(id, pl)
	}

	m.mu.Lock()
	// Keep fills seeded while this poll ran (or mid-poll) if still "open"
	// according to recordedOpens — next poll will close them if gone on OANDA.
	for id, t := range m.known {
		if _, inCurrent := current[id]; inCurrent {
			continue
		}
		if _, open := m.recordedOpens[id]; !open {
			continue
		}
		if _, closed := m.recordedCloses[id]; closed {
			continue
		}
		current[id] = t
	}
	m.known = current
	m.mu.Unlock()
}

// closeCandidates are trades we believe should have closed: previously known or
// executor-seeded opens that are no longer in the openTrades response.
func (m *PositionMonitor) closeCandidates(current map[string]oanda.Trade) map[string]oanda.Trade {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]oanda.Trade)
	add := func(id string, prev oanda.Trade) {
		if id == "" {
			return
		}
		if _, ok := current[id]; ok {
			return
		}
		if _, closed := m.recordedCloses[id]; closed {
			return
		}
		if prev.ID == "" {
			prev.ID = id
		}
		out[id] = prev
	}
	for id, prev := range m.known {
		add(id, prev)
	}
	for id := range m.recordedOpens {
		if _, already := out[id]; already {
			continue
		}
		if prev, ok := m.known[id]; ok {
			add(id, prev)
		} else {
			add(id, oanda.Trade{ID: id})
		}
	}
	return out
}

func (m *PositionMonitor) trackInstrument(instrument string) bool {
	if len(m.instruments) == 0 {
		return true
	}
	_, ok := m.instruments[strings.ToUpper(strings.TrimSpace(instrument))]
	return ok
}

func (m *PositionMonitor) lookupClosedTrade(ctx context.Context, tradeID string) (pl float64, plFound bool, exitPrice float64, units int64) {
	retryDelays := []time.Duration{3 * time.Second, 8 * time.Second, 15 * time.Second}
	for attempt, delay := range append([]time.Duration{0}, retryDelays...) {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return 0, false, 0, 0
			case <-time.After(delay):
			}
		}
		pl, plFound, exitPrice, units = m.lookupClosedTradeWindows(ctx, tradeID)
		if plFound || exitPrice > 0 {
			return pl, plFound, exitPrice, units
		}
		if attempt == len(retryDelays) {
			break
		}
	}
	return 0, false, 0, 0
}

func (m *PositionMonitor) lookupClosedTradeWindows(ctx context.Context, tradeID string) (pl float64, plFound bool, exitPrice float64, units int64) {
	windows := []time.Duration{
		15 * time.Minute,
		2 * time.Hour,
		24 * time.Hour,
		7 * 24 * time.Hour,
	}
	for _, window := range windows {
		since := time.Now().Add(-window)
		txs, err := m.client.TransactionsSinceAll(ctx, since)
		if err != nil {
			slog.Warn("position monitor: transactions lookup failed", "trade_id", tradeID, "window", window.String(), "error", err)
			continue
		}
		if p, ok := oanda.ClosedTradePL(txs, tradeID); ok {
			pl, plFound = p, true
		}
		if ep, u, ok := oanda.ClosedTradeDetails(txs, tradeID); ok {
			exitPrice, units = ep, u
		}
		if plFound || exitPrice > 0 {
			return pl, plFound, exitPrice, units
		}
	}
	return 0, false, 0, 0
}

// SeedOpenTrades registers pre-existing positions after startup reconciliation
// without incrementing daily trade counters (position may predate today).
func (m *PositionMonitor) SeedOpenTrades(trades []oanda.Trade) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range trades {
		m.known[t.ID] = t
		m.recordedOpens[t.ID] = struct{}{}
	}
}

func (m *PositionMonitor) OpenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.known)
}
