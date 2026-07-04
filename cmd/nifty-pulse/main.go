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

// NiftyPulse — daily NSE swing scanner.
//
// Scheduled on the GCP VM via nifty-pulse.timer (18:00 Australia/Sydney, Sun–Fri).
// On demand: sudo systemctl start nifty-pulse.service, or ./scripts/nifty-pulse-run.sh
func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	watchlistPath := flag.String("watchlist", "watchlist.txt", "path to NSE symbol watchlist (one per line)")
	dryRun := flag.Bool("dry-run", false, "scan and log pick without sending email")
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
	sentimentPick, err := sentiment.PickWithSentiment(ctx, ranked, topN)
	if err != nil {
		slog.Error("sentiment pick failed", "error", err)
		os.Exit(1)
	}
	if sentimentPick == nil {
		slog.Info("no pick after sentiment gate; no alert sent")
		return
	}

	var sentimentNote string
	if sentimentPick.SentimentUsed {
		sentimentNote = stockscan.FormatSentimentNote(sentimentPick.Signal)
	}
	reasons := stockscan.BuildReasons(sentimentPick.Candidate, cfg.StockScan, sentimentNote)
	trade := stockscan.BuildPick(sentimentPick.Candidate, cfg.StockScan, reasons)
	contenders := stockscan.BuildContenders(stockscan.TopN(ranked, 5), cfg.StockScan, trade.Candidate.Symbol)

	slog.Info("pick selected",
		"symbol", trade.Candidate.Symbol,
		"entry", trade.Entry,
		"rsi", trade.Candidate.RSI,
		"sma", trade.Candidate.SMA,
	)

	prefix := cfg.Notifications.EffectiveNSEPrefix()
	if *dryRun {
		subject := stockscan.FormatAlertSubject(prefix, trade)
		slog.Info("dry-run: pick ready (email not sent)",
			"subject", subject,
			"symbol", trade.Candidate.Symbol,
			"entry", trade.Entry,
			"stop_loss", trade.StopLoss,
			"target", trade.Target,
			"reasons", trade.Reasons,
			"would_send_to", cfg.Email.AlertTo,
		)
		return
	}

	notifier := notify.New(cfg.Email)
	stockscan.SendAlert(notifier, ctx, prefix, trade, contenders, len(ranked))
	slog.Info("alert dispatched", "symbol", trade.Candidate.Symbol, "to", cfg.Email.AlertTo)
}
