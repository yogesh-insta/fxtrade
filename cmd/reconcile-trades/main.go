package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/report"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	botID := flag.String("bot", "", "bot id (default: all platform bots)")
	installRoot := flag.String("root", "", "optional install root (e.g. /opt/fxtrade) to resolve relative db paths")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if *installRoot != "" {
		if err := os.Chdir(*installRoot); err != nil {
			slog.Warn("chdir", "root", *installRoot, "error", err)
		}
	}
	if cfg.OANDA.Token == "" {
		slog.Error("oanda token not configured")
		os.Exit(1)
	}

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	ctx := context.Background()

	if *botID != "" {
		res := report.ReconcileBot(ctx, cfg, *botID, client)
		printResult(res)
		if res.Err != nil {
			os.Exit(1)
		}
		if res.StillMissing > 0 {
			os.Exit(2)
		}
		return
	}

	summary := report.ReconcileAllBots(ctx, cfg, client)
	exitCode := 0
	for _, res := range summary.Results {
		printResult(res)
		if res.Err != nil {
			exitCode = 1
		} else if res.StillMissing > 0 {
			exitCode = 2
		}
	}
	fmt.Printf("\nTotal reconciled: %d\n", summary.TotalReconciled)
	os.Exit(exitCode)
}

func printResult(res report.ReconcileResult) {
	name := report.BotDisplayName(res.BotID)
	if res.Err != nil {
		fmt.Printf("%s: error %v (db %s)\n", name, res.Err, res.DBPath)
		return
	}
	fmt.Printf("%s: attempted %d, reconciled %d, still missing %d (db %s)\n",
		name, res.Attempted, res.Reconciled, res.StillMissing, res.DBPath)
}
