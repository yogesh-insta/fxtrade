package scanner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/oanda"
)

type Scanner struct {
	cfg    *config.Config
	client *oanda.Client
	meta   map[string]oanda.Instrument
}

func NewScanner(cfg *config.Config, client *oanda.Client, universe Universe) *Scanner {
	return &Scanner{
		cfg:    cfg,
		client: client,
		meta:   universe.Meta,
	}
}

func (s *Scanner) ScanAll(ctx context.Context, symbols []string) []Setup {
	concurrency := s.cfg.Scanner.ScanConcurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	sem := make(chan struct{}, concurrency)
	results := make([]Setup, len(symbols))
	var wg sync.WaitGroup

	for i, sym := range symbols {
		wg.Add(1)
		go func(idx int, instrument string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = s.scanOne(ctx, instrument)
		}(i, sym)
	}
	wg.Wait()
	return results
}

func (s *Scanner) scanOne(ctx context.Context, instrument string) Setup {
	meta, ok := s.meta[instrument]
	if !ok {
		return Setup{Instrument: instrument, SkipReason: "missing instrument metadata"}
	}
	cls := assetClass(s.cfg.Scanner, instrument, meta.Type)
	now := time.Now().UTC()

	setup := Setup{
		Instrument: instrument,
		Class:      cls,
		InSession:  InSession(s.cfg.Scanner, instrument, meta.Type, now),
	}
	if !setup.InSession {
		setup.SkipReason = "outside session"
		return setup
	}

	pricing, err := s.client.Pricing(ctx, instrument)
	if err != nil {
		setup.SkipReason = fmt.Sprintf("pricing: %v", err)
		return setup
	}
	if len(pricing.Prices) == 0 {
		setup.SkipReason = "no pricing"
		return setup
	}
	tick, err := pricing.Prices[0].ToUpdate()
	if err != nil {
		setup.SkipReason = fmt.Sprintf("tick: %v", err)
		return setup
	}
	if !tick.Tradeable {
		setup.SkipReason = "not tradeable"
		return setup
	}

	pipSize := oanda.InstrumentPipSize(instrument, meta.PipLocation)
	setup.SpreadPips = oanda.SpreadPipsFor(instrument, meta.PipLocation, tick.Spread)

	maxSpread := maxSpreadForClass(s.cfg.Scanner, cls)
	sessionOpen := SessionOpen(s.cfg.Scanner, instrument, meta.Type, now)
	if sessionOpen.IsZero() {
		setup.SkipReason = "invalid session"
		return setup
	}

	m15, err := s.client.Candles(ctx, instrument, s.cfg.Scanner.OpeningRangeGranularity, 32)
	if err != nil {
		setup.SkipReason = fmt.Sprintf("M15 candles: %v", err)
		return setup
	}
	gr, ok, reason := BuildOpeningRange(m15.Candles, sessionOpen, s.cfg.Scanner.OpeningRangeCandles, pipSize)
	if !ok {
		setup.SkipReason = reason
		return setup
	}
	setup.Range = gr

	h1, err := s.client.Candles(ctx, instrument, "H1", 6)
	if err != nil {
		setup.SkipReason = fmt.Sprintf("H1 candles: %v", err)
		return setup
	}
	setup.TrendBias = H1TrendBias(h1.Candles)
	setup.RangeSpreadRatio = gr.RangePips / setup.SpreadPips

	score, ok, reason := ScoreSetup(gr.RangePips, setup.SpreadPips, maxSpread, s.cfg.Scanner.MinRangeSpreadRatio, setup.TrendBias)
	if !ok {
		setup.SkipReason = reason
		return setup
	}
	setup.Score = score
	setup.Ready = true
	setup.BreakoutDirection = DetectBreakout(setup, tick.Bid, tick.Ask)
	return setup
}

func maxSpreadForClass(cfg config.ScannerConfig, cls string) float64 {
	if ac, ok := cfg.AssetClasses[cls]; ok {
		switch cls {
		case "CRYPTO":
			if ac.MaxSpreadUSD > 0 {
				return ac.MaxSpreadUSD
			}
		case "INDEX", "ENERGY":
			if ac.MaxSpreadPoints > 0 {
				return ac.MaxSpreadPoints
			}
		default:
			if ac.MaxSpreadPips > 0 {
				return ac.MaxSpreadPips
			}
		}
	}
	switch cls {
	case "CRYPTO":
		return cfg.UniverseFilters.MaxSpreadPipsFX * 20
	case "INDEX", "ENERGY":
		return cfg.UniverseFilters.MaxSpreadPointsIndex
	default:
		return cfg.UniverseFilters.MaxSpreadPipsFX
	}
}

func (s *Scanner) PricingMap(ctx context.Context, symbols []string) map[string]oanda.PriceUpdate {
	out := make(map[string]oanda.PriceUpdate, len(symbols))
	if len(symbols) == 0 {
		return out
	}
	pricing, err := s.client.Pricing(ctx, symbols...)
	if err != nil {
		slog.Warn("batch pricing failed", "error", err)
		return out
	}
	for _, p := range pricing.Prices {
		tick, err := p.ToUpdate()
		if err != nil {
			continue
		}
		out[tick.Instrument] = tick
	}
	return out
}
