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

// AFLPulse pregame — T-30 fixture alerts with Gemini + Google Search.
//
// Scheduled on GCP via afl-pulse-pregame.timer (every 5 min).
// Safe test: go run ./cmd/afl-pulse-pregame -credentials .credentials -dry-run
func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	dryRun := flag.Bool("dry-run", false, "evaluate and log without sending email or updating dedup state")
	injuriesPath := flag.String("injuries", "", "optional path to injuries JSON override")
	logPath := flag.String("log-path", "/opt/fxtrade/logs/afl-pulse-pregame.log", "log path hint for failure alerts")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	if err := cfg.AFL.Validate(); err != nil {
		slog.Error("afl config", "error", err)
		os.Exit(1)
	}
	if cfg.AFL.GeminiAPIKey == "" {
		slog.Error("pregame requires afl.gemini_api_key")
		os.Exit(1)
	}

	timeoutMin := cfg.AFL.OverallTimeoutMin
	if timeoutMin <= 0 {
		timeoutMin = 5
	}
	if timeoutMin < 10 {
		timeoutMin = 10
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMin)*time.Minute)
	defer cancel()

	lead := cfg.AFL.ResolvedPregameLead()
	window := cfg.AFL.ResolvedPregamePollWindow()
	now := time.Now().UTC()

	year := cfg.AFL.StatsSeasonYear
	if year == 0 {
		year = now.Year()
	}
	squiggle := &stats.SquiggleClient{UserAgent: cfg.AFL.SquiggleUserAgent}
	upcoming, err := squiggle.FetchUpcomingGames(ctx, year)
	if err != nil {
		slog.Error("fetch squiggle upcoming", "error", err)
		os.Exit(1)
	}
	inWindow := stats.GamesInPregameWindow(upcoming, now, lead, window)
	if len(inWindow) == 0 {
		slog.Info("no fixtures in pregame window; exiting",
			"lead_min", int(lead.Minutes()),
			"window_min", int(window.Minutes()),
		)
		return
	}

	statePath := cfg.AFL.ResolvedPregameStatePath()
	state, err := afl.LoadPregameState(statePath)
	if err != nil {
		slog.Error("load pregame state", "path", statePath, "error", err)
		os.Exit(1)
	}
	state.Prune(now.Add(-7 * 24 * time.Hour))

	var pending []stats.UpcomingGame
	for _, g := range inWindow {
		if state.WasSent(g.ID) {
			slog.Info("pregame already sent", "squiggle_id", g.ID, "match", fmt.Sprintf("%s vs %s", g.HomeTeam, g.AwayTeam))
			continue
		}
		pending = append(pending, g)
	}
	if len(pending) == 0 {
		slog.Info("all in-window fixtures already sent; exiting")
		return
	}
	slog.Info("pregame fixtures pending", "count", len(pending))

	repo, err := stats.NewRepository(cfg.AFL.StatsDir)
	if err != nil {
		slog.Error("load stats", "error", err)
		os.Exit(1)
	}
	if cfg.AFL.StatsRefreshEnabled() {
		meta, err := repo.RefreshLiveStats(ctx, year, squiggle)
		if err != nil {
			slog.Warn("live stats refresh failed; using seed", "error", err)
		} else {
			slog.Info("live stats refreshed", "round", meta.Round)
		}
	}

	oddsClient := odds.NewClient(cfg.AFL)
	fetched, err := oddsClient.FetchOdds(ctx, repo.ResolveTeam)
	if err != nil {
		slog.Error("fetch odds", "error", err)
		os.Exit(1)
	}

	predictor, err := afl.NewPredictor(cfg.AFL.PredictorType, cfg.AFL.ModelPath, cfg.AFL.ONNXModelPath)
	if err != nil {
		slog.Error("load predictor", "error", err)
		os.Exit(1)
	}
	if tp, err := afl.NewLinearPredictor(cfg.AFL.TotalsModelPath); err == nil {
		afl.TotalsPredictor = tp
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
	evaluator.SetReportMeta(afl.DataProvenance{PredictorType: cfg.AFL.PredictorType, InjuriesFile: injuries})

	var notifier notify.Notifier
	if !*dryRun {
		notifier = notify.New(cfg.Email)
	}

	sentAny := false
	for _, game := range pending {
		sfix := squiggleFixtureFrom(game)
		fixture, h2h, totals, ok := afl.MatchSquiggleToOdds(sfix, fetched.H2H, fetched.Totals, repo.ResolveTeam)
		if !ok {
			slog.Warn("no odds match for squiggle game",
				"squiggle_id", game.ID,
				"match", fmt.Sprintf("%s vs %s", game.HomeTeam, game.AwayTeam),
			)
			continue
		}

		reports, _, err := evaluator.BuildRoundReports(ctx, []afl.Fixture{fixture}, h2h, totals)
		if err != nil {
			slog.Warn("build report failed", "squiggle_id", game.ID, "error", err)
			continue
		}
		if len(reports) == 0 {
			slog.Warn("empty report", "squiggle_id", game.ID)
			continue
		}
		report := reports[0]

		llmResp, llmErr := afl.RunPregameLLM(ctx, cfg.AFL, sfix, report)
		if llmErr != nil {
			slog.Error("pregame gemini failed",
				"squiggle_id", game.ID,
				"match", fmt.Sprintf("%s vs %s", fixture.HomeTeam, fixture.AwayTeam),
				"error", llmErr,
			)
			if !*dryRun {
				alertBody := afl.FormatPregameFailureAlert(
					sfix, fixture.HomeTeam, fixture.AwayTeam, game.Venue, llmErr, *logPath,
				)
				afl.SendPregameFailureAlert(notifier, ctx, cfg.Notifications, fixture.HomeTeam, fixture.AwayTeam, alertBody)
			}
			continue
		}

		body := afl.FormatPregameEmail(sfix, report, llmResp)
		if *dryRun {
			fmt.Println(body)
			slog.Info("dry-run: pregame email ready",
				"squiggle_id", game.ID,
				"match", fmt.Sprintf("%s vs %s", fixture.HomeTeam, fixture.AwayTeam),
			)
			continue
		}

		afl.SendPregameEmail(notifier, ctx, cfg.Notifications, fixture.HomeTeam, fixture.AwayTeam, body)
		state.MarkSent(game.ID)
		sentAny = true
		slog.Info("pregame email sent", "squiggle_id", game.ID)
	}

	if sentAny && !*dryRun {
		if err := state.Save(statePath); err != nil {
			slog.Error("save pregame state", "path", statePath, "error", err)
			os.Exit(1)
		}
	}
}

func squiggleFixtureFrom(g stats.UpcomingGame) afl.SquiggleFixture {
	return afl.SquiggleFixture{
		ID: g.ID, HomeTeam: g.HomeTeam, AwayTeam: g.AwayTeam,
		Kickoff: g.Kickoff, Round: g.Round, Venue: g.Venue,
	}
}
