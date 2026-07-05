package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func TestInsertSignal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trades.db")
	store, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.InsertSignal(sqlite.Signal{
		At:         time.Now().UTC(),
		BotID:      config.BotFxSentiment,
		Instrument: "EUR_USD",
		Mode:       "RANGE",
		Action:     "no_trade",
		Reason:     "spread_gate",
		Details:    map[string]any{"spread_pips": 2.1},
	}); err != nil {
		t.Fatal(err)
	}
}
