package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/report"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	botID := flag.String("bot", config.BotBtcCfd, "bot id: universe_scanner, fx_sentiment, btc_cfd")
	dbPath := flag.String("db", "", "SQLite trades.db (default from bot config)")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	id := config.NormalizeBotID(*botID)
	path, maxDailyLossPct := dbPathForBot(cfg, id, *dbPath)

	store, err := sqlite.Open(path)
	if err != nil {
		slog.Error("open db", "bot", id, "path", path, "error", err)
		os.Exit(1)
	}
	defer store.Close()

	balance := 0.0
	if cfg.OANDA.Token != "" {
		client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
		if summary, err := client.AccountSummary(context.Background()); err == nil {
			balance, _ = oanda.ParsePrice(summary.Account.Balance)
		}
	}

	m, err := store.MetricsReport(balance, maxDailyLossPct)
	if err != nil {
		slog.Error("metrics", "error", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(m, "", "  ")
	fmt.Printf("bot=%s db=%s\n", id, path)
	fmt.Println(string(out))
	fmt.Printf("\nwin_rate=%.1f%% realized_rr=%.2f max_dd=%.2f daily_headroom=%.2f\n",
		m.WinRate*100, m.RealizedRR, m.MaxDrawdown, m.DailyHeadroom)
}

func dbPathForBot(cfg *config.Config, botID, override string) (string, float64) {
	if override != "" {
		return override, dailyLossPctForBot(cfg, botID)
	}
	return report.BotDBPath(cfg, botID), dailyLossPctForBot(cfg, botID)
}

func dailyLossPctForBot(cfg *config.Config, botID string) float64 {
	switch botID {
	case config.BotBtcCfd:
		return cfg.BtcCfd.MaxDailyLossPct
	case config.BotUniverseScanner:
		return cfg.Scanner.DailyLossCapPct
	default:
		return cfg.Risk.MaxDailyLossPct
	}
}
