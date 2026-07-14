package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cryptoPanicURL = "https://cryptopanic.com/api/v1/posts/"

// NewsClient fetches BTC-related news from CryptoPanic or RSS fallback.
type NewsClient struct {
	APIKey     string
	HTTPClient *http.Client
	RSSFeeds   []string
}

// NewNewsClient creates a news fetcher. Empty APIKey enables RSS fallback.
func NewNewsClient(apiKey string, httpClient *http.Client) *NewsClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &NewsClient{
		APIKey:     apiKey,
		HTTPClient: httpClient,
		RSSFeeds: []string{
			"https://www.coindesk.com/arc/outboundfeeds/rss/",
			"https://cointelegraph.com/rss",
		},
	}
}

type cryptoPanicResponse struct {
	Results []struct {
		Title     string `json:"title"`
		Published string `json:"published_at"`
		URL       string `json:"url"`
		Source    struct {
			Title string `json:"title"`
		} `json:"source"`
	} `json:"results"`
}

// FetchNews returns news items in [start, end]. Uses CryptoPanic when API key
// is set; otherwise falls back to CoinDesk/CoinTelegraph RSS.
func (c *NewsClient) FetchNews(ctx context.Context, start, end time.Time) ([]NewsItem, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return c.fetchRSS(ctx, start, end)
	}
	return c.fetchCryptoPanic(ctx, start, end)
}

func (c *NewsClient) fetchCryptoPanic(ctx context.Context, start, end time.Time) ([]NewsItem, error) {
	q := url.Values{}
	q.Set("auth_token", c.APIKey)
	q.Set("currencies", "BTC")
	q.Set("kind", "news")
	q.Set("public", "true")

	body, _, err := HTTPDo(ctx, c.HTTPClient, http.MethodGet, cryptoPanicURL+"?"+q.Encode(), nil, nil)
	if err != nil {
		return nil, NewFetchError("FetchNews", err)
	}

	var parsed cryptoPanicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, NewFetchError("FetchNews", fmt.Errorf("decode: %w", err))
	}

	items := make([]NewsItem, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		ts, err := time.Parse(time.RFC3339, r.Published)
		if err != nil {
			ts, err = time.Parse("2006-01-02T15:04:05Z", r.Published)
			if err != nil {
				continue
			}
		}
		if ts.Before(start) || ts.After(end) {
			continue
		}
		items = append(items, NewsItem{
			Headline:  r.Title,
			Source:    r.Source.Title,
			Timestamp: ts,
			URL:       r.URL,
		})
	}
	if len(items) == 0 {
		return nil, NewNoDataError("FetchNews")
	}
	return items, nil
}
