package btc_cfd

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
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/state"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

var Meta = bot.Meta{
	ID:          config.BotBtcCfd,
	Name:        "BTC CFD Mean Reversion",
	Description: "OANDA BTC_USD M5 mean reversion with ATR-based risk (demo first)",
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
	bc := cfg.BtcCfd

	rm := risk.NewManagerForEnv(btcRisk(cfg), cfg.OANDA.Environment)
	stateStore := state.NewStore(config.StateFileForBot(cfg.State.File, Meta.ID))
	persisted, err := stateStore.Load()
	if err != nil {
		slog.Warn("state load failed", "bot", Meta.ID, "error", err)
	}
	if !persisted.SavedAt.IsZero() {
		rm.RestoreState(persisted.Risk)
		slog.Info("state restored", "bot", Meta.ID, "saved_at", persisted.SavedAt)
	}

	tradeJournal := journal.New(bc.JournalDir)
	tradeRecorder := state.NewTradeRecorder(stateStore, tradeJournal)
	metaStore := newTradeMetaStore()

	tradeDB, err := sqlite.Open(bc.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open trade db: %w", err)
	}

	exec := execution.NewExecutor(client, rm, notifier)
	exec.SetDryRun(deps.DryRun)
	posMon := monitor.New(client, rm, notifier, 30*time.Second, bc.Instrument)
	exec.SetTradeAccounting(posMon)
	posMon.SetOnTradeClosed(func(tradeID, correlationID string, pl float64) {
		tradeRecorder.OnClose(tradeID, correlationID, pl, rm, nil)

		meta, hasMeta := metaStore.Take(tradeID)
		if correlationID == "" && hasMeta {
			correlationID = meta.CorrelationID
		}

		swapCost := lookupSwapCost(context.Background(), client, tradeID, meta.OpenedAt)
		netPL := pl - swapCost
		if swapCost != 0 {
			slog.Info("btc_cfd swap cost", "trade_id", tradeID, "swap", swapCost, "gross_pl", pl, "net_pl", netPL)
		}

		row := sqlite.Trade{
			ClosedAt:      time.Now().UTC(),
			Instrument:    bc.Instrument,
			TradeID:       tradeID,
			CorrelationID: correlationID,
			RealizedPL:    pl,
			SwapCost:      swapCost,
			NetPL:         netPL,
		}
		if hasMeta {
			row.Direction = meta.Direction
			row.SignalPrice = meta.SignalPrice
			row.FillPrice = meta.FillPrice
			row.StopLoss = meta.StopLoss
			row.TakeProfit = meta.TakeProfit
			row.Units = meta.Units
			row.SlippagePct = slippagePct(meta.SignalPrice, meta.FillPrice)
		}
		if err := tradeDB.InsertTrade(row); err != nil {
			slog.Warn("sqlite insert trade failed", "bot", Meta.ID, "error", err)
		}
	})

	instrument := bc.Instrument
	startedAt := time.Now()
	var running bool
	var lastCycle string
	var lastCycleOKAt time.Time
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
	engine := &cycleEngine{
		client:    client,
		bc:        bc,
		exec:      exec,
		rm:        rm,
		notifier:  notifier,
		posMon:    posMon,
		meta:      metaStore,
		lastCycle: &lastCycle,
		mu:        &mu,
		markCycleOK: func() {
			mu.Lock()
			lastCycleOKAt = time.Now()
			mu.Unlock()
		},
	}

	go func() {
		mu.Lock()
		running = true
		mu.Unlock()
		runCycle(ctx, engine)
		mu.Lock()
		running = false
		mu.Unlock()
	}()

	detail := fmt.Sprintf("%s %s mean reversion", instrument, bc.Granularity)

	halt := func() error {
		if err := rm.ActivateKillSwitch(); err != nil {
			return err
		}
		notify.SendRoutine(notifier, context.Background(), "fxtrade: kill switch activated",
			fmt.Sprintf("bot=%s\nhalt_file=%s\n", Meta.ID, rm.HaltFile()))
		return nil
	}

	return &bot.Handle{
		Meta:        Meta,
		Instruments: []string{instrument},
		Status: func() bot.Status {
			mu.RLock()
			run := running
			cycle := lastCycle
			cycleOK := lastCycleOKAt
			mu.RUnlock()
			d := detail
			if cycle != "" {
				d = cycle
			}
			return bot.Status{
				Meta:          Meta,
				Running:       run,
				Halted:        rm.IsHalted(),
				OpenPositions: posMon.OpenCount(),
				Detail:        d,
				StartedAt:     startedAt,
				LastCycleOKAt: cycleOK,
			}
		},
		Halt:          halt,
		OpenPositions: posMon.OpenCount,
		SaveState:     saveState,
	}, nil
}

func lookupSwapCost(ctx context.Context, client *oanda.Client, tradeID string, since time.Time) float64 {
	if since.IsZero() {
		since = time.Now().Add(-7 * 24 * time.Hour)
	}
	txs, err := client.TransactionsSinceAll(ctx, since)
	if err != nil {
		return 0
	}
	cost := oanda.FinancingCost(txs, tradeID)
	if cost < 0 {
		return -cost
	}
	return cost
}

func btcRisk(cfg *config.Config) config.RiskConfig {
	rc := cfg.Risk
	bc := cfg.BtcCfd
	rc.MaxDailyLossPct = bc.MaxDailyLossPct
	rc.MaxTradesPerMonth = 0
	rc.MaxOpenPositions = 1
	rc.RiskPerTradePctBase = bc.PerTradeRiskPct
	rc.RiskPerTradePctHighConf = bc.PerTradeRiskPct
	rc.HaltFile = config.HaltFileForBot(rc.HaltFile, Meta.ID)
	if rc.MaxDailyLossPct == 0 {
		rc.MaxDailyLossPct = 2.5
	}
	if rc.RiskPerTradePctBase == 0 {
		rc.RiskPerTradePctBase = 0.5
	}
	// Spread gate uses MaxSpreadUSD in strategy; avoid FX pip cap blocking BTC.
	rc.MaxSpreadPips = 99999
	return rc
}
