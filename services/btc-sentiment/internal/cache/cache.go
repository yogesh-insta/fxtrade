package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/sentiment"
	_ "modernc.org/sqlite"
)

// Cache stores sentiment results keyed by content window hash.
type Cache interface {
	Get(windowKey string) (*sentiment.SentimentResult, bool, error)
	Set(windowKey string, result sentiment.SentimentResult, ttl time.Duration) error
	Close() error
}

// SQLiteCache is a SQLite-backed Cache.
type SQLiteCache struct {
	db *sql.DB
}

// Open opens or creates a SQLite cache at path.
func Open(path string) (*SQLiteCache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open cache db: %w", err)
	}
	db.SetMaxOpenConns(1)
	c := &SQLiteCache{db: db}
	if err := c.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return c, nil
}

func (c *SQLiteCache) migrate() error {
	_, err := c.db.Exec(`
CREATE TABLE IF NOT EXISTS sentiment_cache (
  window_key TEXT PRIMARY KEY,
  result_json TEXT NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sentiment_cache_expires ON sentiment_cache(expires_at);
`)
	return err
}

// Get returns a non-expired cached result.
func (c *SQLiteCache) Get(windowKey string) (*sentiment.SentimentResult, bool, error) {
	var raw string
	var expiresAt int64
	err := c.db.QueryRow(
		`SELECT result_json, expires_at FROM sentiment_cache WHERE window_key = ?`,
		windowKey,
	).Scan(&raw, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if time.Now().Unix() > expiresAt {
		_, _ = c.db.Exec(`DELETE FROM sentiment_cache WHERE window_key = ?`, windowKey)
		return nil, false, nil
	}
	var result sentiment.SentimentResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, false, fmt.Errorf("decode cached result: %w", err)
	}
	return &result, true, nil
}

// Set stores a result with the given TTL.
func (c *SQLiteCache) Set(windowKey string, result sentiment.SentimentResult, ttl time.Duration) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	expires := time.Now().Add(ttl).Unix()
	_, err = c.db.Exec(
		`INSERT INTO sentiment_cache (window_key, result_json, expires_at) VALUES (?, ?, ?)
		 ON CONFLICT(window_key) DO UPDATE SET result_json = excluded.result_json, expires_at = excluded.expires_at`,
		windowKey, string(raw), expires,
	)
	return err
}

// Close closes the database.
func (c *SQLiteCache) Close() error {
	return c.db.Close()
}

// DB exposes the underlying DB for sharing with the store when using one file.
func (c *SQLiteCache) DB() *sql.DB {
	return c.db
}
