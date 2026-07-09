package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/scanner"
)

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dryRun := flag.Bool("dry-run", true, "scan and rank only; no orders")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if !cfg.ScannerEnabled() && !contains(cfg.EnabledBots(), config.BotUniverseScanner) {
		slog.Error("scanner-test requires universe_scanner bot enabled")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	universe, err := scanner.ResolveUniverse(ctx, cfg, client)
	if err != nil {
		slog.Error("resolve universe", "error", err)
		os.Exit(1)
	}

	rm := risk.NewManagerForEnv(cfg.Risk, cfg.OANDA.Environment)
	notifier := scanner.NewNotifier(cfg, nil, false)
	engine := scanner.NewEngine(cfg, client, execution.NewExecutor(client, rm, nil), rm, notifier, universe)

	ranked := engine.DryRun(ctx)
	fmt.Printf("Universe: %d instruments\n\n", len(universe.Symbols))
	if len(ranked) == 0 {
		fmt.Println("No setups above min_setup_score.")
		os.Exit(0)
	}

	fmt.Printf("Top setups (min score %.2f):\n\n", cfg.Scanner.MinSetupScore)
	for i, s := range ranked {
		if i >= 10 {
			break
		}
		dir := s.BreakoutDirection
		if dir == "" {
			dir = "-"
		}
		fmt.Printf("%2d. %-12s score=%.3f range=%.1f pips spread=%.2f ratio=%.1f trend=%s breakout=%s\n",
			i+1, s.Instrument, s.Score, s.Range.RangePips, s.SpreadPips, s.RangeSpreadRatio,
			trendName(s.TrendBias), dir)
		if s.SkipReason != "" {
			fmt.Printf("    skip: %s\n", s.SkipReason)
		}
	}

	if *dryRun {
		fmt.Println("\n(dry-run — no orders placed)")
	}
}

func trendName(bias int) string {
	switch bias {
	case 1:
		return "bullish"
	case -1:
		return "bearish"
	default:
		return "neutral"
	}
}
