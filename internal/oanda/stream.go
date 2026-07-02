package oanda

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Stream struct {
	baseURL   string
	accountID string
	token     string
	http      *http.Client
}

func NewStream(baseURL, accountID, token string) *Stream {
	return &Stream{
		baseURL:   strings.TrimRight(baseURL, "/"),
		accountID: accountID,
		token:     token,
		http: &http.Client{
			Timeout: 0,
		},
	}
}

func (s *Stream) RunPricingStream(ctx context.Context, instruments []string, out chan<- PriceUpdate) error {
	if len(instruments) == 0 {
		instruments = []string{DefaultInstrument}
	}

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := s.streamOnce(ctx, instruments, out)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		slog.Warn("pricing stream disconnected", "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *Stream) streamOnce(ctx context.Context, instruments []string, out chan<- PriceUpdate) error {
	url := fmt.Sprintf("%s/v3/accounts/%s/pricing/stream?instruments=%s",
		s.baseURL, s.accountID, strings.Join(instruments, ","))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept-Datetime-Format", "RFC3339")

	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("stream status %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var typ struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &typ); err != nil {
			slog.Warn("stream decode type", "error", err)
			continue
		}

		switch typ.Type {
		case "HEARTBEAT":
			continue
		case "PRICE":
			var p ClientPrice
			if err := json.Unmarshal([]byte(line), &p); err != nil {
				slog.Warn("stream decode price", "error", err)
				continue
			}
			tick, err := p.ToUpdate()
			if err != nil {
				slog.Warn("stream price update", "error", err)
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- tick:
			}
		default:
			slog.Debug("stream message", "type", typ.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return fmt.Errorf("stream ended")
}
