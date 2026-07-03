package state

import (
	"time"

	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
)

func BuildSnapshot(rm *risk.Manager, caches map[string]*sentiment.Cache, lastTrade *LastTrade) Snapshot {
	snap := Snapshot{
		Risk:      rm.ExportState(),
		LastTrade: lastTrade,
	}
	if len(caches) > 0 {
		snap.SentimentHistories = make(map[string][]sentiment.SentimentSignal, len(caches))
		for inst, cache := range caches {
			if cache != nil {
				snap.SentimentHistories[inst] = cache.History()
			}
		}
	}
	return snap
}

func ApplySnapshot(snap Snapshot, rm *risk.Manager, caches map[string]*sentiment.Cache, primaryInstrument string) {
	if !snap.SavedAt.IsZero() {
		rm.RestoreState(snap.Risk)
	}
	for inst, history := range snap.SentimentHistories {
		if cache := caches[inst]; cache != nil && len(history) > 0 {
			cache.Restore(history)
		}
	}
	// Legacy single-instrument state files restore into the primary instrument.
	if len(snap.SentimentHistories) == 0 && len(snap.SentimentHistory) > 0 {
		if cache := caches[primaryInstrument]; cache != nil {
			cache.Restore(snap.SentimentHistory)
		}
	}
}

type TradeRecorder struct {
	store    *Store
	journal  *journal.Writer
	lastTrade *LastTrade
}

func NewTradeRecorder(store *Store, j *journal.Writer) *TradeRecorder {
	return &TradeRecorder{store: store, journal: j}
}

func (t *TradeRecorder) OnClose(tradeID, correlationID string, pl float64, rm *risk.Manager, caches map[string]*sentiment.Cache) {
	t.lastTrade = &LastTrade{
		TradeID:       tradeID,
		CorrelationID: correlationID,
		RealizedPL:    pl,
		ClosedAt:      time.Now().UTC(),
	}
	if t.journal != nil {
		_ = t.journal.AppendTrade(journal.TradeRecord{
			At:            t.lastTrade.ClosedAt,
			TradeID:       tradeID,
			CorrelationID: correlationID,
			RealizedPL:    pl,
		})
	}
	if t.store != nil {
		_ = t.store.Save(BuildSnapshot(rm, caches, t.lastTrade))
	}
}

func (t *TradeRecorder) LastTrade() *LastTrade {
	return t.lastTrade
}
