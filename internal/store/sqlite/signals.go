package sqlite

import (
	"encoding/json"
	"fmt"
	"time"
)

const signalsSchema = `
CREATE TABLE IF NOT EXISTS signals (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at TEXT NOT NULL,
	bot_id TEXT NOT NULL,
	instrument TEXT,
	mode TEXT,
	action TEXT NOT NULL,
	reason TEXT,
	direction TEXT,
	setup_score REAL,
	correlation_id TEXT,
	details TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_signals_at ON signals(at);
CREATE INDEX IF NOT EXISTS idx_signals_bot_id ON signals(bot_id);
`

// Signal records a strategy/scanner decision for later fine-tuning.
type Signal struct {
	At            time.Time
	BotID         string
	Instrument    string
	Mode          string
	Action        string
	Reason        string
	Direction     string
	SetupScore    float64
	CorrelationID string
	Details       map[string]any
}

func (s *Store) InsertSignal(sig Signal) error {
	if sig.At.IsZero() {
		sig.At = time.Now().UTC()
	}
	if sig.BotID == "" {
		return fmt.Errorf("insert signal: bot_id required")
	}
	if sig.Action == "" {
		return fmt.Errorf("insert signal: action required")
	}
	var details any
	if len(sig.Details) > 0 {
		b, err := json.Marshal(sig.Details)
		if err != nil {
			return fmt.Errorf("marshal signal details: %w", err)
		}
		details = string(b)
	}
	_, err := s.db.Exec(`
INSERT INTO signals (
	at, bot_id, instrument, mode, action, reason, direction,
	setup_score, correlation_id, details
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.At.UTC().Format(time.RFC3339),
		sig.BotID,
		nullString(sig.Instrument),
		nullString(sig.Mode),
		sig.Action,
		nullString(sig.Reason),
		nullString(sig.Direction),
		nullFloat(sig.SetupScore),
		nullString(sig.CorrelationID),
		details,
	)
	if err != nil {
		return fmt.Errorf("insert signal: %w", err)
	}
	return nil
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
