package monitor

import (
	"context"
	"fmt"
	"log/slog"
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
func (m *PositionMonitor) NoteTradeOpened(tradeID string) {
	if tradeID == "" || m.risk == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.recordedOpens[tradeID]; ok {
		return
	}
	m.recordedOpens[tradeID] = struct{}{}
	m.risk.RecordTradeOpened()
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
		m.mu.RUnlock()
		if !known {
			slog.Info("position opened",
				"trade_id", t.ID,
				"instrument", t.Instrument,
				"units", t.CurrentUnits,
				"price", t.Price,
			)
			m.notify.Send(ctx, "fxtrade: position detected",
				fmt.Sprintf("trade_id=%s\ninstrument=%s\nunits=%s\nprice=%s\n", t.ID, t.Instrument, t.CurrentUnits, t.Price))
			m.NoteTradeOpened(t.ID)
			if m.onTradeOpened != nil {
				m.onTradeOpened(t)
			}
		}
	}

	m.mu.RLock()
	knownCopy := make(map[string]oanda.Trade, len(m.known))
	for id, t := range m.known {
		knownCopy[id] = t
	}
	m.mu.RUnlock()

	for id, prev := range knownCopy {
		if _, ok := current[id]; ok {
			continue
		}
		pl := m.lookupClosedPL(ctx, id)
		slog.Info("position closed externally", "trade_id", id, "instrument", prev.Instrument, "realized_pl", pl)
		m.notify.Send(ctx, "fxtrade: position closed externally",
			fmt.Sprintf("trade_id=%s\ninstrument=%s\nrealized_pl=%.2f\n", id, prev.Instrument, pl))
		m.NoteTradeClosed(id, pl)
	}

	m.mu.Lock()
	m.known = current
	m.mu.Unlock()
}

func (m *PositionMonitor) trackInstrument(instrument string) bool {
	if len(m.instruments) == 0 {
		return true
	}
	_, ok := m.instruments[strings.ToUpper(strings.TrimSpace(instrument))]
	return ok
}

func (m *PositionMonitor) lookupClosedPL(ctx context.Context, tradeID string) float64 {
	since := time.Now().Add(-5 * time.Minute)
	txs, err := m.client.TransactionsSince(ctx, since)
	if err != nil {
		slog.Warn("position monitor: transactions lookup failed", "trade_id", tradeID, "error", err)
		return 0
	}
	if pl, ok := oanda.ClosedTradePL(txs.Transactions, tradeID); ok {
		return pl
	}
	return 0
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
