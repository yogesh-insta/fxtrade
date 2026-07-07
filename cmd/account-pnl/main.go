package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

func main() {
	credsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	flag.Parse()

	cfg, err := config.Load(*credsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	baseline := cfg.OANDA.InitialCapitalAUD
	if baseline <= 0 {
		slog.Error("oanda.initial_capital_aud not set in credentials")
		os.Exit(1)
	}

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	summary, err := client.AccountSummary(context.Background())
	if err != nil {
		slog.Error("account summary", "error", err)
		os.Exit(1)
	}

	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	nav, _ := oanda.ParsePrice(summary.Account.NAV)
	unrealized, _ := oanda.ParsePrice(summary.Account.UnrealizedPL)
	currency := summary.Account.Currency
	if currency == "" {
		currency = "AUD"
	}

	realized := balance - baseline
	total := nav - baseline
	pct := 0.0
	if baseline > 0 {
		pct = total / baseline * 100
	}

	fmt.Printf("account=%s env=%s currency=%s\n", cfg.OANDA.AccountID, cfg.OANDA.Environment, currency)
	if note := cfg.OANDA.InitialCapitalNote; note != "" {
		fmt.Printf("baseline_note=%s\n", note)
	}
	fmt.Printf("initial_capital=%.2f %s\n", baseline, currency)
	fmt.Printf("balance=%.2f realized_pnl=%.2f\n", balance, realized)
	fmt.Printf("unrealized_pnl=%.2f\n", unrealized)
	fmt.Printf("nav=%.2f total_pnl=%.2f (%.2f%%)\n", nav, total, pct)
}
