// Package stockscan implements the NiftyPulse NSE daily swing scanner (cmd/nifty-pulse).
package stockscan

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

// Result holds one symbol's scan outcome.
type Result struct {
	Symbol string
	Bar    Bar
	Candidate Candidate
	Error  error
	Skipped bool
	Reason  string
}

// Scanner fetches Yahoo data and applies technical filters.
type Scanner struct {
	yahoo     *YahooClient
	cfg       config.StockScanConfig
	smaPeriod int
	rsiPeriod int
}

func NewScanner(cfg config.StockScanConfig) *Scanner {
	timeout := time.Duration(cfg.RequestTimeoutSec) * time.Second
	rateLimit := time.Duration(cfg.RateLimitMS) * time.Millisecond
	return &Scanner{
		yahoo:     NewYahooClient(timeout, rateLimit),
		cfg:       cfg,
		smaPeriod: cfg.SMAPeriod,
		rsiPeriod: cfg.RSIPeriod,
	}
}

// LoadWatchlist reads one NSE symbol per line from path.
func LoadWatchlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open watchlist %s: %w", path, err)
	}
	defer f.Close()

	var symbols []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		symbols = append(symbols, strings.ToUpper(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read watchlist %s: %w", path, err)
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("watchlist %s is empty", path)
	}
	return symbols, nil
}

// ScanAll fetches and evaluates all symbols concurrently.
func (s *Scanner) ScanAll(ctx context.Context, symbols []string) []Result {
	concurrency := s.cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	results := make([]Result, len(symbols))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, sym := range symbols {
		wg.Add(1)
		go func(idx int, symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = s.scanOne(ctx, symbol)
		}(i, sym)
	}
	wg.Wait()
	return results
}

func (s *Scanner) scanOne(ctx context.Context, symbol string) Result {
	res := Result{Symbol: symbol}

	bars, err := s.yahoo.FetchDaily(ctx, symbol)
	if err != nil {
		res.Error = err
		slog.Warn("yahoo fetch failed", "symbol", symbol, "error", err)
		return res
	}

	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}

	sma, ok := SMA(closes, s.smaPeriod)
	if !ok {
		res.Skipped = true
		res.Reason = fmt.Sprintf("insufficient data for SMA(%d)", s.smaPeriod)
		return res
	}
	rsi, ok := RSI(closes, s.rsiPeriod)
	if !ok {
		res.Skipped = true
		res.Reason = fmt.Sprintf("insufficient data for RSI(%d)", s.rsiPeriod)
		return res
	}

	last := bars[len(bars)-1]
	res.Bar = last
	res.Candidate = Candidate{
		Symbol: symbol,
		Close:  last.Close,
		SMA:    sma,
		RSI:    rsi,
	}

	if !PassesFilter(last.Close, sma, rsi, s.cfg.RSIMin, s.cfg.RSIMax) {
		res.Skipped = true
		res.Reason = fmt.Sprintf("filter miss close=%.2f sma=%.2f rsi=%.2f", last.Close, sma, rsi)
	}
	return res
}

// CollectCandidates extracts passing candidates from scan results.
func CollectCandidates(results []Result) []Candidate {
	out := make([]Candidate, 0)
	for _, r := range results {
		if r.Error != nil || r.Skipped {
			continue
		}
		out = append(out, r.Candidate)
	}
	return out
}

// Run performs the full scan pipeline up to ranking (no sentiment or notify).
func (s *Scanner) Run(ctx context.Context, watchlistPath string) ([]Candidate, []Result, error) {
	symbols, err := LoadWatchlist(watchlistPath)
	if err != nil {
		return nil, nil, err
	}
	slog.Info("stock scan started", "symbols", len(symbols), "watchlist", watchlistPath)

	results := s.ScanAll(ctx, symbols)
	candidates := CollectCandidates(results)
	ranked := RankByRSI(candidates)

	slog.Info("stock scan complete",
		"scanned", len(symbols),
		"passed", len(candidates),
		"errors", countErrors(results),
	)
	return ranked, results, nil
}

func countErrors(results []Result) int {
	n := 0
	for _, r := range results {
		if r.Error != nil {
			n++
		}
	}
	return n
}
