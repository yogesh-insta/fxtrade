package fx_sentiment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/bot"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/execution"
	"github.com/ym/fxtrade/internal/journal"
	"github.com/ym/fxtrade/internal/monitor"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
	"github.com/ym/fxtrade/internal/sentiment"
	"github.com/ym/fxtrade/internal/state"
	"github.com/ym/fxtrade/internal/store"
	"github.com/ym/fxtrade/internal/store/sqlite"
	"github.com/ym/fxtrade/internal/strategy"
)

var Meta = bot.Meta{
	ID:          config.BotFxSentiment,
	Name:        "FX Sentiment Strategy",
	Description: "FX range/trend strategy with Finnhub headlines and Groq sentiment gate",
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

	rc := cfg.Risk
	rc.HaltFile = config.HaltFileForBot(rc.HaltFile, Meta.ID)
	rm := risk.NewManagerForEnv(rc, cfg.OANDA.Environment)

	stateStore := state.NewStore(config.StateFileForBot(cfg.State.File, Meta.ID))
	persisted, err := stateStore.Load()
	if err != nil {
		slog.Warn("state load failed", "bot", Meta.ID, "error", err)
	}

	instruments := cfg.Instruments
	if len(instruments) == 0 {
		instruments = []string{"AUD_USD", "EUR_USD"}
	}
	primaryInstrument := instruments[0]

	tradeJournal := journal.New(cfg.Strategy.JournalDir)
	tradeRecorder := state.NewTradeRecorder(stateStore, tradeJournal)
	metaStore := store.NewMetaStore()

	tradeDB, err := sqlite.Open(cfg.Strategy.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open trade db: %w", err)
	}

	sentimentCaches := make(map[string]*sentiment.Cache, len(instruments))
	for _, inst := range instruments {
		sentimentCaches[inst] = sentiment.NewCache()
	}

	var sentimentWorker *sentiment.Worker
	if cfg.SentimentEnabled() {
		sw, err := sentiment.NewWorker(cfg, client, notifier)
		if err != nil {
			return nil, fmt.Errorf("sentiment worker: %w", err)
		}
		sentimentWorker = sw
		for _, inst := range instruments {
			if c := sw.CacheFor(inst); c != nil {
				sentimentCaches[inst] = c
			}
		}
	}

	if !persisted.SavedAt.IsZero() {
		state.ApplySnapshot(persisted, rm, sentimentCaches, primaryInstrument)
		slog.Info("state restored", "bot", Meta.ID, "saved_at", persisted.SavedAt)
	}

	exec := execution.NewExecutor(client, rm, notifier)
	exec.SetDryRun(deps.DryRun)
	posMon := monitor.New(client, rm, notifier, 30*time.Second, instruments...)
	posMon.SetOwnerTag(Meta.ID)
	exec.SetTradeAccounting(posMon)

	engines := make([]*strategy.Engine, 0, len(instruments))
	for _, inst := range instruments {
		eng := strategy.NewEngine(cfg, inst, client, exec, rm, notifier, sentimentCaches[inst])
		eng.SetPerformanceStore(Meta.ID, tradeDB)
		engines = append(engines, eng)
	}

	posMon.SetOnTradeClosed(func(tradeID, correlationID string, pl float64) {
		tradeRecorder.OnClose(tradeID, correlationID, pl, rm, sentimentCaches)

		meta, hasMeta := metaStore.Take(tradeID)
		row := store.ClosedTradeRow(meta.Instrument, tradeID, correlationID, pl, meta, hasMeta)
		if err := tradeDB.InsertTrade(row); err != nil {
			slog.Warn("sqlite insert trade failed", "bot", Meta.ID, "error", err)
		}
	})
	posMon.SetOnTradeOpened(func(t oanda.Trade) {
		metaStore.Put(t.ID, store.MetaFromTrade(t))
		for _, eng := range engines {
			if eng.Instrument() == t.Instrument {
				eng.CancelAllPendingNow(context.Background(), "position_opened")
			}
		}
	})

	startedAt := time.Now()
	var running bool
	var lastCycleOKAt time.Time
	var mu sync.RWMutex

	saveState := func() error {
		snap := state.BuildSnapshot(rm, sentimentCaches, tradeRecorder.LastTrade())
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
	if sentimentWorker != nil {
		go sentimentWorker.Run(ctx)
	}
	if cfg.Strategy.Enabled {
		go func() {
			mu.Lock()
			running = true
			mu.Unlock()
			strategy.RunAll(ctx, engines, cfg, func() {
				mu.Lock()
				lastCycleOKAt = time.Now()
				mu.Unlock()
			})
			mu.Lock()
			running = false
			mu.Unlock()
		}()
	}

	tradingDetail := func() string {
		var parts []string
		for _, e := range engines {
			mode := e.LastMode()
			if mode == "" {
				mode = "-"
			}
			parts = append(parts, fmt.Sprintf("%s:%s", e.Instrument(), mode))
		}
		return strings.Join(parts, " ")
	}

	return &bot.Handle{
		Meta:        Meta,
		Instruments: instruments,
		Status: func() bot.Status {
			mu.RLock()
			run := running
			cycleOK := lastCycleOKAt
			mu.RUnlock()
			return bot.Status{
				Meta:          Meta,
				Running:       run,
				Halted:        rm.IsHalted(),
				OpenPositions: posMon.OpenCount(),
				Detail:        tradingDetail(),
				StartedAt:     startedAt,
				LastCycleOKAt: cycleOK,
			}
		},
		Halt:          rm.ActivateKillSwitch,
		OpenPositions: posMon.OpenCount,
		SaveState:     saveState,
	}, nil
}
