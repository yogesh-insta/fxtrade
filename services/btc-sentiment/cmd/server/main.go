package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/agent"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/cache"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/config"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/fetchers"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/gemini"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/server"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/store"
	"github.com/ym/fxtrade/services/btc-sentiment/internal/tools"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		log.Error("config load failed", "err", err)
		os.Exit(1)
	}

	systemPrompt, err := agent.LoadSystemPrompt()
	if err != nil {
		log.Error("load skills failed", "err", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	c, err := cache.Open(cfg.DBPath)
	if err != nil {
		log.Error("cache open failed", "err", err)
		os.Exit(1)
	}
	defer c.Close()

	st, err := store.FromDB(c.DB())
	if err != nil {
		log.Error("store open failed", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	registry := tools.BuildRegistry(tools.Deps{
		News:       fetchers.NewNewsClient(cfg.CryptoPanicAPIKey, httpClient),
		Reddit:     fetchers.NewRedditClient(cfg.RedditClientID, cfg.RedditClientSecret, cfg.RedditUserAgent, httpClient),
		Subreddits: cfg.RedditSubreddits,
		Cache:      c,
		Store:      st,
		CacheTTL:   cfg.CacheTTL,
	})

	model := gemini.NewClient(cfg.GeminiAPIKey, cfg.GeminiModel, httpClient)
	ag := &agent.Agent{
		Model:    model,
		Tools:    registry,
		System:   systemPrompt,
		MaxTurns: 10,
		MinItems: cfg.MinItemsThreshold,
		Window:   cfg.Window,
		Log:      log,
	}

	mux := http.NewServeMux()
	h := &server.Handler{Agent: ag, Log: log}
	h.Register(mux)

	addr := ":" + cfg.Port
	log.Info("listening",
		"addr", addr,
		"model", cfg.GeminiModel,
		"credentials", cfg.CredentialsPath,
		"reddit_configured", cfg.RedditConfigured,
		"reddit_subreddits", cfg.RedditSubreddits,
		"skills_bytes", len(systemPrompt),
		"tools", len(registry.Declarations()),
	)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
