package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/sentiment"
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

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	notifier := notify.New(cfg.Email)

	worker, err := sentiment.NewWorker(cfg, client, notifier)
	if err != nil {
		slog.Error("sentiment worker", "error", err, "hint", "add finnhub.api_key and llm.api_key to .credentials")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	signal, err := worker.RunOnce(ctx)
	if err != nil {
		slog.Error("sentiment cycle failed", "error", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(signal, "", "  ")
	os.Stdout.Write(out)
	os.Stdout.Write([]byte("\n"))
}
