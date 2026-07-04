package afl

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ym/fxtrade/internal/afl/gemini"
	"github.com/ym/fxtrade/internal/config"
)

// EnrichReportsWithLLMAnalytics calls Gemini + Google Search for each fixture when configured.
// Existing model fields on reports are unchanged; LLMAnalytics is populated on success.
func EnrichReportsWithLLMAnalytics(ctx context.Context, cfg config.AFLConfig, round int, reports []MatchReport) {
	if !cfg.AnalyticsEnabled() {
		return
	}

	client := gemini.NewClient(cfg.GeminiAPIKey, cfg.ResolvedGeminiModel())
	concurrency := cfg.LLMConcurrency
	if concurrency < 1 {
		concurrency = 1
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := range reports {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			report := &reports[idx]
			userPrompt := BuildGeminiAnalyticsUserPrompt(round, *report)
			text, err := client.GenerateGrounded(ctx, GeminiAnalyticsSystemPrompt, userPrompt)
			if err != nil {
				slog.Warn("gemini analytics failed",
					"match", fmt.Sprintf("%s vs %s", report.Context.HomeTeam, report.Context.AwayTeam),
					"model", cfg.ResolvedGeminiModel(),
					"error", err,
				)
				return
			}
			report.LLMAnalytics = text
			slog.Info("gemini analytics complete",
				"match", fmt.Sprintf("%s vs %s", report.Context.HomeTeam, report.Context.AwayTeam),
				"chars", len(text),
			)
		}(i)
	}
	wg.Wait()
}
