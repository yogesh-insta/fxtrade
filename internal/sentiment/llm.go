package sentiment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/config"
)

type LLMClient struct {
	cfg  config.LLMConfig
	http *http.Client
}

func NewLLMClient(cfg config.LLMConfig) *LLMClient {
	return &LLMClient{
		cfg: cfg,
		http: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *LLMClient) Analyze(ctx context.Context, systemPrompt string, payload []byte) (SentimentSignal, error) {
	reqBody := chatRequest{
		Model:       c.cfg.Model,
		Temperature: 0,
		ResponseFormat: &responseFormat{Type: "json_object"},
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(payload)},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return SentimentSignal{}, err
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return SentimentSignal{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return SentimentSignal{}, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return SentimentSignal{}, err
	}
	if res.StatusCode >= 300 {
		return SentimentSignal{}, fmt.Errorf("llm status %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return SentimentSignal{}, err
	}
	if len(out.Choices) == 0 {
		return SentimentSignal{}, fmt.Errorf("llm returned no choices")
	}

	content := strings.TrimSpace(out.Choices[0].Message.Content)
	var signal SentimentSignal
	if err := json.Unmarshal([]byte(content), &signal); err != nil {
		return SentimentSignal{}, fmt.Errorf("parse llm json: %w (content=%s)", err, truncate(content, 200))
	}

	signal.AnalyzedAt = time.Now().UTC()
	if signal.ValidMinutes <= 0 {
		signal.ValidMinutes = 30
	}
	signal.Confidence = normalizeConfidence(signal.Confidence)
	return signal, nil
}

func normalizeConfidence(v float64) float64 {
	if v > 1 {
		if v <= 100 {
			return v / 100
		}
		return 1
	}
	if v < 0 {
		return 0
	}
	return v
}
