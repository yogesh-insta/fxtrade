package fetchers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const maxRetries = 3

// HTTPDo performs an HTTP request with exponential backoff (max 3 retries),
// respecting Retry-After headers on 429/5xx responses.
func HTTPDo(ctx context.Context, client *http.Client, method, url string, body []byte, setHeaders func(*http.Request)) ([]byte, int, error) {
	if client == nil {
		client = http.DefaultClient
	}

	var lastBody []byte
	var lastStatus int
	var lastErr error

	operation := func() error {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return backoff.Permanent(err)
		}
		if setHeaders != nil {
			setHeaders(req)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			return err
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			lastErr = err
			return err
		}
		lastBody = respBody
		lastStatus = resp.StatusCode

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if wait := retryAfterDelay(resp.Header.Get("Retry-After")); wait > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
			return lastErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
			return backoff.Permanent(lastErr)
		}
		return nil
	}

	bo := backoff.WithMaxRetries(backoff.NewExponentialBackOff(), uint64(maxRetries))
	bo = backoff.WithContext(bo, ctx)
	if err := backoff.Retry(operation, bo); err != nil {
		if lastErr != nil {
			return lastBody, lastStatus, lastErr
		}
		return lastBody, lastStatus, err
	}
	return lastBody, lastStatus, nil
}

func retryAfterDelay(header string) time.Duration {
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
