package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
)

type PositionMonitor struct {
	client   *oanda.Client
	notify   notify.Notifier
	interval time.Duration
	known    map[string]oanda.Trade
}

func New(client *oanda.Client, n notify.Notifier, interval time.Duration) *PositionMonitor {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &PositionMonitor{
		client:   client,
		notify:   n,
		interval: interval,
		known:    make(map[string]oanda.Trade),
	}
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
		current[t.ID] = t
		if prev, ok := m.known[t.ID]; !ok {
			slog.Info("position opened",
				"trade_id", t.ID,
				"instrument", t.Instrument,
				"units", t.CurrentUnits,
				"price", t.Price,
			)
			m.notify.Send(ctx, "fxtrade: position detected",
				fmt.Sprintf("trade_id=%s\ninstrument=%s\nunits=%s\nprice=%s\n", t.ID, t.Instrument, t.CurrentUnits, t.Price))
		} else if prev.UnrealizedPL != t.UnrealizedPL {
			slog.Debug("position update", "trade_id", t.ID, "unrealized_pl", t.UnrealizedPL)
		}
	}

	for id, prev := range m.known {
		if _, ok := current[id]; !ok {
			slog.Info("position closed externally", "trade_id", id, "last_units", prev.CurrentUnits)
			m.notify.Send(ctx, "fxtrade: position closed externally",
				fmt.Sprintf("trade_id=%s\ninstrument=%s\n", id, prev.Instrument))
		}
	}

	m.known = current
}

func (m *PositionMonitor) OpenCount() int {
	return len(m.known)
}
