// Package gemini calls the Gemini API with Google Search grounding.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// DefaultModel is the cheapest Gemini model that supports Google Search grounding.
const DefaultModel = "gemini-2.5-flash-lite"

// Client performs grounded generateContent requests against the Gemini API.
type Client struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Gemini API client.
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// WithHTTPClient overrides the HTTP client (for tests).
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	if h != nil {
		c.httpClient = h
	}
	return c
}

// WithBaseURL overrides the API base URL (for tests).
func (c *Client) WithBaseURL(base string) *Client {
	if base != "" {
		c.baseURL = strings.TrimRight(base, "/")
	}
	return c
}

type generateRequest struct {
	SystemInstruction *content   `json:"systemInstruction,omitempty"`
	Contents          []content  `json:"contents"`
	Tools             []toolSpec `json:"tools,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type toolSpec struct {
	GoogleSearch struct{} `json:"google_search"`
}

type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *apiError `json:"error,omitempty"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// GenerateGrounded runs a Google Search–grounded completion.
func (c *Client) GenerateGrounded(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	systemPrompt = strings.TrimSpace(systemPrompt)
	userPrompt = strings.TrimSpace(userPrompt)
	if userPrompt == "" {
		return "", fmt.Errorf("gemini: empty user prompt")
	}

	reqBody := generateRequest{
		Contents: []content{{
			Role:  "user",
			Parts: []part{{Text: userPrompt}},
		}},
		Tools: []toolSpec{{}},
	}
	if systemPrompt != "" {
		reqBody.SystemInstruction = &content{
			Parts: []part{{Text: systemPrompt}},
		}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("gemini status %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	var out generateResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("gemini decode: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("gemini api error: %s", out.Error.Message)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}

	text := strings.TrimSpace(out.Candidates[0].Content.Parts[0].Text)
	if text == "" {
		return "", fmt.Errorf("gemini returned empty text")
	}
	return text, nil
}
