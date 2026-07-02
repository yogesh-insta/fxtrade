package state

import (
	"time"

	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
)

func BuildSnapshot(rm *risk.Manager, cache *sentiment.Cache, lastTrade *LastTrade) Snapshot {
	snap := Snapshot{
		Risk:     rm.ExportState(),
		LastTrade: lastTrade,
	}
	if cache != nil {
		snap.SentimentHistory = cache.History()
	}
	return snap
}

func ApplySnapshot(snap Snapshot, rm *risk.Manager, cache *sentiment.Cache) {
	if snap.Risk.MonthKey != "" || snap.Risk.TradesThisMonth > 0 {
		rm.RestoreState(snap.Risk)
	}
	if cache != nil && len(snap.SentimentHistory) > 0 {
		cache.Restore(snap.SentimentHistory)
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

func (t *TradeRecorder) OnClose(tradeID, correlationID string, pl float64, rm *risk.Manager, cache *sentiment.Cache) {
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
		_ = t.store.Save(BuildSnapshot(rm, cache, t.lastTrade))
	}
}

func (t *TradeRecorder) LastTrade() *LastTrade {
	return t.lastTrade
}
