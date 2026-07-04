package stockscan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ym/fxtrade/internal/config"
)

// Candidate is a watchlist symbol that passed technical filters.
type Candidate struct {
	Symbol  string
	Close   float64
	SMA     float64
	RSI     float64
	H1Trend string // optional hourly trend label, empty when unavailable
}

// Contender is a ranked filter-passing symbol shown in the alert email.
type Contender struct {
	Rank      int
	Candidate Candidate
	OneLiner  string
	Selected  bool
}

// FilterCandidates keeps symbols where close > SMA and RSI is within bounds.
func FilterCandidates(candidates []Candidate, rsiMin, rsiMax float64) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		if PassesFilter(c.Close, c.SMA, c.RSI, rsiMin, rsiMax) {
			out = append(out, c)
		}
	}
	return out
}

// RankByRSI sorts ascending by RSI (lowest first).
func RankByRSI(candidates []Candidate) []Candidate {
	out := append([]Candidate(nil), candidates...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].RSI != out[j].RSI {
			return out[i].RSI < out[j].RSI
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out
}

// TopN returns up to n ranked candidates.
func TopN(ranked []Candidate, n int) []Candidate {
	if n <= 0 || len(ranked) == 0 {
		return nil
	}
	if len(ranked) <= n {
		return append([]Candidate(nil), ranked...)
	}
	return append([]Candidate(nil), ranked[:n]...)
}

// pctAboveSMA returns how far close is above SMA as a percentage.
func pctAboveSMA(close, sma float64) float64 {
	if sma == 0 {
		return 0
	}
	return (close - sma) / sma * 100
}

// BuildReasons returns human-readable bullets explaining why a candidate was shortlisted.
func BuildReasons(c Candidate, cfg config.StockScanConfig, sentimentNote string) []string {
	pct := pctAboveSMA(c.Close, c.SMA)
	reasons := []string{
		fmt.Sprintf("Close ₹%.2f is %.1f%% above SMA(%d) ₹%.2f — price above moving average (uptrend filter)",
			c.Close, pct, cfg.SMAPeriod, c.SMA),
		fmt.Sprintf("RSI %.1f is in pullback zone [%.0f–%.0f] — oversold bounce setup within uptrend",
			c.RSI, cfg.RSIMin, cfg.RSIMax),
	}
	if c.H1Trend != "" {
		reasons = append(reasons, fmt.Sprintf("H1 trend: %s", c.H1Trend))
	}
	if sentimentNote != "" {
		reasons = append(reasons, fmt.Sprintf("Sentiment: %s", sentimentNote))
	}
	return reasons
}

// BuildOneLiner returns a brief technical summary for a contender line item.
func BuildOneLiner(c Candidate, cfg config.StockScanConfig) string {
	pct := pctAboveSMA(c.Close, c.SMA)
	line := fmt.Sprintf("RSI %.1f, close ₹%.2f, SMA(%d) ₹%.2f (+%.1f%%)",
		c.RSI, c.Close, cfg.SMAPeriod, c.SMA, pct)
	if c.H1Trend != "" {
		line += fmt.Sprintf(", H1 %s", c.H1Trend)
	}
	return line
}

// BuildContenders builds ranked contender rows for the alert email.
func BuildContenders(ranked []Candidate, cfg config.StockScanConfig, selectedSymbol string) []Contender {
	selectedSymbol = strings.ToUpper(strings.TrimSpace(selectedSymbol))
	out := make([]Contender, len(ranked))
	for i, c := range ranked {
		out[i] = Contender{
			Rank:      i + 1,
			Candidate: c,
			OneLiner:  BuildOneLiner(c, cfg),
			Selected:  strings.EqualFold(c.Symbol, selectedSymbol),
		}
	}
	return out
}
