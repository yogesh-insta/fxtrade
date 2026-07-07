package universe_scanner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/monitor"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/scanner"
	"github.com/ym/fxtrade/internal/state"
	"github.com/ym/fxtrade/internal/store"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

var Meta = bot.Meta{
	ID:          config.BotUniverseScanner,
	Name:        "FXPulse ORB Scanner",
	Description: "OANDA opening-range breakout scanner; ranks universe daily, one trade at a time",
}

type Bot struct {
	deps *bot.Deps
}

func New(deps *bot.Deps) (bot.Bot, error) {
	return &Bot{deps: deps}, nil
}

func (b *Bot) Meta() bot.Meta { return Meta }

func (b *Bot) Start(ctx context.Context, deps *bot.Deps) (*bot.Handle, error) {
	cfg := deps.CFG
	client := deps.Client
	notifier := deps.Notifier

	rm := risk.NewManager(scannerRisk(cfg))
	stateStore := state.NewStore(config.StateFileForBot(cfg.State.File, Meta.ID))
	persisted, err := stateStore.Load()
	if err != nil {
		slog.Warn("state load failed", "bot", Meta.ID, "error", err)
	}
	if !persisted.SavedAt.IsZero() {
		rm.RestoreState(persisted.Risk)
		slog.Info("state restored", "bot", Meta.ID, "saved_at", persisted.SavedAt)
	}

	tradeJournal := journal.New(cfg.Scanner.JournalDir)
	tradeRecorder := state.NewTradeRecorder(stateStore, tradeJournal)
	metaStore := store.NewMetaStore()

	tradeDB, err := sqlite.Open(cfg.Scanner.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open trade db: %w", err)
	}

	universe, err := scanner.ResolveUniverse(ctx, cfg, client)
	if err != nil {
		return nil, fmt.Errorf("resolve universe: %w", err)
	}

	scannerNotifier := scanner.NewNotifier(cfg, notifier, deps.DryRun)
	exec := execution.NewExecutor(client, rm, notifier)
	exec.SetDryRun(deps.DryRun)
	posMon := monitor.New(client, rm, notifier, 30*time.Second, universe.Symbols...)
	exec.SetTradeAccounting(posMon)
	posMon.SetOnTradeOpened(func(t oanda.Trade) {
		metaStore.Put(t.ID, store.MetaFromTrade(t))
	})
	posMon.SetOnTradeClosed(func(tradeID, correlationID string, pl float64) {
		tradeRecorder.OnClose(tradeID, correlationID, pl, rm, nil)

		meta, hasMeta := metaStore.Take(tradeID)
		row := store.ClosedTradeRow(meta.Instrument, tradeID, correlationID, pl, meta, hasMeta)
		if err := tradeDB.InsertTrade(row); err != nil {
			slog.Warn("sqlite insert trade failed", "bot", Meta.ID, "error", err)
		}
	})

	engine := scanner.NewEngine(cfg, client, exec, rm, scannerNotifier, universe)
	engine.SetBotID(Meta.ID)
	engine.SetPerformanceStore(tradeDB, metaStore)

	startedAt := time.Now()
	var running bool
	var mu sync.RWMutex

	saveState := func() error {
		snap := state.BuildSnapshot(rm, nil, tradeRecorder.LastTrade())
		return stateStore.Save(snap)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := saveState(); err != nil {
					slog.Warn("state save failed", "bot", Meta.ID, "error", err)
				}
			}
		}
	}()

	go posMon.Run(ctx)
	go func() {
		mu.Lock()
		running = true
		mu.Unlock()
		engine.Run(ctx)
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	detail := fmt.Sprintf("%d symbols (%s)", len(universe.Symbols), cfg.Scanner.UniversePreset)

	return &bot.Handle{
		Meta:        Meta,
		Instruments: universe.Symbols,
		Status: func() bot.Status {
			mu.RLock()
			run := running
			mu.RUnlock()
			return bot.Status{
				Meta:          Meta,
				Running:       run,
				Halted:        rm.IsHalted(),
				OpenPositions: posMon.OpenCount(),
				Detail:        detail,
				StartedAt:     startedAt,
			}
		},
		Halt:          rm.ActivateKillSwitch,
		OpenPositions: posMon.OpenCount,
		SaveState:     saveState,
	}, nil
}

func scannerRisk(cfg *config.Config) config.RiskConfig {
	rc := cfg.Risk
	rc.MaxDailyLossPct = cfg.Scanner.DailyLossCapPct
	rc.MaxTradesPerMonth = 0
	rc.MaxOpenPositions = 1
	rc.RiskPerTradePctBase = cfg.Scanner.RiskPerTradePct
	rc.RiskPerTradePctHighConf = cfg.Scanner.RiskPerTradePct
	rc.HaltFile = config.HaltFileForBot(rc.HaltFile, Meta.ID)
	if rc.MaxDailyLossPct == 0 {
		rc.MaxDailyLossPct = 1.0
	}
	if rc.RiskPerTradePctBase == 0 {
		rc.RiskPerTradePctBase = 1.0
	}
	return rc
}
