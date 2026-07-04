package stockscan

import "sort"

// Candidate is a watchlist symbol that passed technical filters.
type Candidate struct {
	Symbol string
	Close  float64
	SMA    float64
	RSI    float64
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
