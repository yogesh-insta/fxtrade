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
	"github.com/ym/fxtrade/internal/store/sqlite"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dbPath := flag.String("db", "", "SQLite trades.db (default from btc_cfd config)")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	path := *dbPath
	if path == "" {
		path = cfg.BtcCfd.DBPath
	}

	store, err := sqlite.Open(path)
	if err != nil {
		slog.Error("open db", "error", err)
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

	m, err := store.MetricsReport(balance, cfg.BtcCfd.MaxDailyLossPct)
	if err != nil {
		slog.Error("metrics", "error", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(m, "", "  ")
	fmt.Println(string(out))
	fmt.Printf("\nwin_rate=%.1f%% realized_rr=%.2f max_dd=%.2f daily_headroom=%.2f\n",
		m.WinRate*100, m.RealizedRR, m.MaxDrawdown, m.DailyHeadroom)
}
