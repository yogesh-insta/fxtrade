package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/config"

	_ "github.com/ym/fxtrade/internal/bots"
)

func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	healthAddr := flag.String("health-addr", ":8080", "health HTTP listen address")
	botFilter := flag.String("bot", "", "run only this bot id (for GCP: one systemd unit per bot)")
	dryRun := flag.Bool("dry-run", false, "run full daemon but do not place or close OANDA orders")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err, "hint", "copy .credentials.example to .credentials and fill OANDA demo account_id + token")
		os.Exit(1)
	}

	if err := bot.RunPlatform(cfg, bot.Options{
		HealthAddr: *healthAddr,
		BotFilter:  *botFilter,
		DryRun:     *dryRun,
	}); err != nil {
		slog.Error("platform exited", "error", err)
		os.Exit(1)
	}
}
