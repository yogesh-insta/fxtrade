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
// Scans two universes and emails up to two suggestions:
//  1. One pick from the Nifty 200 watchlist
//  2. One pick from Nifty 500 excluding any symbol already in the Nifty 200 list
//
// Scheduled on the GCP VM via nifty-pulse.timer (18:00 Australia/Sydney, Sun–Fri).
// On demand: sudo systemctl start nifty-pulse.service, or ./scripts/nifty-pulse-run.sh
func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	watchlistPath := flag.String("watchlist", "watchlist.txt", "Nifty 200 NSE symbol watchlist (one per line)")
	watchlistExtendedPath := flag.String("watchlist-extended", "watchlist-nifty500-rest.txt",
		"Nifty 500 ex-200 watchlist (duplicates vs -watchlist are dropped)")
	dryRun := flag.Bool("dry-run", false, "scan and log picks without sending email")
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
		timeoutMin = 15
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMin)*time.Minute)
	defer cancel()

	primary, err := stockscan.LoadWatchlist(*watchlistPath)
	if err != nil {
		slog.Error("load watchlist", "path", *watchlistPath, "error", err)
		os.Exit(1)
	}

	extendedRaw, err := stockscan.LoadWatchlist(*watchlistExtendedPath)
	if err != nil {
		slog.Error("load extended watchlist", "path", *watchlistExtendedPath, "error", err)
		os.Exit(1)
	}
	extended := stockscan.ExcludeSymbols(extendedRaw, primary)
	if dropped := len(extendedRaw) - len(extended); dropped > 0 {
		slog.Info("dropped duplicate symbols from extended watchlist",
			"dropped", dropped,
			"extended_unique", len(extended),
		)
	}

	scanner := stockscan.NewScanner(cfg.StockScan)
	sentiment := stockscan.NewSentimentChecker(cfg)
	topN := cfg.StockScan.SentimentCandidates
	if topN <= 0 {
		topN = 3
	}

	type universeRun struct {
		label    string
		symbols  []string
		pathHint string
	}
	runs := []universeRun{
		{label: stockscan.UniverseNifty200, symbols: primary, pathHint: *watchlistPath},
		{label: stockscan.UniverseNifty500Rest, symbols: extended, pathHint: *watchlistExtendedPath},
	}

	var picks []stockscan.Pick
	contendersBySymbol := make(map[string][]stockscan.Contender)
	passedBySymbol := make(map[string]int)

	for _, run := range runs {
		if len(run.symbols) == 0 {
			slog.Info("skipping empty universe", "universe", run.label)
			continue
		}
		ranked, _, err := scanner.RunSymbols(ctx, run.symbols, run.pathHint)
		if err != nil {
			slog.Error("scan failed", "universe", run.label, "error", err)
			os.Exit(1)
		}
		if len(ranked) == 0 {
			slog.Info("no candidates passed filters", "universe", run.label)
			continue
		}

		sentimentPick, err := sentiment.PickWithSentiment(ctx, ranked, topN)
		if err != nil {
			slog.Error("sentiment pick failed", "universe", run.label, "error", err)
			os.Exit(1)
		}
		if sentimentPick == nil {
			slog.Info("no pick after sentiment gate", "universe", run.label)
			continue
		}

		var sentimentNote string
		if sentimentPick.SentimentUsed {
			sentimentNote = stockscan.FormatSentimentNote(sentimentPick.Signal)
		}
		reasons := stockscan.BuildReasons(sentimentPick.Candidate, cfg.StockScan, sentimentNote)
		trade := stockscan.BuildPickUniverse(sentimentPick.Candidate, cfg.StockScan, reasons, run.label)
		contenders := stockscan.BuildContenders(stockscan.TopN(ranked, 5), cfg.StockScan, trade.Candidate.Symbol)

		slog.Info("pick selected",
			"universe", run.label,
			"symbol", trade.Candidate.Symbol,
			"entry", trade.Entry,
			"rsi", trade.Candidate.RSI,
			"sma", trade.Candidate.SMA,
		)

		picks = append(picks, trade)
		contendersBySymbol[trade.Candidate.Symbol] = contenders
		passedBySymbol[trade.Candidate.Symbol] = len(ranked)
	}

	if len(picks) == 0 {
		slog.Info("no picks after filters/sentiment; no alert sent")
		return
	}

	prefix := cfg.Notifications.EffectiveNSEPrefix()
	if *dryRun {
		subject := stockscan.FormatPicksSubject(prefix, picks)
		slog.Info("dry-run: picks ready (email not sent)",
			"subject", subject,
			"picks", len(picks),
			"would_send_to", cfg.Email.AlertTo,
		)
		for _, p := range picks {
			slog.Info("dry-run pick",
				"universe", p.Universe,
				"symbol", p.Candidate.Symbol,
				"entry", p.Entry,
				"stop_loss", p.StopLoss,
				"target", p.Target,
				"reasons", p.Reasons,
			)
		}
		return
	}

	notifier := notify.New(cfg.Email)
	stockscan.SendPicksAlert(notifier, ctx, prefix, picks, contendersBySymbol, passedBySymbol)
	for _, p := range picks {
		slog.Info("alert dispatched",
			"universe", p.Universe,
			"symbol", p.Candidate.Symbol,
			"to", cfg.Email.AlertTo,
		)
	}
}
