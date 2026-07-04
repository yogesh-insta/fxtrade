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

	if cfg.AFL.StatsRefreshEnabled() {
		year := cfg.AFL.StatsSeasonYear
		if year == 0 {
			year = time.Now().Year()
		}
		squiggle := &stats.SquiggleClient{UserAgent: cfg.AFL.SquiggleUserAgent}
		meta, err := repo.RefreshLiveStats(ctx, year, squiggle)
		if err != nil {
			slog.Warn("live stats refresh failed; using seed teams.json", "error", err)
		} else {
			slog.Info("live stats refreshed", "source", meta.Source, "year", meta.Year, "round", meta.Round)
		}
	}

	predictor, err := afl.NewPredictor(cfg.AFL.PredictorType, cfg.AFL.ModelPath, cfg.AFL.ONNXModelPath)
	if err != nil {
		slog.Error("load predictor", "error", err)
		os.Exit(1)
	}
	if tp, err := afl.NewLinearPredictor(cfg.AFL.TotalsModelPath); err == nil {
		afl.TotalsPredictor = tp
		slog.Info("totals model loaded", "path", cfg.AFL.TotalsModelPath)
	} else {
		slog.Info("heuristic totals model", "hint", "run cmd/afl-train to build totals_coefficients.json")
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
	injuriesCount := 0
	if injuries != "" {
		if n, err := repo.CountInjuries(injuries); err != nil {
			slog.Warn("injuries file not readable", "path", injuries, "error", err)
		} else if n > 0 {
			injuriesCount = n
			slog.Info("injuries loaded", "path", injuries, "unavailable", n)
		}
	}

	wx := weather.NewClient()
	builder := &afl.RepositoryContextBuilder{
		Repo:         repo,
		Weather:      wx,
		InjuriesFile: injuries,
	}
	evaluator := afl.NewEvaluator(cfg.AFL, predictor, builder)

	reportMeta := afl.DataProvenance{
		StatsSource:   "seed teams.json",
		PredictorType: cfg.AFL.PredictorType,
		InjuriesFile:  injuries,
	}
	if injuriesCount > 0 {
		reportMeta.InjuriesApplied = true
		reportMeta.InjuriesCount = injuriesCount
	}
	if live, ok := repo.LiveStatsMeta(); ok {
		reportMeta.StatsSource = live.Source
		reportMeta.StatsDetail = fmt.Sprintf("%d round %d", live.Year, live.Round)
		reportMeta.StatsAsOf = live.AsOf
	}
	evaluator.SetReportMeta(reportMeta)

	fixtures := afl.FixturesFromOdds(fetched.H2H)
	reports, valueBets, err := evaluator.BuildRoundReports(ctx, fixtures, fetched.H2H, fetched.Totals)
	if err != nil {
		slog.Warn("round report completed with errors", "error", err)
	}

	editorialPath := cfg.AFL.EditorialPicksFile
	if editorial, err := afl.LoadRoundEditorial(editorialPath); err != nil {
		slog.Warn("editorial picks not loaded", "path", editorialPath, "error", err)
	} else if len(editorial.Fixtures) > 0 {
		afl.AttachEditorialPicks(reports, editorial)
		slog.Info("editorial picks attached", "path", editorialPath, "fixtures", len(editorial.Fixtures), "round", editorial.Round)
	}

	slog.Info("round report complete", "summary", afl.FormatRoundReportSummary(reports, valueBets))

	for i, r := range reports {
		block := afl.FormatFixturePredictionBlock(r)
		if *dryRun {
			if i > 0 {
				fmt.Println()
			}
			fmt.Println(block)
			if len(r.ValueBets) > 0 {
				fmt.Printf("  Value bets: %d\n", len(r.ValueBets))
			}
		} else {
			slog.Info("fixture prediction", "block", block, "value_bets", len(r.ValueBets))
		}
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
