package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ym/fxtrade/internal/afl"
	"github.com/ym/fxtrade/internal/afl/stats"
	"github.com/ym/fxtrade/internal/afl/train"
	"github.com/ym/fxtrade/internal/config"
)

// Backtest trained AFL models on a holdout season (no parameter fitting on test year).
//
//	go run ./cmd/afl-backtest -credentials .credentials -holdout 2025
func main() {
	credentials := flag.String("credentials", ".credentials", "credentials JSON path")
	statsDir := flag.String("stats-dir", "", "override AFL stats dir")
	fromYear := flag.Int("from", 2018, "first season year")
	toYear := flag.Int("to", time.Now().Year(), "last season year")
	holdout := flag.Int("holdout", 0, "holdout season (default: -to)")
	winModel := flag.String("win-model", "data/afl/model_coefficients.json", "H2H model path")
	totalsModel := flag.String("totals-model", "data/afl/totals_coefficients.json", "totals model path")
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

	holdoutYear := *holdout
	if holdoutYear == 0 {
		holdoutYear = *toYear
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
	_, testSet := train.SplitHoldout(examples, holdoutYear)
	if len(testSet) == 0 {
		slog.Error("empty holdout set")
		os.Exit(1)
	}

	winCoeffs, err := readMatrixModel(*winModel)
	if err != nil {
		slog.Error("read win model", "error", err)
		os.Exit(1)
	}
	totCoeffs, err := readLinearModel(*totalsModel)
	if err != nil {
		slog.Error("read totals model", "error", err)
		os.Exit(1)
	}

	winMetrics := train.EvaluateWinModel(winCoeffs, testSet)
	totalsMetrics := train.EvaluateTotalsModel(totCoeffs, testSet)

	fmt.Println(train.FormatReport(holdoutYear, winMetrics, totalsMetrics))
	fmt.Printf("H2H Brier %.3f\n", winMetrics.Brier)
}

func readMatrixModel(path string) (afl.MatrixModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return afl.MatrixModel{}, err
	}
	var m afl.MatrixModel
	if err := json.Unmarshal(data, &m); err != nil {
		return afl.MatrixModel{}, err
	}
	return m, nil
}

func readLinearModel(path string) (afl.LinearModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return afl.LinearModel{}, err
	}
	var m afl.LinearModel
	if err := json.Unmarshal(data, &m); err != nil {
		return afl.LinearModel{}, err
	}
	return m, nil
}
