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
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	Contents          []content         `json:"contents"`
	Tools             []toolSpec        `json:"tools,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

type generationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text             string `json:"text,omitempty"`
	Thought          bool   `json:"thought,omitempty"`
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}

type toolSpec struct {
	GoogleSearch struct{} `json:"google_search"`
}

type generateResponse struct {
	Candidates []candidate `json:"candidates"`
	Error      *apiError   `json:"error,omitempty"`
}

type candidate struct {
	Content       content `json:"content"`
	FinishReason  string  `json:"finishReason"`
	FinishMessage string  `json:"finishMessage"`
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
		GenerationConfig: &generationConfig{
			Temperature:     0,
			MaxOutputTokens: 4096,
		},
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
	if len(out.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned no candidates")
	}

	text, err := extractResponseText(out.Candidates[0])
	if err != nil {
		return "", err
	}
	return text, nil
}

// extractResponseText collects non-thought text parts; prefers the last JSON-looking block.
func extractResponseText(c candidate) (string, error) {
	parts := c.Content.Parts
	if len(parts) == 0 {
		reason := strings.TrimSpace(c.FinishReason)
		if reason == "" {
			reason = "unknown"
		}
		msg := strings.TrimSpace(c.FinishMessage)
		if msg != "" {
			return "", fmt.Errorf("gemini returned no parts (finishReason=%s: %s)", reason, msg)
		}
		return "", fmt.Errorf("gemini returned no parts (finishReason=%s)", reason)
	}

	var texts []string
	for _, p := range parts {
		if p.Thought {
			continue
		}
		if t := strings.TrimSpace(p.Text); t != "" {
			texts = append(texts, t)
		}
	}
	// If every part was thought-only, fall back to any text-bearing part.
	if len(texts) == 0 {
		for _, p := range parts {
			if t := strings.TrimSpace(p.Text); t != "" {
				texts = append(texts, t)
			}
		}
	}
	if len(texts) == 0 {
		reason := strings.TrimSpace(c.FinishReason)
		if reason == "" {
			reason = "unknown"
		}
		return "", fmt.Errorf("gemini returned empty text (finishReason=%s)", reason)
	}

	for i := len(texts) - 1; i >= 0; i-- {
		if strings.Contains(texts[i], "{") {
			return texts[i], nil
		}
	}
	return texts[len(texts)-1], nil
}
