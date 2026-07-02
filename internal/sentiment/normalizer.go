package sentiment

import (
	"sort"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

var relevanceKeywords = []string{
	"aud", "usd", "rba", "fed", "fomc", "australia", "us dollar", "dollar",
	"iron ore", "commodity", "rate", "inflation", "cpi", "employment", "nfp",
	"reserve bank", "monetary", "forex", "currency", "aussie",
}

type Normalized struct {
	Headlines       []Headline
	Events          []CalendarEvent
	HighEventRisk   bool
}

func Normalize(raw RawData, cfg config.SentimentConfig) Normalized {
	maxAge := time.Duration(cfg.HeadlineMaxAgeHours) * time.Hour
	cutoff := time.Now().UTC().Add(-maxAge)

	filtered := make([]Headline, 0, len(raw.Headlines))
	seen := make(map[string]struct{})
	for _, h := range raw.Headlines {
		if h.Published.Before(cutoff) {
			continue
		}
		if !isRelevant(h.Title + " " + h.Summary) {
			continue
		}
		key := dedupeKey(h)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, h)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Published.After(filtered[j].Published)
	})
	if len(filtered) > cfg.MaxHeadlines {
		filtered = filtered[:cfg.MaxHeadlines]
	}

	events := make([]CalendarEvent, 0, len(raw.Events))
	highRisk := false
	soon := time.Now().UTC().Add(4 * time.Hour)
	for _, e := range raw.Events {
		if e.Time.After(soon) {
			continue
		}
		if strings.EqualFold(e.Country, "AU") || strings.EqualFold(e.Country, "US") {
			events = append(events, e)
			if e.Impact == "high" && e.Time.After(time.Now().UTC()) {
				highRisk = true
			}
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})

	return Normalized{
		Headlines:     filtered,
		Events:        events,
		HighEventRisk: highRisk,
	}
}

func isRelevant(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range relevanceKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func dedupeKey(h Headline) string {
	title := strings.ToLower(strings.TrimSpace(h.Title))
	title = strings.Join(strings.Fields(title), " ")
	return title
}
