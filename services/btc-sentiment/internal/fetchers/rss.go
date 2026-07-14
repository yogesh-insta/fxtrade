package fetchers

import (
	"context"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

func (c *NewsClient) fetchRSS(ctx context.Context, start, end time.Time) ([]NewsItem, error) {
	fp := gofeed.NewParser()
	fp.Client = c.HTTPClient

	var items []NewsItem
	var lastErr error
	for _, feedURL := range c.RSSFeeds {
		feed, err := fp.ParseURLWithContext(feedURL, ctx)
		if err != nil {
			lastErr = err
			continue
		}
		source := feed.Title
		if source == "" {
			source = feedURL
		}
		for _, entry := range feed.Items {
			if entry == nil {
				continue
			}
			ts := entry.PublishedParsed
			if ts == nil {
				ts = entry.UpdatedParsed
			}
			if ts == nil {
				continue
			}
			if ts.Before(start) || ts.After(end) {
				continue
			}
			title := strings.TrimSpace(entry.Title)
			if title == "" {
				continue
			}
			// Prefer BTC-related items from general crypto RSS.
			lower := strings.ToLower(title + " " + entry.Description)
			if !strings.Contains(lower, "bitcoin") && !strings.Contains(lower, "btc") {
				continue
			}
			items = append(items, NewsItem{
				Headline:  title,
				Source:    source,
				Timestamp: *ts,
				URL:       entry.Link,
			})
		}
	}
	if len(items) == 0 {
		if lastErr != nil {
			return nil, NewFetchError("FetchNews", lastErr)
		}
		return nil, NewNoDataError("FetchNews")
	}
	return items, nil
}
