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
	Key          string
	Trades       int
	Wins         int
	Losses       int
	Unreconciled int
	TotalNetPL   float64
}

// DataQuality summarizes whether closed-trade rows have trustworthy P/L.
type DataQuality struct {
	TotalTrades            int
	ReconciledCount        int
	UnreconciledCount      int
	MissingInstrumentCount int
}

// BotAnalysis is a data-driven review of one bot's SQLite store.
type BotAnalysis struct {
	BotID            string
	DBPath           string
	Err              error
	AllTime          sqlite.PeriodMetrics
	Yesterday        sqlite.PeriodMetrics
	YesterdayDate    time.Time
	Trades           []ClosedTrade
	ByInstrument     []BucketStats
	ByExit           []BucketStats
	ByHourUTC        []BucketStats
	Suggestions      []string
	DataQuality      DataQuality
	ReconciledWins   int
	ReconciledLosses int
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
	out.DataQuality = assessDataQuality(trades)
	out.ReconciledWins, out.ReconciledLosses, _ = reconciledWinLossCounts(trades)

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
	out.Suggestions = suggestTweaks(cfg, botID, trades, out.DataQuality, out.AllTime, out.Yesterday, out.ByInstrument, out.ByExit, out.ByHourUTC)
	return out
}

func assessDataQuality(trades []ClosedTrade) DataQuality {
	dq := DataQuality{TotalTrades: len(trades)}
	for _, t := range trades {
		if IsTradeReconciled(t.NetPL, t.RealizedPL) {
			dq.ReconciledCount++
		} else {
			dq.UnreconciledCount++
		}
		if strings.TrimSpace(t.Instrument) == "" {
			dq.MissingInstrumentCount++
		}
	}
	return dq
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
		if !IsTradeReconciled(t.NetPL, t.RealizedPL) {
			b.Unreconciled++
			continue
		}
		if t.NetPL > 0.01 {
			b.Wins++
		} else if t.NetPL < -0.01 {
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
	dq DataQuality,
	all, yesterday sqlite.PeriodMetrics,
	byInst, byExit, byHour []BucketStats,
) []string {
	if len(trades) == 0 {
		return []string{"No closed trades in database yet — run bots live or check db_path."}
	}

	var out []string
	badData := dq.UnreconciledCount > 0 || dq.MissingInstrumentCount > 0
	unreconciledPct := 0.0
	if dq.TotalTrades > 0 {
		unreconciledPct = float64(dq.UnreconciledCount) / float64(dq.TotalTrades)
	}
	reliableMetrics := !badData || unreconciledPct <= 0.5

	if badData {
		out = append(out, fmt.Sprintf(
			"DATA QUALITY: %d reconciled, %d unreconciled ($0 P/L — lookup failed), %d missing instrument — run reconcile-trades before tuning.",
			dq.ReconciledCount, dq.UnreconciledCount, dq.MissingInstrumentCount,
		))
	}

	// Yesterday all-red day (only when yesterday has reconciled losses)
	if yesterday.TradeCount > 0 && yesterday.WinCount == 0 && reliableMetrics && yesterday.TotalNetPL < -0.01 {
		out = append(out, fmt.Sprintf(
			"Yesterday (%s): %d trades, all losses (net %s). Pause new entries until filters are tightened.",
			yesterday.Start.Format("2006-01-02"), yesterday.TradeCount, formatMoney(yesterday.TotalNetPL),
		))
	}

	reconciledWR, reconciledTrades, _ := reconciledWinStats(trades)
	if reliableMetrics && reconciledTrades >= 5 && reconciledWR < 0.4 {
		out = append(out, fmt.Sprintf(
			"Win rate %.0f%% over %d reconciled trades is below 40%% — tighten entry filters before increasing size.",
			reconciledWR*100, reconciledTrades,
		))
	}

	if reliableMetrics && all.RealizedRR > 0 && all.RealizedRR < 1.0 && reconciledWR < 0.55 && reconciledTrades >= 3 {
		out = append(out, fmt.Sprintf(
			"Realized R:R %.2f with %.0f%% win rate — raise take-profit R:R or reduce stop distance.",
			all.RealizedRR, reconciledWR*100,
		))
	}

	// Force-flat analysis
	if reliableMetrics {
		if ff := findBucket(byExit, "force_flat"); ff != nil && ff.Trades >= 2 {
			reconciledFF := ff.Trades - ff.Unreconciled
			if reconciledFF >= 2 {
				ffWR := 0.0
				if reconciledFF > 0 {
					ffWR = float64(ff.Wins) / float64(reconciledFF)
				}
				if ff.TotalNetPL < -0.01 && ffWR < 0.35 {
					out = append(out, fmt.Sprintf(
						"%d force-flat closes net %s (%.0f%% win) — block new entries ~2h before force_flat_utc, or only force-flat losers.",
						ff.Trades, formatMoney(ff.TotalNetPL), ffWR*100,
					))
				}
			}
		}
	}

	// Late-hour losses (scanner force-flat window)
	if reliableMetrics {
		lateLosses, lateTrades := 0, 0
		for _, b := range byHour {
			h := 0
			fmt.Sscanf(b.Key, "%d:", &h)
			if h >= 19 && h <= 21 {
				lateTrades += b.Trades - b.Unreconciled
				if b.TotalNetPL < -0.01 {
					lateLosses += b.Losses
				}
			}
		}
		if lateTrades >= 2 && lateLosses >= lateTrades/2 {
			out = append(out, "Losses cluster 19:00–21:00 UTC — add entry cutoff before 17:00 UTC for FX/metals/index.")
			if cfg.Scanner.EntryCutoffBeforeForceFlatMinutes <= 0 {
				out = append(out, "Scanner: entry_cutoff_before_force_flat_minutes=0 — set 120+ to block late-session entries before force_flat_utc.")
			}
		}
	}

	// Instrument losers — skip when bucket is all unreconciled $0
	if reliableMetrics {
		for _, b := range byInst {
			if b.Key == "(unknown)" {
				continue
			}
			if b.Trades >= 3 && b.Wins == 0 {
				if math.Abs(b.TotalNetPL) < 0.01 && b.Unreconciled == b.Trades {
					continue
				}
				out = append(out, fmt.Sprintf(
					"%s: %d trades, 0 wins (net %s) — remove from watchlist or raise min score for this symbol.",
					b.Key, b.Trades, formatMoney(b.TotalNetPL),
				))
			}
		}
	}

	// Slippage (reconciled trades only)
	if reliableMetrics {
		avgSlip := avgSlippage(trades)
		if avgSlip > 0.15 {
			out = append(out, fmt.Sprintf(
				"Average slippage %.2f%% — lower poll_seconds or tighten max_slippage_pct.",
				avgSlip,
			))
		}
	}

	// Bot-specific config suggestions from stored scores + cfg
	if reliableMetrics || dq.ReconciledCount >= 3 {
		switch botID {
		case config.BotUniverseScanner:
			out = append(out, scannerTweaks(cfg, trades, dq)...)
		case config.BotFxSentiment:
			out = append(out, fxSentimentTweaks(cfg, byExit)...)
		case config.BotBtcCfd:
			if reliableMetrics {
				out = append(out, btcTweaks(cfg, all)...)
			}
		}
	}

	if len(out) == 0 {
		if badData {
			out = append(out, "Fix unreconciled trades (reconcile-trades) before strategy tweaks — metrics are unreliable.")
		} else {
			out = append(out, "No automatic tweaks — metrics look acceptable. Review per-instrument buckets manually.")
		}
	}
	return out
}

func reconciledWinStats(trades []ClosedTrade) (winRate float64, count, wins int) {
	wins, losses, _ := reconciledWinLossCounts(trades)
	count = wins + losses
	if count > 0 {
		winRate = float64(wins) / float64(count)
	}
	return winRate, count, wins
}

func reconciledWinLossCounts(trades []ClosedTrade) (wins, losses, unreconciled int) {
	for _, t := range trades {
		if !IsTradeReconciled(t.NetPL, t.RealizedPL) {
			unreconciled++
			continue
		}
		if t.NetPL > 0.01 {
			wins++
		} else if t.NetPL < -0.01 {
			losses++
		}
	}
	return wins, losses, unreconciled
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

func scannerTweaks(cfg *config.Config, trades []ClosedTrade, dq DataQuality) []string {
	var out []string
	sc := cfg.Scanner
	reliableWR := dq.UnreconciledCount == 0 || float64(dq.UnreconciledCount)/float64(dq.TotalTrades) <= 0.5

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
	if sc.MinRangeSpreadRatio < 2.0 {
		out = append(out, fmt.Sprintf(
			"Scanner: min_range_spread_ratio=%.1f is low — try 2.0+ to skip thin ranges.",
			sc.MinRangeSpreadRatio,
		))
	} else if sc.MinRangeSpreadRatio < 2.5 && reliableWR {
		wr, n, _ := reconciledWinStats(trades)
		if n >= 5 && wr < 0.4 {
			out = append(out, fmt.Sprintf(
				"Scanner: min_range_spread_ratio=%.1f with %.0f%% win rate — consider 2.5+ to skip thin ranges.",
				sc.MinRangeSpreadRatio, wr*100,
			))
		}
	}
	if !sc.RequireTrendAlignment && reliableWR {
		wr, n, _ := reconciledWinStats(trades)
		if n >= 5 && wr < 0.4 {
			out = append(out, "Scanner: require_trend_alignment=false — enable to skip counter-H1 breakouts.")
		}
	}
	if reliableWR && sc.TakeProfitRR < 1.8 {
		wr, n, _ := reconciledWinStats(trades)
		if n >= 3 {
			out = append(out, fmt.Sprintf(
				"Scanner: take_profit_rr=%.1f — consider 2.0+ given %.0f%% win rate over %d reconciled trades.",
				sc.TakeProfitRR, wr*100, n,
			))
		}
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
		if t.SetupScore <= 0 || !IsTradeReconciled(t.NetPL, t.RealizedPL) {
			continue
		}
		if t.NetPL > 0.01 {
			wins = append(wins, t.SetupScore)
		} else if t.NetPL < -0.01 {
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

	fmt.Fprintf(&b, "\nAll-time: %d trades | win rate %s | net %s | R:R %.2f | max DD %s\n",
		a.AllTime.TradeCount,
		formatAnalysisWinRate(a.DataQuality, a.ReconciledWins, a.ReconciledLosses),
		formatMoney(a.AllTime.TotalNetPL),
		a.AllTime.RealizedRR, formatDrawdown(a.AllTime.MaxDrawdown))

	if a.DataQuality.UnreconciledCount > 0 || a.DataQuality.MissingInstrumentCount > 0 {
		fmt.Fprintf(&b, "Data quality: reconciled %d | unreconciled %d ($0 P/L) | missing instrument %d\n",
			a.DataQuality.ReconciledCount, a.DataQuality.UnreconciledCount, a.DataQuality.MissingInstrumentCount)
	}

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
		wr := bucketWinRateLabel(x)
		fmt.Fprintf(b, "  %-20s %2d trades  %s  net %s\n",
			x.Key, x.Trades, wr, formatMoney(x.TotalNetPL))
	}
}

func bucketWinRateLabel(x BucketStats) string {
	reconciled := x.Trades - x.Unreconciled
	if reconciled == 0 && x.Unreconciled > 0 {
		return fmt.Sprintf("n/a (%d unreconciled)", x.Unreconciled)
	}
	wr := 0.0
	if reconciled > 0 {
		wr = float64(x.Wins) / float64(reconciled) * 100
	}
	if x.Unreconciled > 0 {
		return fmt.Sprintf("%.0f%% win (%dW/%dL, %d unreconciled)", wr, x.Wins, x.Losses, x.Unreconciled)
	}
	return fmt.Sprintf("%.0f%% win", wr)
}

func formatAnalysisWinRate(dq DataQuality, reconciledWins, reconciledLosses int) string {
	reconciled := reconciledWins + reconciledLosses
	if dq.UnreconciledCount > 0 && reconciled == 0 {
		return fmt.Sprintf("n/a (%d unreconciled)", dq.UnreconciledCount)
	}
	if reconciled == 0 {
		return "n/a (0 reconciled)"
	}
	wr := float64(reconciledWins) / float64(reconciled) * 100
	if dq.UnreconciledCount > 0 {
		return fmt.Sprintf("%.1f%% (%dW / %dL, %d unreconciled)", wr, reconciledWins, reconciledLosses, dq.UnreconciledCount)
	}
	return fmt.Sprintf("%.1f%% (%dW / %dL)", wr, reconciledWins, reconciledLosses)
}

// FormatDailyTweaks renders suggested config tweaks only (no duplicate P/L buckets).
func FormatDailyTweaks(cfg *config.Config, now time.Time) string {
	var b strings.Builder
	var any bool
	for _, botID := range TradingBotIDs() {
		a := AnalyzeBot(cfg, botID, now)
		if a.Err != nil || len(a.Suggestions) == 0 {
			continue
		}
		var tweaks []string
		for _, s := range a.Suggestions {
			if strings.HasPrefix(s, "No closed trades in database yet") {
				continue
			}
			tweaks = append(tweaks, s)
		}
		if len(tweaks) == 0 {
			continue
		}
		if !any {
			b.WriteString("── Suggested tweaks ──\n")
			any = true
		}
		fmt.Fprintf(&b, "\n%s\n", BotDisplayName(botID))
		for _, s := range tweaks {
			fmt.Fprintf(&b, "  • %s\n", s)
		}
	}
	if !any {
		return ""
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// FormatDailyAnalysis renders a concise analysis block for the daily email.
func FormatDailyAnalysis(cfg *config.Config, now time.Time) string {
	var b strings.Builder
	b.WriteString("── Analysis & Suggested Tweaks ──\n")
	for _, botID := range TradingBotIDs() {
		a := AnalyzeBot(cfg, botID, now)
		fmt.Fprintf(&b, "\n%s\n", BotDisplayName(botID))
		if a.Err != nil {
			fmt.Fprintf(&b, "  (no data — %v)\n", a.Err)
			continue
		}
		if a.AllTime.TradeCount == 0 {
			b.WriteString("  No closed trades in database.\n")
		} else {
			fmt.Fprintf(&b, "  All-time: %d trades, win rate %s, net P&L %s\n",
				a.AllTime.TradeCount,
				formatAnalysisWinRate(a.DataQuality, a.ReconciledWins, a.ReconciledLosses),
				formatMoney(a.AllTime.TotalNetPL),
			)
			if a.DataQuality.UnreconciledCount > 0 || a.DataQuality.MissingInstrumentCount > 0 {
				fmt.Fprintf(&b, "  Data quality: reconciled %d | unreconciled %d ($0 P/L) | missing instrument %d\n",
					a.DataQuality.ReconciledCount, a.DataQuality.UnreconciledCount, a.DataQuality.MissingInstrumentCount)
			}
			if a.Yesterday.TradeCount > 0 {
				fmt.Fprintf(&b, "  Yesterday: %d trades, net %s\n",
					a.Yesterday.TradeCount, formatMoney(a.Yesterday.TotalNetPL))
			}
			writeDailyBucket(&b, "  By exit reason", a.ByExit, 5)
			writeDailyBucket(&b, "  By instrument", a.ByInstrument, 5)
		}
		if len(a.Suggestions) > 0 {
			b.WriteString("  Suggested tweaks:\n")
			for _, s := range a.Suggestions {
				fmt.Fprintf(&b, "  • %s\n", s)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func writeDailyBucket(b *strings.Builder, title string, buckets []BucketStats, limit int) {
	if len(buckets) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", title)
	if limit > len(buckets) {
		limit = len(buckets)
	}
	for i := 0; i < limit; i++ {
		x := buckets[i]
		label := x.Key
		if x.Unreconciled == x.Trades && x.Trades > 0 && math.Abs(x.TotalNetPL) < 0.01 {
			label += " (unreconciled)"
		}
		fmt.Fprintf(b, "    %s: %d trades, net %s\n", label, x.Trades, formatMoney(x.TotalNetPL))
	}
}
