package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
)

func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	count := flag.Int("count", 5, "number of test round-trips")
	units := flag.Int64("units", 100, "units per test order (practice)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	riskCfg := cfg.Risk
	riskCfg.MaxTradesPerMonth = 100
	riskCfg.CooldownAfterLossDays = 0

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	notifier := notify.New(cfg.Email)
	rm := risk.NewManager(riskCfg)
	exec := execution.NewExecutor(client, rm, notifier)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	summary, err := client.AccountSummary(ctx)
	if err != nil {
		slog.Error("account summary", "error", err)
		os.Exit(1)
	}
	balance, _ := oanda.ParsePrice(summary.Account.Balance)
	slog.Info("order test starting", "balance", balance, "orders", *count, "units", *units)

	pricing, err := client.Pricing(ctx, oanda.DefaultInstrument)
	if err != nil {
		slog.Error("pricing", "error", err)
		os.Exit(1)
	}
	tick, err := pricing.Prices[0].ToUpdate()
	if err != nil {
		slog.Error("pricing parse", "error", err)
		os.Exit(1)
	}

	directions := []string{execution.DirectionLong, execution.DirectionShort}
	stopPips := 80.0
	stopDist := stopPips * oanda.PipSize

	for i := 0; i < *count; i++ {
		if ctx.Err() != nil {
			break
		}

		direction := directions[i%2]
		var entry, stop float64
		switch direction {
		case execution.DirectionLong:
			entry = tick.Ask
			stop = entry - stopDist
		default:
			entry = tick.Bid
			stop = entry + stopDist
		}

		open, err := client.OpenTrades(ctx)
		if err != nil {
			slog.Error("open trades", "error", err)
			os.Exit(1)
		}

		corr := fmt.Sprintf("test-%d-%d", i+1, time.Now().UnixNano())
		req := risk.EntryRequest{
			CorrelationID:  corr,
			SpreadPips:     oanda.SpreadPips(tick.Spread),
			OpenPositions:  len(open.Trades),
			Confidence:     0.5,
			AccountBalance: balance,
			StopDistance:   stopDist,
		}

		params := execution.MarketOrderParams{
			Instrument: oanda.DefaultInstrument,
			Direction:  direction,
			Units:      *units,
			StopLoss:   stop,
		}

		slog.Info("placing test order", "n", i+1, "direction", direction, "entry", entry, "stop", stop)
		result, err := exec.PlaceMarket(ctx, req, params)
		if err != nil {
			slog.Error("place order failed", "n", i+1, "error", err)
			os.Exit(1)
		}

		time.Sleep(2 * time.Second)

		pl, err := exec.CloseTrade(ctx, result.TradeID, corr)
		if err != nil {
			slog.Error("close trade failed", "trade_id", result.TradeID, "error", err)
			os.Exit(1)
		}
		slog.Info("round trip complete", "n", i+1, "trade_id", result.TradeID, "pl", pl)

		pricing, err = client.Pricing(ctx, oanda.DefaultInstrument)
		if err != nil {
			slog.Error("pricing refresh", "error", err)
			os.Exit(1)
		}
		tick, err = pricing.Prices[0].ToUpdate()
		if err != nil {
			slog.Error("pricing parse", "error", err)
			os.Exit(1)
		}

		time.Sleep(time.Second)
	}

	slog.Info("order test finished", "completed", *count)
	fmt.Printf("Phase 2 gate: placed and closed %d test orders on practice account\n", *count)
}
