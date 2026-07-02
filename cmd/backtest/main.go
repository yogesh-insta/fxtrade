package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ym/fxtrade/internal/backtest"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dailyCount := flag.Int("days", 200, "daily candles to fetch")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	daily, err := client.Candles(ctx, oanda.DefaultInstrument, "D", *dailyCount)
	if err != nil {
		slog.Error("daily candles", "error", err)
		os.Exit(1)
	}
	weekly, err := client.Candles(ctx, oanda.DefaultInstrument, "W", 52)
	if err != nil {
		slog.Error("weekly candles", "error", err)
		os.Exit(1)
	}
	h4, err := client.Candles(ctx, oanda.DefaultInstrument, "H4", 120)
	if err != nil {
		slog.Error("h4 candles", "error", err)
		os.Exit(1)
	}

	result := backtest.Run(daily.Candles, weekly.Candles, h4.Candles, cfg)
	slog.Info("backtest complete", "summary", result.Summary())
}
