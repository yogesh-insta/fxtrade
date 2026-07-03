package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
	"github.com/ym/fxtrade/internal/strategy"
)

func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	cfg.Strategy.Enabled = true

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	notifier := notify.New(cfg.Email)
	rm := risk.NewManager(cfg.Risk)
	exec := execution.NewExecutor(client, rm, notifier)

	var sw *sentiment.Worker
	if cfg.SentimentEnabled() {
		w, err := sentiment.NewWorker(cfg, client, notifier)
		if err == nil {
			if _, err := w.RunOnce(context.Background()); err != nil {
				slog.Warn("sentiment refresh failed", "error", err)
			}
			sw = w
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	for _, inst := range cfg.Instruments {
		var cache *sentiment.Cache
		if sw != nil {
			cache = sw.CacheFor(inst)
		}
		engine := strategy.NewEngine(cfg, inst, client, exec, rm, notifier, cache)
		engine.RunOnce(ctx)
		slog.Info("strategy cycle complete", "instrument", inst, "mode", engine.LastMode())
	}
}
