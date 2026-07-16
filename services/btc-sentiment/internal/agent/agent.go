package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/gemini"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/sentiment"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/tools"
)

const defaultMaxTurns = 10

// Model is the Gemini generateContent interface (mockable).
type Model interface {
	Generate(ctx context.Context, req gemini.GenerateRequest) (gemini.GenerateResponse, error)
}

// Agent runs a Gemini function-calling loop over registered tools + skills.
type Agent struct {
	Model    Model
	Tools    *tools.Registry
	System   string
	MaxTurns int
	MinItems int
	Window   time.Duration
	Log      *slog.Logger
}

// Result is the HTTP-facing outcome of one agent run.
type Result struct {
	sentiment.SentimentResult
	CacheUsed    bool `json:"cache_used"`
	FallbackUsed bool `json:"fallback_used"`
	NewsCount    int  `json:"news_count"`
	RedditCount  int  `json:"reddit_count"`
	Turns        int  `json:"turns"`
}

// Run executes one sentiment pass ending at now.
func (a *Agent) Run(ctx context.Context, now time.Time) (Result, error) {
	log := a.Log
	if log == nil {
		log = slog.Default()
	}
	if a.MaxTurns <= 0 {
		a.MaxTurns = defaultMaxTurns
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	end := now.UTC()
	start := end.Add(-a.Window)

	system := a.System
	if system == "" {
		var err error
		system, err = LoadSystemPrompt()
		if err != nil {
			return Result{}, err
		}
	}

	userMsg := fmt.Sprintf(
		`Run a BTC/USD sentiment pass.
window_start=%s
window_end=%s
min_items_threshold=%d

Use tools to fetch data, check cache, score if needed, log, and finish with emit_sentiment.`,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		a.MinItems,
	)

	contents := []gemini.Content{{
		Role:  "user",
		Parts: []gemini.Part{{Text: userMsg}},
	}}

	for turn := 1; turn <= a.MaxTurns; turn++ {
		resp, err := a.Model.Generate(ctx, gemini.GenerateRequest{
			SystemInstruction: &gemini.Content{Parts: []gemini.Part{{Text: system}}},
			Contents:          contents,
			Tools:             a.Tools.GeminiTools(),
			ToolConfig: &gemini.ToolConfig{
				FunctionCallingConfig: &gemini.FunctionCallingConfig{Mode: "AUTO"},
			},
			GenerationConfig: &gemini.GenerationConfig{Temperature: 0, MaxOutputTokens: 2048},
		})
		if err != nil {
			log.Error("gemini generate failed", "err", err, "turn", turn, "fallback_used", true)
			return fallbackResult(turn), nil
		}

		modelContent, ok := resp.ModelContent()
		if !ok {
			log.Error("gemini returned no candidates", "turn", turn, "fallback_used", true)
			return fallbackResult(turn), nil
		}
		contents = append(contents, modelContent)

		calls := resp.FunctionCalls()
		if len(calls) == 0 {
			// Model replied with prose; nudge it to emit.
			contents = append(contents, gemini.Content{
				Role:  "user",
				Parts: []gemini.Part{{Text: "You must call the emit_sentiment tool to finish. Do not reply with prose only."}},
			})
			continue
		}

		var responseParts []gemini.Part
		for _, part := range calls {
			fc := part.FunctionCall
			if fc == nil {
				continue
			}
			log.Info("tool call", "turn", turn, "tool", fc.Name)

			out, terminal, callErr := a.Tools.Call(ctx, fc.Name, fc.Args)
			respBody := map[string]any{}
			if callErr != nil {
				respBody["error"] = callErr.Error()
				log.Warn("tool error", "tool", fc.Name, "err", callErr)
			} else {
				respBody["result"] = out
			}

			fr := &gemini.FunctionResponse{
				ID:       fc.ID,
				Name:     fc.Name,
				Response: respBody,
			}
			responseParts = append(responseParts, gemini.Part{FunctionResponse: fr})

			if terminal && callErr == nil {
				if emit, ok := out.(tools.EmitOutcome); ok {
					log.Info("agent finished", "turn", turn, "cache_used", emit.CacheUsed, "fallback_used", emit.FallbackUsed)
					return Result{
						SentimentResult: emit.Result,
						CacheUsed:       emit.CacheUsed,
						FallbackUsed:    emit.FallbackUsed,
						NewsCount:       emit.NewsCount,
						RedditCount:     emit.RedditCount,
						Turns:           turn,
					}, nil
				}
			}
		}

		contents = append(contents, gemini.Content{
			Role:  "user",
			Parts: responseParts,
		})
	}

	log.Error("agent exceeded max turns", "max_turns", a.MaxTurns, "fallback_used", true)
	return fallbackResult(a.MaxTurns), nil
}

func fallbackResult(turns int) Result {
	note := "scoring failed, using neutral default"
	return Result{
		SentimentResult: sentiment.NeutralFallback(note),
		FallbackUsed:    true,
		Turns:           turns,
	}
}
