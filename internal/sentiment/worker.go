package sentiment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/schedule"
)

type Worker struct {
	cfg         config.SentimentConfig
	instruments []string
	fetcher     *Fetcher
	llm         *LLMClient
	oanda       *oanda.Client
	caches      map[string]*Cache
	notifier    notify.Notifier
}

func NewWorker(cfg *config.Config, oandaClient *oanda.Client, notifier notify.Notifier) (*Worker, error) {
	if !cfg.SentimentEnabled() {
		return nil, fmt.Errorf("sentiment pipeline requires finnhub.api_key and llm.api_key in .credentials")
	}
	instruments := cfg.Instruments
	if len(instruments) == 0 {
		instruments = []string{oanda.DefaultInstrument}
	}
	caches := make(map[string]*Cache, len(instruments))
	for _, inst := range instruments {
		caches[inst] = NewCache()
	}
	return &Worker{
		cfg:         cfg.Sentiment,
		instruments: instruments,
		fetcher:     NewFetcher(cfg.Finnhub),
		llm:         NewLLMClient(cfg.LLM),
		oanda:       oandaClient,
		caches:      caches,
		notifier:    notifier,
	}, nil
}

// Cache returns the cache for the first (primary) instrument.
func (w *Worker) Cache() *Cache {
	return w.caches[w.instruments[0]]
}

// CacheFor returns the cache for a specific instrument (nil if not configured).
func (w *Worker) CacheFor(instrument string) *Cache {
	return w.caches[instrument]
}

func (w *Worker) Instruments() []string {
	return w.instruments
}

func (w *Worker) Run(ctx context.Context) {
	if w.cfg.IntervalMinutes < config.MinSentimentIntervalMinutes {
		slog.Warn("sentiment interval below minimum; using floor",
			"configured_minutes", w.cfg.IntervalMinutes,
			"floor_minutes", config.MinSentimentIntervalMinutes,
		)
	}
	interval := time.Duration(w.effectiveIntervalMinutes()) * time.Minute
	schedule.RunPeriodic(ctx, "sentiment worker", interval, func(cycleCtx context.Context) {
		if _, err := w.runOnce(cycleCtx); err != nil {
			slog.Warn("sentiment cycle finished with errors", "error", err)
		}
	})
}

// effectiveIntervalMinutes is the actual cycle cadence after applying the floor.
func (w *Worker) effectiveIntervalMinutes() int {
	if w.cfg.IntervalMinutes < config.MinSentimentIntervalMinutes {
		return config.MinSentimentIntervalMinutes
	}
	return w.cfg.IntervalMinutes
}

// RunOnce runs one full cycle over all instruments and returns the primary
// instrument's signal (for test harnesses).
func (w *Worker) RunOnce(ctx context.Context) (SentimentSignal, error) {
	return w.runOnce(ctx)
}

func (w *Worker) runOnce(ctx context.Context) (SentimentSignal, error) {
	start := time.Now()
	slog.Info("sentiment cycle starting", "instruments", w.instruments)

	maxAge := time.Duration(w.cfg.HeadlineMaxAgeHours) * time.Hour
	raw, err := w.fetcher.FetchAll(ctx, maxAge)
	if err != nil {
		w.auditError(start, "", err)
		return SentimentSignal{}, err
	}

	norm := Normalize(raw, w.cfg)
	slog.Info("sentiment fetch complete",
		"headlines", len(norm.Headlines),
		"events", len(norm.Events),
		"high_event_risk", norm.HighEventRisk,
	)

	var primary SentimentSignal
	var firstErr error
	for i, instrument := range w.instruments {
		signal, err := w.analyzeInstrument(ctx, instrument, norm, start)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if i == 0 {
			primary = signal
		}
	}
	if firstErr != nil && primary.Direction == "" {
		return SentimentSignal{}, firstErr
	}
	return primary, nil
}

func (w *Worker) analyzeInstrument(ctx context.Context, instrument string, norm Normalized, start time.Time) (SentimentSignal, error) {
	price, err := market.BuildPriceContext(ctx, w.oanda, instrument, start)
	if err != nil {
		w.auditError(start, instrument, err)
		return SentimentSignal{}, err
	}

	payload := BuildPayload(instrument, norm, price, start)
	payloadJSON, err := payload.JSON()
	if err != nil {
		w.auditError(start, instrument, err)
		return SentimentSignal{}, err
	}

	signal, err := w.llm.Analyze(ctx, SystemPromptFor(instrument), payloadJSON)
	if err != nil {
		w.auditError(start, instrument, err)
		return SentimentSignal{}, err
	}
	signal.Instrument = instrument

	// Keep a signal valid until the next scheduled cycle so the strategy gate
	// has continuous coverage; otherwise a short valid_minutes (default 30)
	// leaves a gap when the cycle interval is longer.
	if iv := w.effectiveIntervalMinutes(); signal.ValidMinutes < iv {
		signal.ValidMinutes = iv
	}

	if cache := w.caches[instrument]; cache != nil {
		cache.Set(signal)
	}
	_ = AppendAudit(w.cfg.AuditDir, AuditRecord{
		At:         start.UTC(),
		Instrument: instrument,
		Payload:    payload,
		Response:   signal,
	})

	signalJSON, _ := json.Marshal(signal)
	slog.Info("sentiment signal", "instrument", instrument, "json", string(signalJSON))

	subject := fmt.Sprintf("fxtrade: %s sentiment %s (%.0f%%)", instrument, signal.Direction, signal.Confidence*100)
	body := FormatEmailBody(signal, w.cfg.IntervalMinutes)
	notify.SendDigest(w.notifier, ctx, "sentiment:"+instrument, subject, body)

	return signal, nil
}

func (w *Worker) auditError(at time.Time, instrument string, err error) {
	slog.Error("sentiment cycle failed", "instrument", instrument, "error", err)
	_ = AppendAudit(w.cfg.AuditDir, AuditRecord{
		At:         at.UTC(),
		Instrument: instrument,
		Error:      err.Error(),
	})
	label := instrument
	if label == "" {
		label = "all instruments"
	}
	notify.SendRoutine(w.notifier, context.Background(), "fxtrade: sentiment failed",
		fmt.Sprintf("Sentiment cycle failed for %s at %s\n\nError: %v\n\nThe daemon will retry on the next scheduled cycle.\n", label, at.Format(time.RFC3339), err))
}
