package strategy

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/sentiment"
)

type CycleSummary struct {
	Instrument   string
	Mode         string
	ModeChanged  string
	Action       string
	Reason       string
	IntervalMins int
}

func FormatCycleEmail(snap market.Snapshot, band RangeBand, cfg *config.Config, summary CycleSummary, sig sentiment.SentimentSignal, hasSig bool) string {
	instrument := summary.Instrument
	if instrument == "" {
		instrument = oanda.DefaultInstrument
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s strategy check (every %d min)\n", instrument, summary.IntervalMins)

	switch summary.Action {
	case "market_entry", "place_buy_limit", "place_sell_limit":
		fmt.Fprintf(&b, "Action taken: %s\n\n", summary.Action)
	case "manage_position", "pending_limits":
		fmt.Fprintf(&b, "Status: %s\n\n", summary.Action)
	default:
		b.WriteString("No trade placed.\n\n")
	}

	fmt.Fprintf(&b, "Mode: %s\n", summary.Mode)
	if summary.ModeChanged != "" {
		fmt.Fprintf(&b, "Mode changed: %s\n", summary.ModeChanged)
	}
	if snap.Mid > 0 {
		fmt.Fprintf(&b, "Price: %s  |  spread: %.1f pips\n", oanda.FormatPrice(snap.Mid), snap.SpreadPips)
	} else {
		b.WriteString("Price: unavailable (market snapshot not loaded)\n")
	}

	if summary.Reason != "" {
		fmt.Fprintf(&b, "\nDecision: %s\n", summary.Reason)
	}

	if snap.Mid > 0 && (summary.Mode == ModeStandAside || (summary.Mode == ModeRange && !band.Valid)) {
		b.WriteString("\nWhy standing aside:\n")
		b.WriteString(notify.BulletList(ExplainStandAside(snap, band, cfg.RangeMode)))
	} else if summary.Mode == ModeTrend && summary.Action == "no_trade" && summary.Reason != "" {
		b.WriteString("\nTrend entry not taken:\n")
		b.WriteString(notify.BulletList([]string{summary.Reason}))
	}

	if snap.Mid > 0 && (band.WidthPips > 0 || summary.Mode != ModeStandAside) {
		b.WriteString("\nRange structure:\n")
		b.WriteString(notify.BulletList(ExplainRange(band, cfg.RangeMode)))
	}

	if summary.Mode == ModeTrend {
		b.WriteString("\nTrend structure:\n")
		b.WriteString(notify.BulletList(ExplainTrend(snap)))
	}

	b.WriteString("\nSentiment:\n")
	b.WriteString(notify.BulletList(ExplainSentiment(sig, hasSig)))

	b.WriteString("\nReminder: max 4 trades/month, 1 open position. STAND_ASIDE is normal most of the time.\n")
	return b.String()
}

func ExplainStandAside(snap market.Snapshot, band RangeBand, cfg config.RangeModeConfig) []string {
	var reasons []string

	if band.WidthPips < cfg.MinRangeWidthPips {
		reasons = append(reasons, fmt.Sprintf("Range too narrow: %.0f pips (need ≥%.0f)", band.WidthPips, cfg.MinRangeWidthPips))
	}
	if band.TouchesHigh < cfg.MinBoundaryTouches {
		reasons = append(reasons, fmt.Sprintf("Not enough resistance touches: %d (need ≥%d)", band.TouchesHigh, cfg.MinBoundaryTouches))
	}
	if band.TouchesLow < cfg.MinBoundaryTouches {
		reasons = append(reasons, fmt.Sprintf("Not enough support touches: %d (need ≥%d)", band.TouchesLow, cfg.MinBoundaryTouches))
	}
	if !weeklyEMAsFlat(snap, cfg.FlatEMAThresholdPct) && snap.Mid > 0 {
		diff := math.Abs(snap.EMA20W-snap.EMA50W) / snap.Mid * 100
		reasons = append(reasons, fmt.Sprintf("Weekly EMAs trending apart: %.2f%% apart (need ≤%.2f%% for range)", diff, cfg.FlatEMAThresholdPct))
	}

	longRegime := snap.EMA20W > snap.EMA50W && snap.Mid > snap.EMA20W
	shortRegime := snap.EMA20W < snap.EMA50W && snap.Mid < snap.EMA50W
	if !longRegime && !shortRegime {
		if snap.EMA20W > snap.EMA50W {
			reasons = append(reasons, fmt.Sprintf("Bullish weekly structure but price %.5f is below 20-week EMA %.5f — trend not confirmed", snap.Mid, snap.EMA20W))
		} else if snap.EMA20W < snap.EMA50W {
			reasons = append(reasons, fmt.Sprintf("Bearish weekly structure but price %.5f is above 20-week EMA %.5f — trend not confirmed", snap.Mid, snap.EMA20W))
		} else {
			reasons = append(reasons, "Weekly regime is choppy — no clean trend")
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "Market is between range and trend setups")
	}
	return reasons
}

func ExplainRange(band RangeBand, cfg config.RangeModeConfig) []string {
	return []string{
		fmt.Sprintf("Valid range: %s", notify.Checkmark(band.Valid)),
		fmt.Sprintf("Width: %.0f pips (need ≥%.0f)", band.WidthPips, cfg.MinRangeWidthPips),
		fmt.Sprintf("Touches: %d high / %d low (need ≥%d each)", band.TouchesHigh, band.TouchesLow, cfg.MinBoundaryTouches),
		fmt.Sprintf("Support limit: %s  |  resistance limit: %s", oanda.FormatPrice(band.BuyLimitPrice), oanda.FormatPrice(band.SellLimitPrice)),
	}
}

func ExplainTrend(snap market.Snapshot) []string {
	longRegime := snap.EMA20W > snap.EMA50W && snap.Mid > snap.EMA20W
	shortRegime := snap.EMA20W < snap.EMA50W && snap.Mid < snap.EMA50W
	regime := "choppy"
	if longRegime {
		regime = "bullish (price above 20w & 50w EMAs)"
	} else if shortRegime {
		regime = "bearish (price below 20w & 50w EMAs)"
	}
	return []string{
		fmt.Sprintf("Weekly regime: %s", regime),
		fmt.Sprintf("H4 RSI: %.1f  |  H4 close: %s  |  H4 EMA20: %s", snap.RSI14H4, oanda.FormatPrice(snap.LastH4Close), oanda.FormatPrice(snap.EMA20H4)),
		"Needs H4 pullback to EMA20 with RSI in band before market entry",
	}
}

func ExplainSentiment(sig sentiment.SentimentSignal, hasSig bool) []string {
	if !hasSig {
		return []string{"No fresh sentiment reading (runs every 30 min)"}
	}
	age := time.Since(sig.AnalyzedAt).Round(time.Minute)
	return []string{
		fmt.Sprintf("%s @ %.0f%% confidence (%s base bias) — opinion only", sig.Direction, sig.Confidence*100, sig.BaseBias),
		fmt.Sprintf("Updated %s ago  |  event risk: %s", age, sig.EventRisk),
	}
}

func CycleEmailSubject(summary CycleSummary) string {
	instrument := summary.Instrument
	if instrument == "" {
		instrument = oanda.DefaultInstrument
	}
	if summary.Action != "" && summary.Action != "no_trade" {
		return fmt.Sprintf("fxtrade: %s %s — %s", instrument, summary.Mode, summary.Action)
	}
	return fmt.Sprintf("fxtrade: %s strategy — %s", instrument, summary.Mode)
}
