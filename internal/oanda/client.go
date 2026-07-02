package oanda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	accountID string
	token     string
	http      *http.Client
}

func NewClient(baseURL, accountID, token string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		accountID: accountID,
		token:     token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) AccountSummary(ctx context.Context) (*AccountSummary, error) {
	path := fmt.Sprintf("/v3/accounts/%s/summary", c.accountID)
	var out AccountSummary
	if err := c.getJSON(ctx, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Pricing(ctx context.Context, instruments ...string) (*PricingResponse, error) {
	if len(instruments) == 0 {
		instruments = []string{DefaultInstrument}
	}
	path := fmt.Sprintf("/v3/accounts/%s/pricing", c.accountID)
	q := url.Values{}
	q.Set("instruments", strings.Join(instruments, ","))

	var out PricingResponse
	if err := c.getJSON(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResponse, error) {
	path := fmt.Sprintf("/v3/accounts/%s/orders", c.accountID)
	var out CreateOrderResponse
	if err := c.postJSON(ctx, path, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) OpenTrades(ctx context.Context) (*OpenTradesResponse, error) {
	path := fmt.Sprintf("/v3/accounts/%s/openTrades", c.accountID)
	var out OpenTradesResponse
	if err := c.getJSON(ctx, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) OpenPositions(ctx context.Context) (*OpenPositionsResponse, error) {
	path := fmt.Sprintf("/v3/accounts/%s/openPositions", c.accountID)
	var out OpenPositionsResponse
	if err := c.getJSON(ctx, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CloseTrade(ctx context.Context, tradeID string, units string) (*CloseTradeResponse, error) {
	if units == "" {
		units = "ALL"
	}
	path := fmt.Sprintf("/v3/accounts/%s/trades/%s/close", c.accountID, tradeID)
	var out CloseTradeResponse
	if err := c.putJSON(ctx, path, CloseTradeRequest{Units: units}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PendingOrders(ctx context.Context) (*PendingOrdersResponse, error) {
	path := fmt.Sprintf("/v3/accounts/%s/pendingOrders", c.accountID)
	var out PendingOrdersResponse
	if err := c.getJSON(ctx, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CancelOrder(ctx context.Context, orderID string) (*CancelOrderResponse, error) {
	path := fmt.Sprintf("/v3/accounts/%s/orders/%s/cancel", c.accountID, orderID)
	var out CancelOrderResponse
	if err := c.putJSON(ctx, path, struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Candles(ctx context.Context, inst, granularity string, count int) (*CandlesResponse, error) {
	if inst == "" {
		inst = DefaultInstrument
	}
	path := fmt.Sprintf("/v3/instruments/%s/candles", inst)
	q := url.Values{}
	q.Set("granularity", granularity)
	q.Set("price", "M")
	q.Set("count", fmt.Sprintf("%d", count))

	var out CandlesResponse
	if err := c.getJSON(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, nil, body, out)
}

func (c *Client) putJSON(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPut, path, nil, body, out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	u := c.baseURL + path
	if q != nil {
		u += "?" + q.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept-Datetime-Format", "RFC3339")
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("oanda %s %s: %s", res.Status, path, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if q != nil {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept-Datetime-Format", "RFC3339")

	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("oanda %s %s: %s", res.Status, path, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
