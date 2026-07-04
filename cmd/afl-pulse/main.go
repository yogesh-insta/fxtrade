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

// AFLPulse — weekly AFL round scanner with score projections and value bets.
//
// Scheduled on the GCP VM via afl-pulse.timer (Thu 18:00 Australia/Melbourne).
// On demand: sudo systemctl start afl-pulse.service, or ./scripts/afl-pulse-run.sh
// Safe test:  go run ./cmd/afl-pulse -credentials .credentials -dry-run
func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dryRun := flag.Bool("dry-run", false, "evaluate and log predictions without sending email")
	injuriesPath := flag.String("injuries", "", "optional path to injuries JSON override")
	valueOnly := flag.Bool("value-only", false, "email only when value bets exist (legacy mode)")
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
	fetched, err := oddsClient.FetchOdds(ctx, repo.ResolveTeam)
	if err != nil {
		slog.Error("fetch odds", "error", err)
		os.Exit(1)
	}
	slog.Info("odds fetched", "h2h_outcomes", len(fetched.H2H), "totals_lines", len(fetched.Totals))
	if len(fetched.H2H) == 0 {
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

	fixtures := afl.FixturesFromOdds(fetched.H2H)
	reports, valueBets, err := evaluator.BuildRoundReports(ctx, fixtures, fetched.H2H, fetched.Totals)
	if err != nil {
		slog.Warn("round report completed with errors", "error", err)
	}
	slog.Info("round report complete", "summary", afl.FormatRoundReportSummary(reports, valueBets))

	for _, r := range reports {
		slog.Info("fixture prediction",
			"match", string(r.Context.HomeTeam)+" vs "+string(r.Context.AwayTeam),
			"winner", r.Score.PredictedWinner,
			"score", r.Score.HomeScore,
			"away_score", r.Score.AwayScore,
			"total", r.Score.TotalScore,
			"margin", r.Score.Margin,
			"home_win_prob", r.HomeWinProb,
			"value_bets", len(r.ValueBets),
		)
	}

	for i, vb := range afl.TopN(valueBets, cfg.AFL.AlertTopN) {
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

	if *valueOnly && len(valueBets) == 0 {
		slog.Info("value-only mode: no value bets above threshold; no alert sent", "min_ev", cfg.AFL.MinEVThreshold)
		return
	}
	if len(reports) == 0 {
		slog.Info("no fixture reports generated; exiting")
		return
	}

	if *dryRun {
		slog.Info("dry-run: skipping email alert")
		return
	}

	notifier := notify.New(cfg.Email)
	if *valueOnly {
		afl.SendAlertMulti(notifier, ctx, cfg.AFL, cfg.Notifications, valueBets)
		slog.Info("value-only alert dispatched", "value_bets", len(valueBets))
		return
	}

	afl.SendRoundReport(notifier, ctx, cfg.AFL, cfg.Notifications, reports, valueBets)
	slog.Info("round report email dispatched", "fixtures", len(reports), "value_bets", len(valueBets))
}
