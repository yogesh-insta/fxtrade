package agent_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/agent"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/cache"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/fetchers"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/gemini"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/store"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/tools"
)

type scriptedModel struct {
	steps []gemini.GenerateResponse
	i     int
}

func (m *scriptedModel) Generate(ctx context.Context, req gemini.GenerateRequest) (gemini.GenerateResponse, error) {
	if m.i >= len(m.steps) {
		return gemini.GenerateResponse{}, context.Canceled
	}
	step := m.steps[m.i]
	m.i++
	return step, nil
}

type newsStub struct {
	items []fetchers.NewsItem
}

func (n newsStub) FetchNews(ctx context.Context, start, end time.Time) ([]fetchers.NewsItem, error) {
	return n.items, nil
}

type redditStub struct{}

func (redditStub) FetchRedditPosts(ctx context.Context, subreddits []string, start, end time.Time) ([]fetchers.RedditPost, error) {
	return nil, fetchers.NewNoDataError("FetchRedditPosts")
}

func TestLoadSystemPromptIncludesSkills(t *testing.T) {
	p, err := agent.LoadSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"Skill: core", "emit_sentiment", "fetch_news", "Sentiment scoring"} {
		if !contains(p, needle) {
			t.Fatalf("system prompt missing %q", needle)
		}
	}
}

func TestAgentToolLoopEmit(t *testing.T) {
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
		News: newsStub{items: []fetchers.NewsItem{
			{Headline: "BTC climbs on ETF inflows", Timestamp: time.Now()},
			{Headline: "Bitcoin demand rises", Timestamp: time.Now()},
			{Headline: "Whales accumulate", Timestamp: time.Now()},
		}},
		Reddit:     redditStub{},
		Subreddits: []string{"Bitcoin"},
		Cache:      c,
		Store:      st,
		CacheTTL:   time.Hour,
	})

	model := &scriptedModel{steps: []gemini.GenerateResponse{
		{Candidates: []gemini.Candidate{{Content: gemini.Content{Role: "model", Parts: []gemini.Part{
			{FunctionCall: &gemini.FunctionCall{Name: "fetch_news", Args: map[string]any{
				"start": "2026-07-14T08:00:00Z", "end": "2026-07-14T12:00:00Z",
			}}},
			{FunctionCall: &gemini.FunctionCall{Name: "fetch_reddit", Args: map[string]any{
				"start": "2026-07-14T08:00:00Z", "end": "2026-07-14T12:00:00Z",
			}}},
		}}}}},
		{Candidates: []gemini.Candidate{{Content: gemini.Content{Role: "model", Parts: []gemini.Part{
			{FunctionCall: &gemini.FunctionCall{Name: "emit_sentiment", Args: map[string]any{
				"sentiment_score": 0.4,
				"confidence":      0.7,
				"low_confidence":  false,
				"key_drivers":     []any{"etf", "demand"},
				"news_count":      3.0,
				"reddit_count":    0.0,
				"cache_used":      false,
				"fallback_used":   false,
			}}},
		}}}}},
	}}

	ag := &agent.Agent{
		Model:    model,
		Tools:    reg,
		System:   "test",
		MaxTurns: 5,
		MinItems: 3,
		Window:   4 * time.Hour,
	}

	res, err := ag.Run(context.Background(), time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if res.SentimentScore != 0.4 || res.NewsCount != 3 || res.Turns != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestAgentFallbackOnMaxTurns(t *testing.T) {
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
		News:     newsStub{},
		Reddit:   redditStub{},
		Cache:    c,
		Store:    st,
		CacheTTL: time.Hour,
	})
	model := &scriptedModel{steps: []gemini.GenerateResponse{
		{Candidates: []gemini.Candidate{{Content: gemini.Content{Role: "model", Parts: []gemini.Part{
			{Text: "I refuse to use tools"},
		}}}}},
	}}
	ag := &agent.Agent{Model: model, Tools: reg, System: "test", MaxTurns: 1, Window: time.Hour}
	res, err := ag.Run(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !res.FallbackUsed || res.SentimentScore != 0 {
		t.Fatalf("expected fallback, got %+v", res)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
