package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/cache"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/fetchers"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/gemini"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/sentiment"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/store"
)

// Deps are concrete backends tools call into.
type Deps struct {
	News       fetchers.NewsFetcher
	Reddit     fetchers.RedditFetcher
	Subreddits []string
	Cache      cache.Cache
	Store      store.Store
	CacheTTL   time.Duration
}

// EmitOutcome is returned when emit_sentiment succeeds (terminal).
type EmitOutcome struct {
	Result       sentiment.SentimentResult `json:"result"`
	CacheUsed    bool                      `json:"cache_used"`
	FallbackUsed bool                      `json:"fallback_used"`
	NewsCount    int                       `json:"news_count"`
	RedditCount  int                       `json:"reddit_count"`
}

// BuildRegistry wires all agent tools.
func BuildRegistry(d Deps) *Registry {
	return NewRegistry(
		fetchNewsTool(d),
		fetchRedditTool(d),
		computeWindowKeyTool(),
		cacheGetTool(d),
		cacheSetTool(d),
		logRunTool(d),
		emitSentimentTool(),
	)
}

func fetchNewsTool(d Deps) Tool {
	return Tool{
		Decl: gemini.FunctionDeclaration{
			Name:        "fetch_news",
			Description: "Fetch BTC-related news headlines for [start, end] (RFC3339 UTC).",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"start": {Type: gemini.TypeString, Description: "Window start RFC3339 UTC"},
					"end":   {Type: gemini.TypeString, Description: "Window end RFC3339 UTC"},
				},
				Required: []string{"start", "end"},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			start, end, err := parseWindow(args)
			if err != nil {
				return nil, err
			}
			items, err := d.News.FetchNews(ctx, start, end)
			if err != nil {
				return map[string]any{"ok": false, "error": err.Error(), "items": []any{}, "count": 0}, nil
			}
			return map[string]any{"ok": true, "count": len(items), "items": asJSON(items)}, nil
		},
	}
}

func fetchRedditTool(d Deps) Tool {
	return Tool{
		Decl: gemini.FunctionDeclaration{
			Name:        "fetch_reddit",
			Description: "Fetch Reddit posts for configured subreddits in [start, end] (RFC3339 UTC).",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"start": {Type: gemini.TypeString, Description: "Window start RFC3339 UTC"},
					"end":   {Type: gemini.TypeString, Description: "Window end RFC3339 UTC"},
				},
				Required: []string{"start", "end"},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			start, end, err := parseWindow(args)
			if err != nil {
				return nil, err
			}
			items, err := d.Reddit.FetchRedditPosts(ctx, d.Subreddits, start, end)
			if err != nil {
				return map[string]any{"ok": false, "error": err.Error(), "items": []any{}, "count": 0}, nil
			}
			return map[string]any{"ok": true, "count": len(items), "items": asJSON(items)}, nil
		},
	}
}

func computeWindowKeyTool() Tool {
	return Tool{
		Decl: gemini.FunctionDeclaration{
			Name:        "compute_window_key",
			Description: "SHA256 hex of concatenated headlines and post texts (stable cache key).",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"headlines": {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
					"posts":     {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
				},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			headlines, err := argStringSlice(args, "headlines")
			if err != nil {
				return nil, err
			}
			posts, err := argStringSlice(args, "posts")
			if err != nil {
				return nil, err
			}
			var b strings.Builder
			for _, h := range headlines {
				b.WriteString(h)
				b.WriteByte('\n')
			}
			for _, p := range posts {
				b.WriteString(p)
				b.WriteByte('\n')
			}
			sum := sha256.Sum256([]byte(b.String()))
			return map[string]any{"window_key": hex.EncodeToString(sum[:])}, nil
		},
	}
}

func cacheGetTool(d Deps) Tool {
	return Tool{
		Decl: gemini.FunctionDeclaration{
			Name:        "cache_get",
			Description: "Get a cached SentimentResult by window_key, if unexpired.",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"window_key": {Type: gemini.TypeString},
				},
				Required: []string{"window_key"},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			key, err := argString(args, "window_key")
			if err != nil {
				return nil, err
			}
			res, ok, err := d.Cache.Get(key)
			if err != nil {
				return map[string]any{"hit": false, "error": err.Error()}, nil
			}
			if !ok || res == nil {
				return map[string]any{"hit": false}, nil
			}
			return map[string]any{"hit": true, "result": asJSON(*res)}, nil
		},
	}
}

func cacheSetTool(d Deps) Tool {
	resultSchema := &gemini.Schema{
		Type: gemini.TypeObject,
		Properties: map[string]*gemini.Schema{
			"sentiment_score": {Type: gemini.TypeNumber},
			"confidence":      {Type: gemini.TypeNumber},
			"low_confidence":  {Type: gemini.TypeBoolean},
			"key_drivers":     {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
			"divergence_note": {Type: gemini.TypeString},
		},
		Required: []string{"sentiment_score", "confidence", "low_confidence"},
	}
	return Tool{
		Decl: gemini.FunctionDeclaration{
			Name:        "cache_set",
			Description: "Store a SentimentResult under window_key with configured TTL.",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"window_key": {Type: gemini.TypeString},
					"result":     resultSchema,
				},
				Required: []string{"window_key", "result"},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			key, err := argString(args, "window_key")
			if err != nil {
				return nil, err
			}
			result, err := parseResult(args["result"])
			if err != nil {
				return nil, err
			}
			if err := d.Cache.Set(key, result, d.CacheTTL); err != nil {
				return map[string]any{"ok": false, "error": err.Error()}, nil
			}
			return map[string]any{"ok": true}, nil
		},
	}
}

func logRunTool(d Deps) Tool {
	return Tool{
		Decl: gemini.FunctionDeclaration{
			Name:        "log_run",
			Description: "Persist an audit row for this sentiment pass (for later backtesting).",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"window_start":  {Type: gemini.TypeString},
					"window_end":    {Type: gemini.TypeString},
					"news_count":    {Type: gemini.TypeInteger},
					"reddit_count":  {Type: gemini.TypeInteger},
					"cache_used":    {Type: gemini.TypeBoolean},
					"fallback_used": {Type: gemini.TypeBoolean},
					"result": {
						Type: gemini.TypeObject,
						Properties: map[string]*gemini.Schema{
							"sentiment_score": {Type: gemini.TypeNumber},
							"confidence":      {Type: gemini.TypeNumber},
							"low_confidence":  {Type: gemini.TypeBoolean},
							"key_drivers":     {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
							"divergence_note": {Type: gemini.TypeString},
						},
					},
				},
				Required: []string{"window_start", "window_end", "result"},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			ws, err := argString(args, "window_start")
			if err != nil {
				return nil, err
			}
			we, err := argString(args, "window_end")
			if err != nil {
				return nil, err
			}
			start, err := time.Parse(time.RFC3339, ws)
			if err != nil {
				return nil, fmt.Errorf("window_start: %w", err)
			}
			end, err := time.Parse(time.RFC3339, we)
			if err != nil {
				return nil, fmt.Errorf("window_end: %w", err)
			}
			result, err := parseResult(args["result"])
			if err != nil {
				return nil, err
			}
			rec := store.RunRecord{
				Timestamp:    time.Now().UTC(),
				WindowStart:  start.UTC(),
				WindowEnd:    end.UTC(),
				NewsCount:    intArg(args, "news_count"),
				RedditCount:  intArg(args, "reddit_count"),
				ParsedResult: result,
				CacheUsed:    boolArg(args, "cache_used"),
				FallbackUsed: boolArg(args, "fallback_used"),
			}
			if err := d.Store.LogRun(ctx, rec); err != nil {
				return map[string]any{"ok": false, "error": err.Error()}, nil
			}
			return map[string]any{"ok": true}, nil
		},
	}
}

func emitSentimentTool() Tool {
	return Tool{
		Terminal: true,
		Decl: gemini.FunctionDeclaration{
			Name:        "emit_sentiment",
			Description: "End the run by emitting the final SentimentResult. Call exactly once.",
			Parameters: &gemini.Schema{
				Type: gemini.TypeObject,
				Properties: map[string]*gemini.Schema{
					"sentiment_score": {Type: gemini.TypeNumber, Description: "-1.0 to 1.0"},
					"confidence":      {Type: gemini.TypeNumber, Description: "0.0 to 1.0"},
					"low_confidence":  {Type: gemini.TypeBoolean},
					"key_drivers":     {Type: gemini.TypeArray, Items: &gemini.Schema{Type: gemini.TypeString}},
					"divergence_note": {Type: gemini.TypeString},
					"cache_used":      {Type: gemini.TypeBoolean},
					"fallback_used":   {Type: gemini.TypeBoolean},
					"news_count":      {Type: gemini.TypeInteger},
					"reddit_count":    {Type: gemini.TypeInteger},
				},
				Required: []string{"sentiment_score", "confidence", "low_confidence"},
			},
		},
		Handle: func(ctx context.Context, args map[string]any) (any, error) {
			result, err := parseResult(args)
			if err != nil {
				return nil, err
			}
			return EmitOutcome{
				Result:       result,
				CacheUsed:    boolArg(args, "cache_used"),
				FallbackUsed: boolArg(args, "fallback_used"),
				NewsCount:    intArg(args, "news_count"),
				RedditCount:  intArg(args, "reddit_count"),
			}, nil
		},
	}
}

func parseWindow(args map[string]any) (time.Time, time.Time, error) {
	ss, err := argString(args, "start")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	es, err := argString(args, "end")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start, err := time.Parse(time.RFC3339, ss)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start: %w", err)
	}
	end, err := time.Parse(time.RFC3339, es)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end: %w", err)
	}
	return start.UTC(), end.UTC(), nil
}

func parseResult(v any) (sentiment.SentimentResult, error) {
	if v == nil {
		return sentiment.SentimentResult{}, fmt.Errorf("missing result fields")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return sentiment.SentimentResult{}, err
	}
	return sentiment.ParseAndValidate(string(b))
}

func intArg(args map[string]any, key string) int {
	v, ok := args[key]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}

func boolArg(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
