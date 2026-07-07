package report

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/store/sqlite"
)

// ClosedTrade is a row from trades.db used for performance analysis.
type ClosedTrade struct {
	ClosedAt      time.Time
	Instrument    string
	Direction     string
	TradeID       string
	CorrelationID string
	NetPL         float64
	RealizedPL    float64
	SwapCost      float64
	SlippagePct   float64
	SetupScore    float64
}

// BucketStats aggregates P/L for a grouping key (instrument, exit reason, hour, etc.).
type BucketStats struct {
	Key        string
	Trades     int
	Wins       int
	Losses     int
	TotalNetPL float64
}

// BotAnalysis is a data-driven review of one bot's SQLite store.
type BotAnalysis struct {
	BotID      string
	DBPath     string
	Err        error
	AllTime    sqlite.PeriodMetrics
	Yesterday  sqlite.PeriodMetrics
	YesterdayDate time.Time
	Trades     []ClosedTrade
	ByInstrument []BucketStats
	ByExit     []BucketStats
	ByHourUTC  []BucketStats
	Suggestions []string
}

// AnalyzeBot reads trades.db and returns metrics plus config tweak suggestions.
func AnalyzeBot(cfg *config.Config, botID string, now time.Time) BotAnalysis {
	botID = config.NormalizeBotID(botID)
	out := BotAnalysis{
		BotID:  botID,
		DBPath: BotDBPath(cfg, botID),
	}

	store, err := sqlite.Open(out.DBPath)
	if err != nil {
		out.Err = err
		return out
	}
	defer store.Close()

	trades, err := loadClosedTrades(store)
	if err != nil {
		out.Err = err
		return out
	}
	out.Trades = trades

	allStart := time.Time{}
	allEnd := now.UTC().Add(24 * time.Hour)
	if pm, err := store.PeriodMetricsUTC(allStart, allEnd); err != nil {
		out.Err = err
		return out
	} else {
		out.AllTime = pm
	}

	yesterday := reportYesterdayUTC(now)
	out.YesterdayDate = yesterday
	yStart := yesterday
	yEnd := yesterday.Add(24 * time.Hour)
	if pm, err := store.PeriodMetricsUTC(yStart, yEnd); err != nil {
		out.Err = err
		return out
	} else {
		out.Yesterday = pm
	}

	out.ByInstrument = bucketTrades(trades, func(t ClosedTrade) string { return t.Instrument })
	out.ByExit = bucketTrades(trades, func(t ClosedTrade) string { return exitReason(t.CorrelationID) })
	out.ByHourUTC = bucketTrades(trades, func(t ClosedTrade) string {
		return fmt.Sprintf("%02d:00", t.ClosedAt.UTC().Hour())
	})
	out.Suggestions = suggestTweaks(cfg, botID, trades, out.AllTime, out.Yesterday, out.ByInstrument, out.ByExit, out.ByHourUTC)
	return out
}

func reportYesterdayUTC(now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return today.Add(-24 * time.Hour)
}

func loadClosedTrades(store *sqlite.Store) ([]ClosedTrade, error) {
	rows, err := store.ListClosedTrades()
	if err != nil {
		return nil, err
	}
	scores, _ := store.SignalScoresByCorrelation()
	out := make([]ClosedTrade, 0, len(rows))
	for _, r := range rows {
		ct := ClosedTrade{
			ClosedAt:      r.ClosedAt,
			Instrument:    r.Instrument,
			Direction:     r.Direction,
			TradeID:       r.TradeID,
			CorrelationID: r.CorrelationID,
			NetPL:         r.NetPL,
			RealizedPL:    r.RealizedPL,
			SwapCost:      r.SwapCost,
			SlippagePct:   r.SlippagePct,
		}
		if sc, ok := scores[r.CorrelationID]; ok {
			ct.SetupScore = sc
		}
		out = append(out, ct)
	}
	return out, nil
}

func bucketTrades(trades []ClosedTrade, keyFn func(ClosedTrade) string) []BucketStats {
	m := make(map[string]*BucketStats)
	for _, t := range trades {
		k := keyFn(t)
		if k == "" {
			k = "(unknown)"
		}
		b, ok := m[k]
		if !ok {
			b = &BucketStats{Key: k}
			m[k] = b
		}
		b.Trades++
		b.TotalNetPL += t.NetPL
		if t.NetPL > 0 {
			b.Wins++
		} else if t.NetPL < 0 {
			b.Losses++
		}
	}
	out := make([]BucketStats, 0, len(m))
	for _, b := range m {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalNetPL != out[j].TotalNetPL {
			return out[i].TotalNetPL < out[j].TotalNetPL
		}
		return out[i].Trades > out[j].Trades
	})
	return out
}

func exitReason(correlationID string) string {
	c := strings.ToLower(correlationID)
	switch {
	case strings.Contains(c, "force_flat"):
		return "force_flat"
	case strings.Contains(c, "tp1-"):
		return "tp1_partial"
	case strings.Contains(c, "max-hold"):
		return "max_hold"
	case strings.Contains(c, "scan-"):
		return "scanner_breakout"
	case strings.Contains(c, "range-buy"), strings.Contains(c, "range-sell"):
		return "range_limit"
	case strings.Contains(c, "bc-"):
		return "btc_signal"
	default:
		if correlationID == "" {
			return "external_sl_tp"
		}
		return "other"
	}
}

func suggestTweaks(
	cfg *config.Config,
	botID string,
	trades []ClosedTrade,
	all, yesterday sqlite.PeriodMetrics,
	byInst, byExit, byHour []BucketStats,
) []string {
	if len(trades) == 0 {
		return []string{"No closed trades in database yet — run bots live or check db_path."}
	}

	var out []string

	// Yesterday all-red day
	if yesterday.TradeCount > 0 && yesterday.WinCount == 0 {
		out = append(out, fmt.Sprintf(
			"Yesterday (%s): %d trades, all losses (net %s). Pause new entries until filters are tightened.",
			yesterday.Start.Format("2006-01-02"), yesterday.TradeCount, formatMoney(yesterday.TotalNetPL),
		))
	}

	if all.TradeCount >= 5 && all.WinRate < 0.4 {
		out = append(out, fmt.Sprintf(
			"Win rate %.0f%% over %d trades is below 40%% — tighten entry filters before increasing size.",
			all.WinRate*100, all.TradeCount,
		))
	}

	if all.RealizedRR > 0 && all.RealizedRR < 1.0 && all.WinRate < 0.55 {
		out = append(out, fmt.Sprintf(
			"Realized R:R %.2f with %.0f%% win rate — raise take-profit R:R or reduce stop distance.",
			all.RealizedRR, all.WinRate*100,
		))
	}

	// Force-flat analysis
	if ff := findBucket(byExit, "force_flat"); ff != nil && ff.Trades >= 2 {
		ffWR := float64(ff.Wins) / float64(ff.Trades)
		if ff.TotalNetPL < 0 && ffWR < 0.35 {
			out = append(out, fmt.Sprintf(
				"%d force-flat closes net %s (%.0f%% win) — block new entries ~2h before force_flat_utc, or only force-flat losers.",
				ff.Trades, formatMoney(ff.TotalNetPL), ffWR*100,
			))
		}
	}

	// Late-hour losses (scanner force-flat window)
	lateLosses, lateTrades := 0, 0
	for _, b := range byHour {
		h := 0
		fmt.Sscanf(b.Key, "%d:", &h)
		if h >= 19 && h <= 21 {
			lateTrades += b.Trades
			if b.TotalNetPL < 0 {
				lateLosses += b.Losses
			}
		}
	}
	if lateTrades >= 2 && lateLosses >= lateTrades/2 {
		out = append(out, "Losses cluster 19:00–21:00 UTC — add entry cutoff before 17:00 UTC for FX/metals/index.")
	}

	// Instrument losers
	for _, b := range byInst {
		if b.Trades >= 3 && b.Wins == 0 {
			out = append(out, fmt.Sprintf(
				"%s: %d trades, 0 wins (net %s) — remove from watchlist or raise min score for this symbol.",
				b.Key, b.Trades, formatMoney(b.TotalNetPL),
			))
		}
	}

	// Slippage
	avgSlip := avgSlippage(trades)
	if avgSlip > 0.15 {
		out = append(out, fmt.Sprintf(
			"Average slippage %.2f%% — lower poll_seconds or tighten max_slippage_pct.",
			avgSlip,
		))
	}

	// Bot-specific config suggestions from stored scores + cfg
	switch botID {
	case config.BotUniverseScanner:
		out = append(out, scannerTweaks(cfg, trades)...)
	case config.BotFxSentiment:
		out = append(out, fxSentimentTweaks(cfg, byExit)...)
	case config.BotBtcCfd:
		out = append(out, btcTweaks(cfg, all)...)
	}

	if len(out) == 0 {
		out = append(out, "No automatic tweaks — metrics look acceptable. Review per-instrument buckets manually.")
	}
	return out
}

func findBucket(buckets []BucketStats, key string) *BucketStats {
	for i := range buckets {
		if buckets[i].Key == key {
			return &buckets[i]
		}
	}
	return nil
}

func avgSlippage(trades []ClosedTrade) float64 {
	var sum float64
	var n int
	for _, t := range trades {
		if t.SlippagePct != 0 {
			sum += math.Abs(t.SlippagePct)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func scannerTweaks(cfg *config.Config, trades []ClosedTrade) []string {
	var out []string
	sc := cfg.Scanner

	winScores, lossScores := scoreSamples(trades)
	if len(winScores) >= 2 && len(lossScores) >= 2 {
		medWin := median(winScores)
		medLoss := median(lossScores)
		if medLoss < medWin && sc.MinSetupScore < medWin {
			suggest := math.Max(sc.MinSetupScore+0.15, medWin)
			suggest = math.Min(suggest, 0.85)
			out = append(out, fmt.Sprintf(
				"Scanner: winning trades median setup score %.2f vs losers %.2f — raise min_setup_score from %.2f to ~%.2f.",
				medWin, medLoss, sc.MinSetupScore, suggest,
			))
		}
	}

	if sc.OpeningRangeCandles < 2 {
		out = append(out, fmt.Sprintf(
			"Scanner: opening_range_candles=%d — use at least 2 M15 candles to reduce false breakouts.",
			sc.OpeningRangeCandles,
		))
	}
	if sc.MinRangeSpreadRatio < 2.5 {
		out = append(out, fmt.Sprintf(
			"Scanner: min_range_spread_ratio=%.1f is low — try 3.0+ to skip thin ranges.",
			sc.MinRangeSpreadRatio,
		))
	}
	if sc.TakeProfitRR < 1.8 {
		out = append(out, fmt.Sprintf(
			"Scanner: take_profit_rr=%.1f — consider 2.0+ given current win rate.",
			sc.TakeProfitRR,
		))
	}
	return out
}

func fxSentimentTweaks(cfg *config.Config, byExit []BucketStats) []string {
	var out []string
	if tp1 := findBucket(byExit, "tp1_partial"); tp1 != nil && tp1.Trades > 0 {
		out = append(out, "FX sentiment: tp1 partial closes detected — fix partial P/L double-booking in monitor before trusting daily loss caps.")
	}
	if cfg.RangeMode.MinBoundaryTouches < 2 {
		out = append(out, fmt.Sprintf(
			"FX sentiment: min_boundary_touches=%d — use 2+ for stronger range validation.",
			cfg.RangeMode.MinBoundaryTouches,
		))
	}
	if cfg.LLMGate.SentimentPersistenceReadings < 2 {
		out = append(out, "FX sentiment: sentiment_persistence_readings=1 — require 2 consistent LLM readings before entry.")
	}
	if cfg.Risk.MaxOpenPositions > 1 {
		out = append(out, fmt.Sprintf(
			"FX sentiment: max_open_positions=%d on shared account — use 1 to avoid correlated USD exposure.",
			cfg.Risk.MaxOpenPositions,
		))
	}
	return out
}

func btcTweaks(cfg *config.Config, all sqlite.PeriodMetrics) []string {
	var out []string
	bc := cfg.BtcCfd
	if all.TradeCount >= 3 && all.WinRate < 0.4 {
		if !bc.M15Confirmation {
			out = append(out, "BTC: enable m15_confirmation to filter counter-trend mean-reversion entries.")
		}
		if bc.DeviationATR < 1.5 {
			out = append(out, fmt.Sprintf(
				"BTC: deviation_atr_multiple=%.1f — try 1.5+ so entries wait for deeper stretch.",
				bc.DeviationATR,
			))
		}
		if bc.MaxTradesPerDay > 5 {
			out = append(out, fmt.Sprintf(
				"BTC: max_trades_per_day=%d — cap at 5 on losing streaks.",
				bc.MaxTradesPerDay,
			))
		}
	}
	if bc.TargetRR < 1.4 {
		out = append(out, fmt.Sprintf(
			"BTC: target_rr=%.1f — thin edge after spread; try 1.5+.",
			bc.TargetRR,
		))
	}
	return out
}

func scoreSamples(trades []ClosedTrade) (wins, losses []float64) {
	for _, t := range trades {
		if t.SetupScore <= 0 {
			continue
		}
		if t.NetPL > 0 {
			wins = append(wins, t.SetupScore)
		} else if t.NetPL < 0 {
			losses = append(losses, t.SetupScore)
		}
	}
	return wins, losses
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 0 {
		return (cp[mid-1] + cp[mid]) / 2
	}
	return cp[mid]
}

// FormatAnalysis renders a human-readable report.
func FormatAnalysis(a BotAnalysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "══ %s ══\n", BotDisplayName(a.BotID))
	fmt.Fprintf(&b, "db: %s\n", a.DBPath)
	if a.Err != nil {
		fmt.Fprintf(&b, "error: %v\n\n", a.Err)
		return b.String()
	}

	fmt.Fprintf(&b, "\nAll-time: %d trades | win rate %.0f%% | net %s | R:R %.2f | max DD %s\n",
		a.AllTime.TradeCount, a.AllTime.WinRate*100, formatMoney(a.AllTime.TotalNetPL),
		a.AllTime.RealizedRR, formatDrawdown(a.AllTime.MaxDrawdown))

	if a.Yesterday.TradeCount > 0 {
		fmt.Fprintf(&b, "Yesterday (%s): %d trades | %dW/%dL | net %s\n",
			a.YesterdayDate.Format("2006-01-02"), a.Yesterday.TradeCount,
			a.Yesterday.WinCount, a.Yesterday.LossCount, formatMoney(a.Yesterday.TotalNetPL))
	} else {
		fmt.Fprintf(&b, "Yesterday (%s): no closed trades\n", a.YesterdayDate.Format("2006-01-02"))
	}

	writeBucket(&b, "By exit reason", a.ByExit)
	writeBucket(&b, "By instrument (worst first)", a.ByInstrument)
	writeBucket(&b, "By close hour UTC", a.ByHourUTC)

	b.WriteString("\nSuggested tweaks:\n")
	for i, s := range a.Suggestions {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, s)
	}
	b.WriteString("\n")
	return b.String()
}

func writeBucket(b *strings.Builder, title string, buckets []BucketStats) {
	if len(buckets) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	limit := len(buckets)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		x := buckets[i]
		wr := 0.0
		if x.Trades > 0 {
			wr = float64(x.Wins) / float64(x.Trades) * 100
		}
		fmt.Fprintf(b, "  %-20s %2d trades  %3.0f%% win  net %s\n",
			x.Key, x.Trades, wr, formatMoney(x.TotalNetPL))
	}
}
