package sentiment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/ym/fxtrade/internal/config"
)

const (
	finnhubNewsURL     = "https://finnhub.io/api/v1/news"
	finnhubCalendarURL = "https://finnhub.io/api/v1/calendar/economic"

	rbaMediaURL    = "https://www.rba.gov.au/rss/rss-cb-media-releases.xml"
	rbaSpeechURL   = "https://www.rba.gov.au/rss/rss-cb-speeches.xml"
	fedMonetaryURL = "https://www.federalreserve.gov/feeds/press_monetary.xml"
)

type Fetcher struct {
	finnhubKey string
	http       *http.Client
	feed       *gofeed.Parser

	calMu      sync.Mutex
	lastCalErr string
}

func NewFetcher(cfg config.FinnhubConfig) *Fetcher {
	return &Fetcher{
		finnhubKey: cfg.APIKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		feed: gofeed.NewParser(),
	}
}

func (f *Fetcher) FetchAll(ctx context.Context, maxAge time.Duration) (RawData, error) {
	var out RawData
	out.FetchedAt = time.Now()

	news, err := f.fetchFinnhubNews(ctx)
	if err != nil {
		return RawData{}, fmt.Errorf("finnhub news: %w", err)
	}
	out.Headlines = append(out.Headlines, news...)

	events, err := f.fetchFinnhubCalendar(ctx)
	if err != nil {
		f.logCalendarError(err)
	} else {
		f.resetCalendarError()
		out.Events = events
	}

	rssFeeds := []struct {
		source string
		url    string
	}{
		{"RBA", rbaMediaURL},
		{"RBA", rbaSpeechURL},
		{"Fed", fedMonetaryURL},
	}
	for _, feed := range rssFeeds {
		items, err := f.fetchRSS(ctx, feed.source, feed.url, maxAge)
		if err != nil {
			return RawData{}, fmt.Errorf("rss %s: %w", feed.source, err)
		}
		out.Headlines = append(out.Headlines, items...)
	}

	return out, nil
}

// logCalendarError logs the calendar failure once per distinct error, at INFO
// (economic-calendar access is a known plan limitation, not a per-cycle alarm).
func (f *Fetcher) logCalendarError(err error) {
	msg := err.Error()
	f.calMu.Lock()
	first := msg != f.lastCalErr
	f.lastCalErr = msg
	f.calMu.Unlock()
	if first {
		slog.Info("finnhub economic calendar unavailable; continuing without events (suppressing repeats)", "error", err)
	}
}

func (f *Fetcher) resetCalendarError() {
	f.calMu.Lock()
	recovered := f.lastCalErr != ""
	f.lastCalErr = ""
	f.calMu.Unlock()
	if recovered {
		slog.Info("finnhub economic calendar recovered")
	}
}

type finnhubNewsItem struct {
	Category string `json:"category"`
	Datetime int64  `json:"datetime"`
	Headline string `json:"headline"`
	ID       int64  `json:"id"`
	Source   string `json:"source"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

func (f *Fetcher) fetchFinnhubNews(ctx context.Context) ([]Headline, error) {
	q := url.Values{}
	q.Set("category", "forex")
	q.Set("token", f.finnhubKey)

	var items []finnhubNewsItem
	if err := f.getJSON(ctx, finnhubNewsURL, q, &items); err != nil {
		return nil, err
	}

	out := make([]Headline, 0, len(items))
	for _, item := range items {
		out = append(out, Headline{
			Source:    "Finnhub/" + item.Source,
			Title:     item.Headline,
			Summary:   item.Summary,
			URL:       item.URL,
			Published: time.Unix(item.Datetime, 0).UTC(),
		})
	}
	return out, nil
}

type finnhubCalendarResponse struct {
	EconomicCalendar []struct {
		Country string `json:"country"`
		Event   string `json:"event"`
		Impact  string `json:"impact"`
		Time    string `json:"time"`
	} `json:"economicCalendar"`
}

func (f *Fetcher) fetchFinnhubCalendar(ctx context.Context) ([]CalendarEvent, error) {
	now := time.Now().UTC()
	from := now.Format("2006-01-02")
	to := now.AddDate(0, 0, 2).Format("2006-01-02")

	q := url.Values{}
	q.Set("from", from)
	q.Set("to", to)
	q.Set("token", f.finnhubKey)

	var resp finnhubCalendarResponse
	if err := f.getJSON(ctx, finnhubCalendarURL, q, &resp); err != nil {
		return nil, err
	}

	out := make([]CalendarEvent, 0, len(resp.EconomicCalendar))
	for _, e := range resp.EconomicCalendar {
		t, err := parseCalendarTime(e.Time)
		if err != nil {
			continue
		}
		out = append(out, CalendarEvent{
			Time:    t,
			Country: e.Country,
			Event:   e.Event,
			Impact:  strings.ToLower(e.Impact),
		})
	}
	return out, nil
}

func parseCalendarTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse calendar time %q", s)
}

func (f *Fetcher) fetchRSS(ctx context.Context, source, feedURL string, maxAge time.Duration) ([]Headline, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := f.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("rss status %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	feed, err := f.feed.Parse(res.Body)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	out := make([]Headline, 0, len(feed.Items))
	for _, item := range feed.Items {
		pub := item.PublishedParsed
		if pub == nil {
			pub = item.UpdatedParsed
		}
		if pub == nil || pub.Before(cutoff) {
			continue
		}
		summary := item.Description
		if item.Content != "" {
			summary = item.Content
		}
		out = append(out, Headline{
			Source:    source,
			Title:     item.Title,
			Summary:   summary,
			URL:       item.Link,
			Published: pub.UTC(),
		})
	}
	return out, nil
}

func (f *Fetcher) getJSON(ctx context.Context, base string, q url.Values, out any) error {
	u := base + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := f.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("status %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}
