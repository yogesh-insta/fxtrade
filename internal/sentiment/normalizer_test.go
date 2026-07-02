package sentiment_test

import (
	"testing"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/sentiment"
)

func TestNormalizeDedupeAndCap(t *testing.T) {
	now := time.Now().UTC()
	raw := sentiment.RawData{
		Headlines: []sentiment.Headline{
			{Source: "Finnhub", Title: "AUD rises on RBA hold", Summary: "Australian dollar...", Published: now},
			{Source: "Reuters", Title: "AUD rises on RBA hold", Summary: "duplicate", Published: now.Add(-time.Hour)},
			{Source: "Fed", Title: "FOMC keeps rates steady", Summary: "US dollar...", Published: now},
			{Source: "Other", Title: "Local sports results", Summary: "unrelated", Published: now},
		},
	}
	cfg := config.DefaultSentimentConfig()
	cfg.MaxHeadlines = 2

	out := sentiment.Normalize(raw, cfg)
	if len(out.Headlines) != 2 {
		t.Fatalf("expected 2 headlines, got %d", len(out.Headlines))
	}
}

func TestSentimentSignalValidity(t *testing.T) {
	s := sentiment.SentimentSignal{
		AnalyzedAt:   time.Now().UTC(),
		ValidMinutes: 30,
	}
	if !s.IsValid(time.Now().UTC().Add(10 * time.Minute)) {
		t.Fatal("expected valid signal")
	}
	if s.IsValid(time.Now().UTC().Add(31 * time.Minute)) {
		t.Fatal("expected expired signal")
	}
}
