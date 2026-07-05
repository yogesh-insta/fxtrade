package btc_cfd

import (
	"sync"
	"time"
)

// tradeMeta holds entry context for enriched SQLite rows on close.
type tradeMeta struct {
	CorrelationID string
	Direction     string
	SignalPrice   float64
	FillPrice     float64
	StopLoss      float64
	TakeProfit    float64
	Units         int64
	OpenedAt      time.Time
}

type tradeMetaStore struct {
	mu   sync.RWMutex
	byID map[string]tradeMeta
}

func newTradeMetaStore() *tradeMetaStore {
	return &tradeMetaStore{byID: make(map[string]tradeMeta)}
}

func (s *tradeMetaStore) Put(tradeID string, m tradeMeta) {
	if tradeID == "" {
		return
	}
	s.mu.Lock()
	s.byID[tradeID] = m
	s.mu.Unlock()
}

func (s *tradeMetaStore) Take(tradeID string) (tradeMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[tradeID]
	if ok {
		delete(s.byID, tradeID)
	}
	return m, ok
}

func slippagePct(signal, fill float64) float64 {
	if signal <= 0 || fill <= 0 {
		return 0
	}
	d := (fill - signal) / signal
	if d < 0 {
		d = -d
	}
	return d * 100
}
