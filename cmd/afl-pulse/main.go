package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/fxtrade/internal/afl"
	"github.com/ym/fxtrade/internal/afl/odds"
	"github.com/ym/fxtrade/internal/afl/stats"
	"github.com/ym/fxtrade/internal/afl/weather"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/notify"
)

// AFLPulse — on-demand AFL value betting scanner.
//
// On demand: go run ./cmd/afl-pulse -credentials .credentials
// Safe test:  go run ./cmd/afl-pulse -credentials .credentials -dry-run
func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dryRun := flag.Bool("dry-run", false, "evaluate and log value bets without sending email")
	injuriesPath := flag.String("injuries", "", "optional path to injuries JSON override")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.AFL.Validate(); err != nil {
		slog.Error("afl config", "error", err, "hint", "set afl.odds_api_key in .credentials")
		os.Exit(1)
	}

	timeoutMin := cfg.AFL.OverallTimeoutMin
	if timeoutMin <= 0 {
		timeoutMin = 5
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMin)*time.Minute)
	defer cancel()

	repo, err := stats.NewRepository(cfg.AFL.StatsDir)
	if err != nil {
		slog.Error("load stats", "error", err, "dir", cfg.AFL.StatsDir)
		os.Exit(1)
	}

	predictor, err := afl.NewPredictor(cfg.AFL.PredictorType, cfg.AFL.ModelPath, cfg.AFL.ONNXModelPath)
	if err != nil {
		slog.Error("load predictor", "error", err)
		os.Exit(1)
	}

	oddsClient := odds.NewClient(cfg.AFL)
	marketOdds, err := oddsClient.FetchOdds(ctx, repo.ResolveTeam)
	if err != nil {
		slog.Error("fetch odds", "error", err)
		os.Exit(1)
	}
	slog.Info("odds fetched", "outcomes", len(marketOdds))
	if len(marketOdds) == 0 {
		slog.Info("no upcoming AFL markets; exiting")
		return
	}

	injuries := cfg.AFL.InjuriesFile
	if *injuriesPath != "" {
		injuries = *injuriesPath
	}

	wx := weather.NewClient()
	builder := &afl.RepositoryContextBuilder{
		Repo:         repo,
		Weather:      wx,
		InjuriesFile: injuries,
	}
	evaluator := afl.NewEvaluator(cfg.AFL, predictor, builder)

	valueBets, err := evaluator.Evaluate(ctx, marketOdds)
	if err != nil {
		slog.Warn("evaluate completed with errors", "error", err)
	}
	slog.Info("evaluation complete", "summary", afl.FormatEvaluateSummary(valueBets))

	if len(valueBets) == 0 {
		slog.Info("no value bets above threshold; no alert sent", "min_ev", cfg.AFL.MinEVThreshold)
		return
	}

	topN := afl.TopN(valueBets, cfg.AFL.AlertTopN)
	for i, vb := range topN {
		slog.Info("value bet",
			"rank", i+1,
			"match", string(vb.HomeTeam)+" vs "+string(vb.AwayTeam),
			"pick", vb.Team,
			"odds", vb.DecimalOdds,
			"bookmaker", vb.Bookmaker,
			"model_prob", vb.ModelProb,
			"ev", vb.EV,
		)
	}

	if *dryRun {
		slog.Info("dry-run: skipping email alert")
		return
	}

	notifier := notify.New(cfg.Email)
	prefix := cfg.Notifications.EffectiveAFLPrefix()
	afl.SendAlertMulti(notifier, ctx, cfg.AFL, cfg.Notifications, valueBets)
	slog.Info("alert dispatched", "pick", topN[0].Team, "prefix", prefix)
}
