package store

import (
	"sync"
	"time"
)

// TradeMeta holds entry context for enriched SQLite rows on close.
type TradeMeta struct {
	Instrument    string
	CorrelationID string
	Direction     string
	SignalPrice   float64
	FillPrice     float64
	StopLoss      float64
	TakeProfit    float64
	Units         int64
	OpenedAt      time.Time
	SetupScore    float64
	Mode          string
}

type MetaStore struct {
	mu   sync.RWMutex
	byID map[string]TradeMeta
}

func NewMetaStore() *MetaStore {
	return &MetaStore{byID: make(map[string]TradeMeta)}
}

func (s *MetaStore) Put(tradeID string, m TradeMeta) {
	if tradeID == "" || s == nil {
		return
	}
	s.mu.Lock()
	s.byID[tradeID] = m
	s.mu.Unlock()
}

func (s *MetaStore) Take(tradeID string) (TradeMeta, bool) {
	if s == nil {
		return TradeMeta{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[tradeID]
	if ok {
		delete(s.byID, tradeID)
	}
	return m, ok
}

func SlippagePct(signal, fill float64) float64 {
	if signal <= 0 || fill <= 0 {
		return 0
	}
	d := (fill - signal) / signal
	if d < 0 {
		d = -d
	}
	return d * 100
}
