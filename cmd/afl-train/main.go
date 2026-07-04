package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/ym/fxtrade/internal/afl/train"
	"github.com/ym/fxtrade/internal/afl/stats"
	"github.com/ym/fxtrade/internal/config"
)

// Train AFL win-probability and totals models from Squiggle historical games.
//
//	go run ./cmd/afl-train -credentials .credentials -from 2018 -to 2025 -holdout 2025
func main() {
	credentials := flag.String("credentials", ".credentials", "credentials JSON path")
	statsDir := flag.String("stats-dir", "", "override AFL stats dir")
	fromYear := flag.Int("from", 2018, "first season year")
	toYear := flag.Int("to", time.Now().Year(), "last season year")
	holdout := flag.Int("holdout", 0, "holdout season for eval (0 = use -to year only on test)")
	winOut := flag.String("win-model", "data/afl/model_coefficients.json", "output H2H model path")
	totalsOut := flag.String("totals-model", "data/afl/totals_coefficients.json", "output totals model path")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentials)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	dir := cfg.AFL.StatsDir
	if *statsDir != "" {
		dir = *statsDir
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	repo, err := stats.NewRepository(dir)
	if err != nil {
		slog.Error("load stats repo", "error", err)
		os.Exit(1)
	}

	client := &stats.SquiggleClient{UserAgent: cfg.AFL.SquiggleUserAgent}
	examples, err := train.BuildExamples(ctx, repo, client, *fromYear, *toYear)
	if err != nil {
		slog.Error("build dataset", "error", err)
		os.Exit(1)
	}
	if len(examples) == 0 {
		slog.Error("no training examples")
		os.Exit(1)
	}
	slog.Info("dataset built", "examples", len(examples), "from", *fromYear, "to", *toYear)

	holdoutYear := *holdout
	if holdoutYear == 0 {
		holdoutYear = *toYear
	}
	trainSet, testSet := train.SplitHoldout(examples, holdoutYear)
	if len(trainSet) == 0 {
		slog.Error("empty training set; lower holdout year")
		os.Exit(1)
	}
	slog.Info("split", "train", len(trainSet), "holdout_test", len(testSet), "holdout_year", holdoutYear)

	winModel := train.TrainWinModel(trainSet)
	totalsModel := train.TrainTotalsModel(trainSet)

	winMetrics := train.EvaluateWinModel(winModel, testSet)
	totalsMetrics := train.EvaluateTotalsModel(totalsModel, testSet)
	slog.Info("evaluation", "report", train.FormatReport(holdoutYear, winMetrics, totalsMetrics))

	if err := train.SaveModels(*winOut, *totalsOut, winModel, totalsModel); err != nil {
		slog.Error("save models", "error", err)
		os.Exit(1)
	}
	slog.Info("models saved", "win", *winOut, "totals", *totalsOut)
}
