package cache_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/cache"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/sentiment"
)

func TestCacheHitMiss(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.Open(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	note := "test"
	want := sentiment.SentimentResult{
		SentimentScore: 0.5,
		Confidence:     0.8,
		LowConfidence:  false,
		KeyDrivers:     []string{"etf inflows"},
		DivergenceNote: &note,
	}

	got, ok, err := c.Get("missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != nil {
		t.Fatalf("expected miss, got ok=%v result=%v", ok, got)
	}

	if err := c.Set("k1", want, time.Hour); err != nil {
		t.Fatal(err)
	}
	got, ok, err = c.Get("k1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got == nil {
		t.Fatal("expected hit")
	}
	if got.SentimentScore != want.SentimentScore || got.Confidence != want.Confidence {
		t.Fatalf("mismatch: %+v", got)
	}

	if err := c.Set("k2", want, -time.Second); err != nil {
		t.Fatal(err)
	}
	got, ok, err = c.Get("k2")
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != nil {
		t.Fatalf("expected expired miss, got ok=%v", ok)
	}
}
