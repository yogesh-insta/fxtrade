package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	persisted, err := stateStore.Load()
	if err != nil {
		slog.Warn("state load failed", "error", err)
	}

	tradeJournal := journal.New(cfg.Strategy.JournalDir)
	tradeRecorder := state.NewTradeRecorder(stateStore, tradeJournal)

	instruments := cfg.Instruments
	primaryInstrument := instruments[0]

	var sentimentWorker *sentiment.Worker
	sentimentCaches := make(map[string]*sentiment.Cache, len(instruments))
	for _, inst := range instruments {
		sentimentCaches[inst] = sentiment.NewCache()
	}

	exec := execution.NewExecutor(client, rm, notifier)
	posMon := monitor.New(client, rm, notifier, 30*time.Second)
	exec.SetTradeAccounting(posMon)
	posMon.SetOnTradeClosed(func(tradeID, correlationID string, pl float64) {
		tradeRecorder.OnClose(tradeID, correlationID, pl, rm, sentimentCaches)
	})
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
		for _, inst := range instruments {
			if c := sw.CacheFor(inst); c != nil {
				sentimentCaches[inst] = c
			}
		}
		healthSrv.SetSentimentStatus(func() (bool, string, float64, time.Time) {
			// Combined view across instruments; confidence/time from primary.
			now := time.Now()
			var parts []string
			var primaryConf float64
			var primaryAt time.Time
			for _, inst := range instruments {
				sig, ok := sentimentCaches[inst].Current(now)
				if !ok {
					continue
				}
				parts = append(parts, fmt.Sprintf("%s %s %.0f%%", inst, sig.Direction, sig.Confidence*100))
				if inst == primaryInstrument {
					primaryConf = sig.Confidence
					primaryAt = sig.AnalyzedAt
				}
			}
			return true, strings.Join(parts, " | "), primaryConf, primaryAt
		})
	} else {
		slog.Warn("sentiment pipeline disabled", "hint", "add finnhub.api_key and llm.api_key to .credentials")
	}

	if !persisted.SavedAt.IsZero() {
		state.ApplySnapshot(persisted, rm, sentimentCaches, primaryInstrument)
		slog.Info("state restored", "saved_at", persisted.SavedAt, "trades_this_month", persisted.Risk.TradesThisMonth)
	}

	engines := make([]*strategy.Engine, 0, len(instruments))
	for _, inst := range instruments {
		engines = append(engines, strategy.NewEngine(cfg, inst, client, exec, rm, notifier, sentimentCaches[inst]))
	}
	posMon.SetOnTradeOpened(func(t oanda.Trade) {
		for _, eng := range engines {
			if eng.Instrument() == t.Instrument {
				eng.CancelAllPendingNow(context.Background(), "position_opened")
			}
		}
	})
	healthSrv.SetTradingMode(func() string {
		var parts []string
		for _, e := range engines {
			mode := e.LastMode()
			if mode == "" {
				mode = "-"
			}
			parts = append(parts, fmt.Sprintf("%s:%s", e.Instrument(), mode))
		}
		return strings.Join(parts, " ")
	})

	saveState := func() {
		snap := state.BuildSnapshot(rm, sentimentCaches, tradeRecorder.LastTrade())
		if err := stateStore.Save(snap); err != nil {
			slog.Warn("state save failed", "error", err)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := runStartupChecks(ctx, client, notifier, cfg); err != nil {
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
		if err := stream.RunPricingStream(ctx, instruments, ticks); err != nil && ctx.Err() == nil {
			slog.Error("pricing stream exited", "error", err)
		}
	}()

	go posMon.Run(ctx)

	if sentimentWorker != nil {
		go sentimentWorker.Run(ctx)
	}

	if cfg.Strategy.Enabled {
		go strategy.RunAll(ctx, engines, cfg)
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
		"instruments", instruments,
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

func runStartupChecks(ctx context.Context, client *oanda.Client, notifier notify.Notifier, cfg *config.Config) error {
	summary, err := client.AccountSummary(ctx)
	if err != nil {
		return err
	}
	slog.Info("account connected",
		"id", summary.Account.ID,
		"balance", summary.Account.Balance,
		"nav", summary.Account.NAV,
	)

	for _, inst := range cfg.Instruments {
		pricing, err := client.Pricing(ctx, inst)
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

		candles, err := client.Candles(ctx, inst, "H1", 5)
		if err != nil {
			return err
		}
		slog.Info("candles fetched", "instrument", inst, "count", len(candles.Candles), "granularity", candles.Granularity)
	}

	notifier.Send(ctx, "fxtrade: daemon started",
		fmt.Sprintf("fxtrade daemon is running.\n\nEnvironment: %s\nInstruments: %s\nStrategy cycle: every %d min\nSentiment cycle: every %d min\n",
			cfg.OANDA.Environment, strings.Join(cfg.Instruments, ", "), cfg.Strategy.CycleMinutes, cfg.Sentiment.IntervalMinutes))

	return nil
}
