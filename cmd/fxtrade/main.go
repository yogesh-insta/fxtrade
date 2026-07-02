package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/health"
	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/monitor"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
	"github.com/ym/fxtrade/internal/state"
	"github.com/ym/fxtrade/internal/strategy"
)

func main() {
	credentialsPath := flag.String("credentials", ".credentials", "path to credentials JSON")
	healthAddr := flag.String("health-addr", ":8080", "health HTTP listen address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load(*credentialsPath)
	if err != nil {
		slog.Error("load config", "error", err, "hint", "copy .credentials.example to .credentials and fill OANDA demo account_id + token")
		os.Exit(1)
	}

	client := oanda.NewClient(cfg.OANDA.RESTBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	stream := oanda.NewStream(cfg.OANDA.StreamBaseURL(), cfg.OANDA.AccountID, cfg.OANDA.Token)
	notifier := notify.New(cfg.Email)
	rm := risk.NewManager(cfg.Risk)
	stateStore := state.NewStore(cfg.State.File)
	persisted, _ := stateStore.Load()

	tradeJournal := journal.New(cfg.Strategy.JournalDir)
	tradeRecorder := state.NewTradeRecorder(stateStore, tradeJournal)

	var sentimentWorker *sentiment.Worker
	var sentimentCache *sentiment.Cache

	exec := execution.NewExecutor(client, rm, notifier)
	exec.SetCloseHook(func(tradeID, correlationID string, pl float64) {
		tradeRecorder.OnClose(tradeID, correlationID, pl, rm, sentimentCache)
	})
	posMon := monitor.New(client, notifier, 30*time.Second)
	healthSrv := health.NewServer()
	healthSrv.SetHaltedCheck(rm.IsHalted)
	healthSrv.SetKillHandler(rm.ActivateKillSwitch)
	healthSrv.SetOpenPositions(posMon.OpenCount)

	if cfg.SentimentEnabled() {
		sw, err := sentiment.NewWorker(cfg, client, notifier)
		if err != nil {
			slog.Error("sentiment worker init", "error", err)
			os.Exit(1)
		}
		sentimentWorker = sw
		sentimentCache = sentimentWorker.Cache()
		if !persisted.SavedAt.IsZero() {
			state.ApplySnapshot(persisted, rm, sentimentCache)
			slog.Info("state restored", "saved_at", persisted.SavedAt, "trades_this_month", persisted.Risk.TradesThisMonth)
		}
		healthSrv.SetSentimentStatus(func() (bool, string, float64, time.Time) {
			sig, ok := sentimentWorker.Cache().Current(time.Now())
			if !ok {
				return true, "", 0, time.Time{}
			}
			return true, sig.Direction, sig.Confidence, sig.AnalyzedAt
		})
	} else {
		if !persisted.SavedAt.IsZero() {
			state.ApplySnapshot(persisted, rm, nil)
			slog.Info("state restored", "saved_at", persisted.SavedAt)
		}
		slog.Warn("sentiment pipeline disabled", "hint", "add finnhub.api_key and llm.api_key to .credentials")
	}

	stratEngine := strategy.NewEngine(cfg, client, exec, rm, notifier, sentimentCache)
	healthSrv.SetTradingMode(stratEngine.LastMode)

	saveState := func() {
		snap := state.BuildSnapshot(rm, sentimentCache, tradeRecorder.LastTrade())
		if err := stateStore.Save(snap); err != nil {
			slog.Warn("state save failed", "error", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := runStartupChecks(ctx, client, notifier); err != nil {
		slog.Error("startup checks failed", "error", err)
		os.Exit(1)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				saveState()
			}
		}
	}()

	ticks := make(chan oanda.PriceUpdate, 64)
	go func() {
		healthSrv.SetStreamConnected(true)
		defer healthSrv.SetStreamConnected(false)
		if err := stream.RunPricingStream(ctx, []string{oanda.DefaultInstrument}, ticks); err != nil && ctx.Err() == nil {
			slog.Error("pricing stream exited", "error", err)
		}
	}()

	go posMon.Run(ctx)

	if sentimentWorker != nil {
		go sentimentWorker.Run(ctx)
	}

	if cfg.Strategy.Enabled {
		go stratEngine.Run(ctx)
	} else {
		slog.Warn("strategy loop disabled", "hint", "set strategy.enabled=true in .credentials to trade")
	}

	go func() {
		slog.Info("health server listening", "addr", *healthAddr)
		if err := http.ListenAndServe(*healthAddr, healthSrv.Handler()); err != nil {
			slog.Error("health server", "error", err)
			cancel()
		}
	}()

	slog.Info("fxtrade running",
		"environment", cfg.OANDA.Environment,
		"instrument", oanda.DefaultInstrument,
		"email", cfg.Email.Enabled(),
		"sentiment", cfg.SentimentEnabled(),
		"strategy", cfg.Strategy.Enabled,
		"halt_file", cfg.Risk.HaltFile,
	)

	for {
		select {
		case <-ctx.Done():
			saveState()
			notifier.Send(ctx, "fxtrade: daemon stopping", "graceful shutdown")
			slog.Info("shutting down")
			return
		case tick := <-ticks:
			healthSrv.RecordTick(tick.Instrument, tick.Time)
			if rm.IsHalted() {
				slog.Warn("kill switch active — stream only, no new trades", "file", cfg.Risk.HaltFile)
				continue
			}
			slog.Debug("tick",
				"instrument", tick.Instrument,
				"bid", tick.Bid,
				"ask", tick.Ask,
				"spread_pips", fmt.Sprintf("%.1f", oanda.SpreadPips(tick.Spread)),
				"tradeable", tick.Tradeable,
			)
		}
	}
}

func runStartupChecks(ctx context.Context, client *oanda.Client, notifier notify.Notifier) error {
	summary, err := client.AccountSummary(ctx)
	if err != nil {
		return err
	}
	slog.Info("account connected",
		"id", summary.Account.ID,
		"balance", summary.Account.Balance,
		"nav", summary.Account.NAV,
	)

	pricing, err := client.Pricing(ctx, oanda.DefaultInstrument)
	if err != nil {
		return err
	}
	if len(pricing.Prices) > 0 {
		tick, err := pricing.Prices[0].ToUpdate()
		if err != nil {
			return err
		}
		slog.Info("pricing snapshot",
			"instrument", tick.Instrument,
			"bid", tick.Bid,
			"ask", tick.Ask,
			"spread_pips", fmt.Sprintf("%.1f", oanda.SpreadPips(tick.Spread)),
		)
	}

	candles, err := client.Candles(ctx, oanda.DefaultInstrument, "H1", 5)
	if err != nil {
		return err
	}
	slog.Info("candles fetched", "count", len(candles.Candles), "granularity", candles.Granularity)

	notifier.Send(ctx, "fxtrade: daemon started",
		fmt.Sprintf("environment=practice\ninstrument=%s\n", oanda.DefaultInstrument))

	return nil
}
