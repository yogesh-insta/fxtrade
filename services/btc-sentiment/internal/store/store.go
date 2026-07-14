package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/sentiment"
	_ "modernc.org/sqlite"
)

// RunRecord is one scoring run for audit/backtesting.
type RunRecord struct {
	Timestamp      time.Time
	WindowStart    time.Time
	WindowEnd      time.Time
	NewsCount      int
	RedditCount    int
	LLMRequest     string
	LLMResponse    string
	ParsedResult   sentiment.SentimentResult
	CacheUsed      bool
	FallbackUsed   bool
}

// Store persists run records (SQLite now; BigQuery later).
type Store interface {
	LogRun(ctx context.Context, run RunRecord) error
	Close() error
}

// SQLiteStore logs runs to SQLite.
type SQLiteStore struct {
	db     *sql.DB
	owned  bool
}

// Open opens a new SQLite store at path.
func Open(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open store db: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db, owned: true}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// FromDB uses an existing DB connection (e.g. shared with cache).
func FromDB(db *sql.DB) (*SQLiteStore, error) {
	s := &SQLiteStore{db: db, owned: false}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS sentiment_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  window_start INTEGER NOT NULL,
  window_end INTEGER NOT NULL,
  news_count INTEGER NOT NULL,
  reddit_count INTEGER NOT NULL,
  llm_request TEXT,
  llm_response TEXT,
  parsed_result TEXT NOT NULL,
  cache_used INTEGER NOT NULL,
  fallback_used INTEGER NOT NULL
);
`)
	return err
}

// LogRun inserts a run audit row.
func (s *SQLiteStore) LogRun(ctx context.Context, run RunRecord) error {
	parsed, err := json.Marshal(run.ParsedResult)
	if err != nil {
		return err
	}
	cacheUsed, fallbackUsed := 0, 0
	if run.CacheUsed {
		cacheUsed = 1
	}
	if run.FallbackUsed {
		fallbackUsed = 1
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO sentiment_runs (
  ts, window_start, window_end, news_count, reddit_count,
  llm_request, llm_response, parsed_result, cache_used, fallback_used
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.Timestamp.Unix(),
		run.WindowStart.Unix(),
		run.WindowEnd.Unix(),
		run.NewsCount,
		run.RedditCount,
		run.LLMRequest,
		run.LLMResponse,
		string(parsed),
		cacheUsed,
		fallbackUsed,
	)
	return err
}

// Close closes the DB if this store owns it.
func (s *SQLiteStore) Close() error {
	if s.owned {
		return s.db.Close()
	}
	return nil
}
