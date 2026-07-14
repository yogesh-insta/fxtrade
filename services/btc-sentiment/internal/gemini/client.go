package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	DefaultModel   = "gemini-2.5-flash-lite"
	maxRetries     = 3
)

// Client talks to Gemini generateContent with optional function declarations.
type Client struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
	MaxTokens  int
}

func NewClient(apiKey, model string, httpClient *http.Client) *Client {
	if model == "" {
		model = DefaultModel
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    DefaultBaseURL,
		HTTPClient: httpClient,
		MaxTokens:  2048,
	}
}

func (c *Client) WithBaseURL(base string) *Client {
	if base != "" {
		c.BaseURL = strings.TrimRight(base, "/")
	}
	return c
}

// --- wire types (function calling) ---

type SchemaType string

const (
	TypeObject  SchemaType = "OBJECT"
	TypeString  SchemaType = "STRING"
	TypeNumber  SchemaType = "NUMBER"
	TypeInteger SchemaType = "INTEGER"
	TypeBoolean SchemaType = "BOOLEAN"
	TypeArray   SchemaType = "ARRAY"
)

type Schema struct {
	Type        SchemaType         `json:"type,omitempty"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
}

type FunctionDeclaration struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parameters  *Schema `json:"parameters,omitempty"`
}

type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations"`
}

type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text             string            `json:"text,omitempty"`
	Thought          bool              `json:"thought,omitempty"`
	ThoughtSignature string            `json:"thoughtSignature,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

type FunctionCall struct {
	ID   string         `json:"id,omitempty"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type FunctionResponse struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Response any    `json:"response"`
}

type GenerateRequest struct {
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Contents          []Content         `json:"contents"`
	Tools             []Tool            `json:"tools,omitempty"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
	ToolConfig        *ToolConfig       `json:"toolConfig,omitempty"`
}

type GenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type FunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // AUTO, ANY, NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type GenerateResponse struct {
	Candidates []Candidate `json:"candidates"`
	Error      *APIError   `json:"error,omitempty"`
}

type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason"`
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Generate sends one generateContent turn (may return text and/or function calls).
func (c *Client) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if c.APIKey == "" {
		return GenerateResponse{}, fmt.Errorf("gemini api key empty")
	}
	if req.GenerationConfig == nil {
		req.GenerationConfig = &GenerationConfig{Temperature: 0, MaxOutputTokens: c.MaxTokens}
	}

	raw, err := json.Marshal(req)
	if err != nil {
		return GenerateResponse{}, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimRight(c.BaseURL, "/"), c.Model)
	var out GenerateResponse
	var lastErr error

	operation := func() error {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return backoff.Permanent(err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-goog-api-key", c.APIKey)

		resp, err := c.HTTPClient.Do(httpReq)
		if err != nil {
			lastErr = err
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			lastErr = err
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if wait := retryAfterDelay(resp.Header.Get("Retry-After")); wait > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(wait):
				}
			}
			lastErr = fmt.Errorf("gemini status %s: %s", resp.Status, truncate(string(body), 300))
			return lastErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("gemini status %s: %s", resp.Status, truncate(string(body), 300))
			return backoff.Permanent(lastErr)
		}

		if err := json.Unmarshal(body, &out); err != nil {
			lastErr = fmt.Errorf("gemini decode: %w", err)
			return backoff.Permanent(lastErr)
		}
		if out.Error != nil {
			lastErr = fmt.Errorf("gemini api error: %s", out.Error.Message)
			return backoff.Permanent(lastErr)
		}
		return nil
	}

	bo := backoff.WithMaxRetries(backoff.NewExponentialBackOff(), uint64(maxRetries))
	bo = backoff.WithContext(bo, ctx)
	if err := backoff.Retry(operation, bo); err != nil {
		if lastErr != nil {
			return GenerateResponse{}, lastErr
		}
		return GenerateResponse{}, err
	}
	return out, nil
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

// FunctionCalls extracts functionCall parts from the first candidate.
func (r GenerateResponse) FunctionCalls() []Part {
	if len(r.Candidates) == 0 {
		return nil
	}
	var out []Part
	for _, p := range r.Candidates[0].Content.Parts {
		if p.FunctionCall != nil {
			out = append(out, p)
		}
	}
	return out
}

// ModelContent returns the model candidate content (for history).
func (r GenerateResponse) ModelContent() (Content, bool) {
	if len(r.Candidates) == 0 {
		return Content{}, false
	}
	c := r.Candidates[0].Content
	if c.Role == "" {
		c.Role = "model"
	}
	return c, true
}
