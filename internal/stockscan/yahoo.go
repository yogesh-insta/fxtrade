package stockscan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const yahooChartURL = "https://query1.finance.yahoo.com/v8/finance/chart/"

// Bar is one daily OHLCV candle.
type Bar struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// YahooClient fetches NSE daily candles via Yahoo Finance.
type YahooClient struct {
	http      *http.Client
	rateLimit time.Duration
	mu        sync.Mutex
	lastReq   time.Time
	userAgent string
}

func NewYahooClient(requestTimeout time.Duration, rateLimit time.Duration) *YahooClient {
	if requestTimeout <= 0 {
		requestTimeout = 15 * time.Second
	}
	if rateLimit <= 0 {
		rateLimit = 300 * time.Millisecond
	}
	return &YahooClient{
		http:      &http.Client{Timeout: requestTimeout},
		rateLimit: rateLimit,
		userAgent: "fxtrade-stockscan/1.0",
	}
}

// QuoteHistory is daily bars plus Yahoo's company name for a symbol.
type QuoteHistory struct {
	Symbol string
	Name   string // longName preferred, else shortName
	Bars   []Bar
}

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol    string `json:"symbol"`
				ShortName string `json:"shortName"`
				LongName  string `json:"longName"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

func NSESymbol(symbol string) string {
	symbol = strings.TrimSpace(strings.ToUpper(symbol))
	if strings.HasSuffix(symbol, ".NS") {
		return symbol
	}
	return symbol + ".NS"
}

func (c *YahooClient) FetchDaily(ctx context.Context, symbol string) (QuoteHistory, error) {
	out := QuoteHistory{Symbol: strings.TrimSpace(strings.ToUpper(symbol))}
	if err := c.waitRateLimit(ctx); err != nil {
		return out, err
	}

	yahooSym := NSESymbol(symbol)
	url := fmt.Sprintf("%s%s?range=90d&interval=1d", yahooChartURL, yahooSym)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return out, err
	}
	if res.StatusCode >= 300 {
		return out, fmt.Errorf("yahoo %s: status %s: %s", yahooSym, res.Status, strings.TrimSpace(string(body)))
	}

	var parsed chartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return out, fmt.Errorf("yahoo %s: parse: %w", yahooSym, err)
	}
	if parsed.Chart.Error != nil {
		return out, fmt.Errorf("yahoo %s: %s", yahooSym, parsed.Chart.Error.Description)
	}
	if len(parsed.Chart.Result) == 0 {
		return out, fmt.Errorf("yahoo %s: empty result", yahooSym)
	}

	result := parsed.Chart.Result[0]
	out.Name = companyNameFromMeta(result.Meta.LongName, result.Meta.ShortName)
	if len(result.Indicators.Quote) == 0 {
		return out, fmt.Errorf("yahoo %s: missing quote", yahooSym)
	}
	q := result.Indicators.Quote[0]
	n := len(result.Timestamp)
	if n == 0 {
		return out, fmt.Errorf("yahoo %s: no timestamps", yahooSym)
	}

	bars := make([]Bar, 0, n)
	for i := 0; i < n; i++ {
		closeVal := derefFloat(q.Close, i)
		if closeVal == nil {
			continue
		}
		bar := Bar{
			Date:  time.Unix(result.Timestamp[i], 0).UTC(),
			Close: *closeVal,
		}
		if v := derefFloat(q.Open, i); v != nil {
			bar.Open = *v
		}
		if v := derefFloat(q.High, i); v != nil {
			bar.High = *v
		}
		if v := derefFloat(q.Low, i); v != nil {
			bar.Low = *v
		}
		if v := derefInt(q.Volume, i); v != nil {
			bar.Volume = *v
		}
		bars = append(bars, bar)
	}
	if len(bars) == 0 {
		return out, fmt.Errorf("yahoo %s: no valid bars", yahooSym)
	}
	out.Bars = bars
	return out, nil
}

func companyNameFromMeta(longName, shortName string) string {
	if n := strings.TrimSpace(longName); n != "" {
		return n
	}
	return strings.TrimSpace(shortName)
}

func (c *YahooClient) waitRateLimit(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.lastReq.IsZero() {
		wait := c.rateLimit - time.Since(c.lastReq)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	c.lastReq = time.Now()
	return nil
}

func derefFloat(values []*float64, i int) *float64 {
	if i >= len(values) {
		return nil
	}
	return values[i]
}

func derefInt(values []*int64, i int) *int64 {
	if i >= len(values) {
		return nil
	}
	return values[i]
}
