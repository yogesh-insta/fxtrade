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
)

type Worker struct {
	cfg      config.SentimentConfig
	fetcher  *Fetcher
	llm      *LLMClient
	oanda    *oanda.Client
	cache    *Cache
	notifier notify.Notifier
}

func NewWorker(cfg *config.Config, oandaClient *oanda.Client, notifier notify.Notifier) (*Worker, error) {
	if !cfg.SentimentEnabled() {
		return nil, fmt.Errorf("sentiment pipeline requires finnhub.api_key and llm.api_key in .credentials")
	}
	return &Worker{
		cfg:      cfg.Sentiment,
		fetcher:  NewFetcher(cfg.Finnhub),
		llm:      NewLLMClient(cfg.LLM),
		oanda:    oandaClient,
		cache:    NewCache(),
		notifier: notifier,
	}, nil
}

func (w *Worker) Cache() *Cache {
	return w.cache
}

func (w *Worker) Run(ctx context.Context) {
	interval := time.Duration(w.cfg.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 30 * time.Minute
	}

	slog.Info("sentiment worker started", "interval", interval)
	w.runOnce(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("sentiment worker stopping")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (SentimentSignal, error) {
	return w.runOnce(ctx)
}

func (w *Worker) runOnce(ctx context.Context) (SentimentSignal, error) {
	start := time.Now()
	slog.Info("sentiment cycle starting")

	maxAge := time.Duration(w.cfg.HeadlineMaxAgeHours) * time.Hour
	raw, err := w.fetcher.FetchAll(ctx, maxAge)
	if err != nil {
		w.auditError(start, err)
		return SentimentSignal{}, err
	}

	norm := Normalize(raw, w.cfg)
	slog.Info("sentiment fetch complete",
		"headlines", len(norm.Headlines),
		"events", len(norm.Events),
		"high_event_risk", norm.HighEventRisk,
	)

	price, err := market.BuildPriceContext(ctx, w.oanda, oanda.DefaultInstrument, start)
	if err != nil {
		w.auditError(start, err)
		return SentimentSignal{}, err
	}

	payload := BuildPayload(norm, price, start)
	payloadJSON, err := payload.JSON()
	if err != nil {
		w.auditError(start, err)
		return SentimentSignal{}, err
	}

	signal, err := w.llm.Analyze(ctx, payloadJSON)
	if err != nil {
		w.auditError(start, err)
		return SentimentSignal{}, err
	}

	w.cache.Set(signal)
	_ = AppendAudit(w.cfg.AuditDir, AuditRecord{
		At:       start.UTC(),
		Payload:  payload,
		Response: signal,
	})

	signalJSON, _ := json.Marshal(signal)
	slog.Info("sentiment signal", "json", string(signalJSON))

	subject := fmt.Sprintf("fxtrade: sentiment %s conf=%.2f", signal.Direction, signal.Confidence)
	body := fmt.Sprintf("direction=%s\nconfidence=%.2f\naud_bias=%s\nevent_risk=%s\ndrivers=%v\nrisks=%v\n",
		signal.Direction, signal.Confidence, signal.AUDBias, signal.EventRisk, signal.Drivers, signal.Risks)
	w.notifier.Send(ctx, subject, body)

	return signal, nil
}

func (w *Worker) auditError(at time.Time, err error) {
	slog.Error("sentiment cycle failed", "error", err)
	_ = AppendAudit(w.cfg.AuditDir, AuditRecord{
		At:    at.UTC(),
		Error: err.Error(),
	})
}
