package stockscan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/ym/fxtrade/internal/config"
	"github.com/ym/fxtrade/internal/sentiment"
)

const (
	defaultRSSMaxAge   = 48 * time.Hour
	stockSentimentPrompt = `You are an Indian equity market analyst. Given recent news headlines and a stock symbol, assess short-term sentiment for a swing trade entry.

Respond ONLY with JSON:
{
  "direction": "Positive Sentiment" | "Negative Sentiment" | "Neutral Sentiment",
  "confidence": 0.0-1.0,
  "drivers": ["brief reason"],
  "risks": ["brief risk"]
}

Flag "Negative Sentiment" when headlines suggest material downside: earnings miss, regulatory action, fraud, major downgrade, sharp selloff, or company-specific bad news. Otherwise prefer Neutral or Positive.`
)

// StockSentimentSignal is the LLM response for equity headline checks.
type StockSentimentSignal struct {
	Direction  string   `json:"direction"`
	Confidence float64  `json:"confidence"`
	Drivers    []string `json:"drivers"`
	Risks      []string `json:"risks"`
}

func (s StockSentimentSignal) IsNegative() bool {
	return strings.EqualFold(strings.TrimSpace(s.Direction), "Negative Sentiment")
}

// SentimentChecker validates ranked picks against market RSS headlines.
type SentimentChecker struct {
	llm      *sentiment.LLMClient
	feedURLs []string
	maxAge   time.Duration
	maxHeads int
	http     *http.Client
	feed     *gofeed.Parser
}

func NewSentimentChecker(cfg *config.Config) *SentimentChecker {
	urls := cfg.StockScan.RSSFeeds
	if len(urls) == 0 {
		urls = DefaultRSSFeeds()
	}
	maxHeads := cfg.Sentiment.MaxHeadlines
	if maxHeads <= 0 {
		maxHeads = 25
	}
	maxAge := time.Duration(cfg.Sentiment.HeadlineMaxAgeHours) * time.Hour
	if maxAge <= 0 {
		maxAge = defaultRSSMaxAge
	}

	var llm *sentiment.LLMClient
	if cfg.LLM.APIKey != "" {
		llm = sentiment.NewLLMClient(cfg.LLM)
	}

	return &SentimentChecker{
		llm:      llm,
		feedURLs: urls,
		maxAge:   maxAge,
		maxHeads: maxHeads,
		http:     &http.Client{Timeout: 30 * time.Second},
		feed:     gofeed.NewParser(),
	}
}

func DefaultRSSFeeds() []string {
	return []string{
		"https://economictimes.indiatimes.com/markets/rssfeeds/1977021501.cms",
		"https://www.business-standard.com/rss/markets-106.rss",
	}
}

// Available reports whether LLM sentiment gating can run.
func (c *SentimentChecker) Available() bool {
	return c != nil && c.llm != nil
}

// PickWithSentiment walks ranked candidates and returns the first non-negative pick.
// Falls back to the top ranked candidate when LLM or RSS is unavailable.
func (c *SentimentChecker) PickWithSentiment(ctx context.Context, ranked []Candidate, topN int) (*Candidate, error) {
	if len(ranked) == 0 {
		return nil, nil
	}

	candidates := TopN(ranked, topN)
	if !c.Available() {
		slog.Warn("sentiment check skipped: llm.api_key not configured; using top ranked pick")
		pick := candidates[0]
		return &pick, nil
	}

	headlines, err := c.fetchHeadlines(ctx)
	if err != nil {
		slog.Warn("sentiment RSS fetch failed; proceeding with top ranked pick", "error", err)
		pick := candidates[0]
		return &pick, nil
	}

	for _, cand := range candidates {
		signal, err := c.analyze(ctx, cand.Symbol, headlines)
		if err != nil {
			slog.Warn("sentiment LLM failed; accepting candidate without sentiment gate",
				"symbol", cand.Symbol,
				"error", err,
			)
			pick := cand
			return &pick, nil
		}
		slog.Info("stock sentiment",
			"symbol", cand.Symbol,
			"direction", signal.Direction,
			"confidence", signal.Confidence,
		)
		if signal.IsNegative() {
			slog.Info("skipping negative sentiment candidate", "symbol", cand.Symbol)
			continue
		}
		pick := cand
		return &pick, nil
	}

	slog.Warn("all top candidates flagged negative sentiment; no pick")
	return nil, nil
}

func (c *SentimentChecker) fetchHeadlines(ctx context.Context) ([]sentiment.Headline, error) {
	var all []sentiment.Headline
	cutoff := time.Now().UTC().Add(-c.maxAge)

	for _, feedURL := range c.feedURLs {
		items, err := c.fetchFeed(ctx, feedURL, cutoff)
		if err != nil {
			slog.Warn("rss feed failed", "url", feedURL, "error", err)
			continue
		}
		all = append(all, items...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no RSS headlines fetched")
	}
	if len(all) > c.maxHeads {
		all = all[:c.maxHeads]
	}
	return all, nil
}

func (c *SentimentChecker) fetchFeed(ctx context.Context, feedURL string, cutoff time.Time) ([]sentiment.Headline, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fxtrade-stockscan/1.0")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("status %s", res.Status)
	}

	parsed, err := c.feed.Parse(res.Body)
	if err != nil {
		return nil, err
	}

	source := feedSourceName(feedURL)
	out := make([]sentiment.Headline, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		pub := item.PublishedParsed
		if pub == nil {
			pub = item.UpdatedParsed
		}
		if pub != nil && pub.Before(cutoff) {
			continue
		}
		published := time.Now().UTC()
		if pub != nil {
			published = pub.UTC()
		}
		summary := item.Description
		if item.Content != "" {
			summary = item.Content
		}
		out = append(out, sentiment.Headline{
			Source:    source,
			Title:     item.Title,
			Summary:   summary,
			URL:       item.Link,
			Published: published,
		})
	}
	return out, nil
}

func feedSourceName(url string) string {
	switch {
	case strings.Contains(url, "economictimes"):
		return "Economic Times"
	case strings.Contains(url, "nseindia"):
		return "NSE"
	default:
		return "RSS"
	}
}

type stockSentimentPayload struct {
	Task       string               `json:"task"`
	Symbol     string               `json:"symbol"`
	AsOf       time.Time            `json:"as_of"`
	Headlines  []stockHeadlineEntry `json:"headlines"`
}

type stockHeadlineEntry struct {
	Source  string    `json:"source"`
	Time    time.Time `json:"time"`
	Title   string    `json:"title"`
	Summary string    `json:"summary"`
}

func (c *SentimentChecker) analyze(ctx context.Context, symbol string, headlines []sentiment.Headline) (StockSentimentSignal, error) {
	relevant := filterHeadlinesForSymbol(symbol, headlines)
	payload := stockSentimentPayload{
		Task:   "nse_stock_sentiment",
		Symbol: symbol,
		AsOf:   time.Now().UTC(),
	}
	for _, h := range relevant {
		payload.Headlines = append(payload.Headlines, stockHeadlineEntry{
			Source:  h.Source,
			Time:    h.Published,
			Title:   h.Title,
			Summary: truncateText(h.Summary, 400),
		})
	}
	if len(payload.Headlines) == 0 {
		for _, h := range headlines {
			payload.Headlines = append(payload.Headlines, stockHeadlineEntry{
				Source:  h.Source,
				Time:    h.Published,
				Title:   h.Title,
				Summary: truncateText(h.Summary, 400),
			})
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return StockSentimentSignal{}, err
	}

	signal, err := c.llm.Analyze(ctx, stockSentimentPrompt, data)
	if err != nil {
		return StockSentimentSignal{}, err
	}

	out := StockSentimentSignal{
		Direction:  signal.Direction,
		Confidence: signal.Confidence,
		Drivers:    signal.Drivers,
		Risks:      signal.Risks,
	}
	if out.Direction == "" {
		out.Direction = "Neutral Sentiment"
	}
	return out, nil
}

func filterHeadlinesForSymbol(symbol string, headlines []sentiment.Headline) []sentiment.Headline {
	keys := symbolAliases(symbol)
	var out []sentiment.Headline
	for _, h := range headlines {
		text := strings.ToLower(h.Title + " " + h.Summary)
		for _, key := range keys {
			if key != "" && strings.Contains(text, strings.ToLower(key)) {
				out = append(out, h)
				break
			}
		}
	}
	return out
}

func symbolAliases(symbol string) []string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	aliases := []string{symbol}
	switch symbol {
	case "TATAMOTORS":
		aliases = append(aliases, "Tata Motors")
	case "M&M", "MANDM":
		aliases = append(aliases, "Mahindra")
	case "HDFCBANK":
		aliases = append(aliases, "HDFC Bank")
	case "ICICIBANK":
		aliases = append(aliases, "ICICI Bank")
	case "SBIN":
		aliases = append(aliases, "SBI", "State Bank")
	case "RELIANCE":
		aliases = append(aliases, "Reliance Industries")
	case "BAJFINANCE":
		aliases = append(aliases, "Bajaj Finance")
	case "BAJAJFINSV":
		aliases = append(aliases, "Bajaj Finserv")
	}
	return aliases
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
