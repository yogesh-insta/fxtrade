package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/stockscan"
)

// NiftyPulse — daily NSE swing scanner. Run via cron on the GCP VM, e.g.:
//
//	30 3 * * 1-5 cd /opt/fxtrade && /usr/local/bin/nifty-pulse -credentials .credentials -watchlist watchlist.txt >> logs/nifty-pulse.log 2>&1
//
// 03:30 UTC ≈ 09:00 IST (adjust for DST if needed).
func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	watchlistPath := flag.String("watchlist", "watchlist.txt", "path to NSE symbol watchlist (one per line)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	timeoutMin := cfg.StockScan.OverallTimeoutMin
	if timeoutMin <= 0 {
		timeoutMin = 10
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMin)*time.Minute)
	defer cancel()

	scanner := stockscan.NewScanner(cfg.StockScan)
	ranked, _, err := scanner.Run(ctx, *watchlistPath)
	if err != nil {
		slog.Error("scan failed", "error", err)
		os.Exit(1)
	}
	if len(ranked) == 0 {
		slog.Info("no candidates passed filters; no alert sent")
		return
	}

	sentiment := stockscan.NewSentimentChecker(cfg)
	topN := cfg.StockScan.SentimentCandidates
	if topN <= 0 {
		topN = 3
	}
	pick, err := sentiment.PickWithSentiment(ctx, ranked, topN)
	if err != nil {
		slog.Error("sentiment pick failed", "error", err)
		os.Exit(1)
	}
	if pick == nil {
		slog.Info("no pick after sentiment gate; no alert sent")
		return
	}

	trade := stockscan.BuildPick(*pick, cfg.StockScan)
	slog.Info("pick selected",
		"symbol", trade.Candidate.Symbol,
		"entry", trade.Entry,
		"rsi", trade.Candidate.RSI,
		"sma", trade.Candidate.SMA,
	)

	prefix := cfg.Notifications.EffectiveNSEPrefix()
	notifier := notify.New(cfg.Email)
	stockscan.SendAlert(notifier, ctx, prefix, trade)
	slog.Info("alert dispatched", "symbol", trade.Candidate.Symbol, "to", cfg.Email.AlertTo)
}
