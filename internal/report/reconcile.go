package report

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

// ReconcileResult summarizes one bot's trade backfill from OANDA.
type ReconcileResult struct {
	BotID        string
	DBPath       string
	Attempted    int
	Reconciled   int
	StillMissing int
	Err          error
}

// ReconcileSummary aggregates reconciliation across bots.
type ReconcileSummary struct {
	Results         []ReconcileResult
	TotalReconciled int
}

func (s ReconcileSummary) AttemptedTotal() int {
	n := 0
	for _, r := range s.Results {
		n += r.Attempted
	}
	return n
}

func (s ReconcileSummary) StillMissingTotal() int {
	n := 0
	for _, r := range s.Results {
		n += r.StillMissing
	}
	return n
}

// ReconcileAllBots backfills missing P/L and instruments for every trading bot DB.
func ReconcileAllBots(ctx context.Context, cfg *config.Config, client *oanda.Client) ReconcileSummary {
	var summary ReconcileSummary
	for _, botID := range TradingBotIDs() {
		res := ReconcileBot(ctx, cfg, botID, client)
		summary.Results = append(summary.Results, res)
		if res.Err == nil {
			summary.TotalReconciled += res.Reconciled
		}
	}
	return summary
}

// ReconcileBot queries OANDA transactions and updates incomplete rows in trades.db.
func ReconcileBot(ctx context.Context, cfg *config.Config, botID string, client *oanda.Client) ReconcileResult {
	botID = config.NormalizeBotID(botID)
	res := ReconcileResult{
		BotID:  botID,
		DBPath: BotDBPath(cfg, botID),
	}
	if client == nil {
		res.Err = context.Canceled
		return res
	}

	store, err := sqlite.Open(res.DBPath)
	if err != nil {
		res.Err = err
		return res
	}
	defer store.Close()

	rows, err := store.ListTradesNeedingReconciliation()
	if err != nil {
		res.Err = err
		return res
	}
	res.Attempted = len(rows)
	if len(rows) == 0 {
		return res
	}

	txs, txErr := fetchReconcileTransactions(ctx, client)
	if txErr != nil {
		slog.Warn("reconcile: bulk transaction fetch failed", "bot", botID, "error", txErr)
	}

	for _, row := range rows {
		pl, inst, ok := lookupTradePLInTransactions(txs, row.TradeID)
		if !ok && txErr == nil {
			pl, inst, ok = lookupTradePL(ctx, client, row.TradeID, row.ClosedAt)
		}
		if !ok {
			res.StillMissing++
			continue
		}
		if strings.TrimSpace(inst) == "" {
			inst = row.Instrument
		}
		netPL := pl - row.SwapCost
		if err := store.UpdateTradeReconciliation(row.TradeID, inst, pl, netPL); err != nil {
			slog.Warn("reconcile trade update failed",
				"bot", botID, "trade_id", row.TradeID, "error", err)
			res.StillMissing++
			continue
		}
		res.Reconciled++
		slog.Info("reconciled trade",
			"bot", botID, "trade_id", row.TradeID, "instrument", inst, "net_pl", netPL)
	}
	return res
}

func fetchReconcileTransactions(ctx context.Context, client *oanda.Client) ([]oanda.Transaction, error) {
	windows := []time.Duration{
		7 * 24 * time.Hour,
		30 * 24 * time.Hour,
	}
	var lastErr error
	for _, window := range windows {
		txs, err := client.TransactionsSinceAll(ctx, time.Now().Add(-window))
		if err != nil {
			lastErr = err
			slog.Warn("reconcile: transactions fetch failed", "window", window.String(), "error", err)
			continue
		}
		return txs, nil
	}
	return nil, lastErr
}

func lookupTradePLInTransactions(txs []oanda.Transaction, tradeID string) (realizedPL float64, instrument string, ok bool) {
	if len(txs) == 0 || tradeID == "" {
		return 0, "", false
	}
	pl, plFound := oanda.ClosedTradePL(txs, tradeID)
	if inst, found := oanda.ClosedTradeInstrument(txs, tradeID); found {
		instrument = inst
	}
	if plFound {
		return pl, instrument, true
	}
	return 0, instrument, false
}

var reconcileRetryDelays = []time.Duration{
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
}

var reconcileLookupWindows = []time.Duration{
	24 * time.Hour,
	7 * 24 * time.Hour,
}

func lookupTradePL(ctx context.Context, client *oanda.Client, tradeID string, closedAt time.Time) (realizedPL float64, instrument string, ok bool) {
	attempts := append([]time.Duration{0}, reconcileRetryDelays...)
	for _, delay := range attempts {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return 0, "", false
			case <-time.After(delay):
			}
		}
		for _, window := range reconcileLookupWindows {
			since := closedAt.Add(-window)
			if since.After(closedAt) {
				since = closedAt.Add(-24 * time.Hour)
			}
			txs, err := client.TransactionsSince(ctx, since)
			if err != nil {
				slog.Warn("reconcile: transactions lookup failed",
					"trade_id", tradeID, "window", window.String(), "error", err)
				continue
			}
			plFound := false
			if p, found := oanda.ClosedTradePL(txs.Transactions, tradeID); found {
				realizedPL, plFound = p, true
			}
			if inst, found := oanda.ClosedTradeInstrument(txs.Transactions, tradeID); found {
				instrument = inst
			}
			if plFound {
				return realizedPL, instrument, true
			}
		}
	}
	return 0, "", false
}

// IsTradeReconciled reports whether a closed trade has usable P/L (not a failed lookup).
func IsTradeReconciled(netPL, realizedPL float64) bool {
	return math.Abs(netPL) > 0.01 || math.Abs(realizedPL) > 0.01
}
