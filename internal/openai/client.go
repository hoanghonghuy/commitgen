package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"commitgen/internal/vscodeprompt"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type ChatRequest struct {
	Messages    []vscodeprompt.OpenAIMessage
	Temperature float64
}

type chatReq struct {
	Model       string                     `json:"model"`
	Messages    []vscodeprompt.OpenAIMessage `json:"messages"`
	Temperature float64                    `json:"temperature,omitempty"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) Chat(ctx context.Context, req ChatRequest) (string, error) {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	url := base + "/chat/completions"

	payload, _ := json.Marshal(chatReq{
		Model:       c.cfg.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
	})

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	var out chatResp
	if err := json.Unmarshal(b, &out); err != nil {
		return "", fmt.Errorf("decode error: %v\nraw: %s", err, string(b))
	}
	if out.Error != nil {
		return "", fmt.Errorf("llm error: %s (%s)", out.Error.Message, out.Error.Type)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm: empty choices")
	}
	return out.Choices[0].Message.Content, nil
}
