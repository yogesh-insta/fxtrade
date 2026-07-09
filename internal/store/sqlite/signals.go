package sqlite

import (
	"database/sql"
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

// SignalScoresByCorrelation maps entry correlation IDs to setup scores from signals.
func (s *Store) SignalScoresByCorrelation() (map[string]float64, error) {
	rows, err := s.db.Query(`
SELECT correlation_id, setup_score FROM signals
WHERE action = 'entry_taken' AND correlation_id IS NOT NULL AND correlation_id != ''`)
	if err != nil {
		return nil, fmt.Errorf("query signal scores: %w", err)
	}
	defer rows.Close()

	out := make(map[string]float64)
	for rows.Next() {
		var corr sql.NullString
		var score sql.NullFloat64
		if err := rows.Scan(&corr, &score); err != nil {
			return nil, err
		}
		if corr.Valid && score.Valid {
			out[corr.String] = score.Float64
		}
	}
	return out, rows.Err()
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
