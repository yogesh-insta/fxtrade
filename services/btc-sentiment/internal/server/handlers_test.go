package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/agent"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/cache"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/fetchers"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/gemini"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/server"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/store"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/tools"
)

type oneShotModel struct{}

func (oneShotModel) Generate(ctx context.Context, req gemini.GenerateRequest) (gemini.GenerateResponse, error) {
	return gemini.GenerateResponse{Candidates: []gemini.Candidate{{Content: gemini.Content{
		Role: "model",
		Parts: []gemini.Part{{FunctionCall: &gemini.FunctionCall{
			Name: "emit_sentiment",
			Args: map[string]any{
				"sentiment_score": 0.1,
				"confidence":      0.5,
				"low_confidence":  true,
				"divergence_note": "no data available for window.",
				"fallback_used":   true,
			},
		}}},
	}}}}, nil
}

type emptyNews struct{}

func (emptyNews) FetchNews(ctx context.Context, start, end time.Time) ([]fetchers.NewsItem, error) {
	return nil, fetchers.NewNoDataError("FetchNews")
}

type emptyReddit struct{}

func (emptyReddit) FetchRedditPosts(ctx context.Context, subreddits []string, start, end time.Time) ([]fetchers.RedditPost, error) {
	return nil, fetchers.NewNoDataError("FetchRedditPosts")
}

func TestHandlerAgent(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	st, err := store.FromDB(c.DB())
	if err != nil {
		t.Fatal(err)
	}
	reg := tools.BuildRegistry(tools.Deps{
		News: emptyNews{}, Reddit: emptyReddit{}, Cache: c, Store: st, CacheTTL: time.Hour,
	})
	ag := &agent.Agent{Model: oneShotModel{}, Tools: reg, System: "test", Window: 4 * time.Hour, MaxTurns: 3}
	h := &server.Handler{Agent: ag}
	mux := http.NewServeMux()
	h.Register(mux)

	hz := httptest.NewRecorder()
	mux.ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if hz.Code != 200 {
		t.Fatalf("healthz=%d", hz.Code)
	}

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/run-sentiment-pass", bytes.NewBufferString(`{}`)))
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var res agent.Result
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.FallbackUsed || res.SentimentScore != 0.1 {
		t.Fatalf("unexpected %+v", res)
	}
}
