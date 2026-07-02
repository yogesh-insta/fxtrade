package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
)

type LastTrade struct {
	TradeID       string    `json:"trade_id"`
	CorrelationID string    `json:"correlation_id"`
	RealizedPL    float64   `json:"realized_pl"`
	ClosedAt      time.Time `json:"closed_at"`
}

type Snapshot struct {
	Risk              risk.State              `json:"risk"`
	LastTrade         *LastTrade              `json:"last_trade,omitempty"`
	SentimentHistory  []sentiment.SentimentSignal `json:"sentiment_history,omitempty"`
	SavedAt           time.Time               `json:"saved_at"`
}

type Store struct {
	path string
}

func NewStore(path string) *Store {
	if path == "" {
		path = "data/state.json"
	}
	return &Store{path: path}
}

func (s *Store) Load() (Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, nil
		}
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("parse state %s: %w", s.path, err)
	}
	return snap, nil
}

func (s *Store) Save(snap Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	snap.SavedAt = time.Now().UTC()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
