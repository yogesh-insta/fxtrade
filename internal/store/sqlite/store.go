package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS trades (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	closed_at TEXT NOT NULL,
	instrument TEXT NOT NULL,
	direction TEXT,
	trade_id TEXT NOT NULL,
	correlation_id TEXT,
	signal_price REAL,
	fill_price REAL,
	stop_loss REAL,
	take_profit REAL,
	units INTEGER,
	realized_pl REAL NOT NULL,
	swap_cost REAL DEFAULT 0,
	net_pl REAL,
	slippage_pct REAL,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_trades_closed_at ON trades(closed_at);
CREATE INDEX IF NOT EXISTS idx_trades_trade_id ON trades(trade_id);
`

// Trade is a closed-trade row for P&L tracking (see docs/btc_cfd_bot_spec.md §11).
type Trade struct {
	ClosedAt      time.Time
	Instrument    string
	Direction     string
	TradeID       string
	CorrelationID string
	SignalPrice   float64
	FillPrice     float64
	StopLoss      float64
	TakeProfit    float64
	Units         int64
	RealizedPL    float64
	SwapCost      float64
	NetPL         float64
	SlippagePct   float64
}

// Store persists closed trades in SQLite (primary on-VM store for btc_cfd).
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "data/btc_cfd/trades.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) InsertTrade(t Trade) error {
	if t.ClosedAt.IsZero() {
		t.ClosedAt = time.Now().UTC()
	}
	if t.NetPL == 0 && t.RealizedPL != 0 {
		t.NetPL = t.RealizedPL - t.SwapCost
	}
	_, err := s.db.Exec(`
INSERT INTO trades (
	closed_at, instrument, direction, trade_id, correlation_id,
	signal_price, fill_price, stop_loss, take_profit, units,
	realized_pl, swap_cost, net_pl, slippage_pct
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ClosedAt.UTC().Format(time.RFC3339),
		t.Instrument,
		t.Direction,
		t.TradeID,
		t.CorrelationID,
		nullFloat(t.SignalPrice),
		nullFloat(t.FillPrice),
		nullFloat(t.StopLoss),
		nullFloat(t.TakeProfit),
		nullInt(t.Units),
		t.RealizedPL,
		t.SwapCost,
		t.NetPL,
		nullFloat(t.SlippagePct),
	)
	if err != nil {
		return fmt.Errorf("insert trade: %w", err)
	}
	return nil
}

func nullFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
