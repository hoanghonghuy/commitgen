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

	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
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
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type chatReq struct {
	Model       string                       `json:"model"`
	Messages    []vscodeprompt.OpenAIMessage `json:"messages"`
	Temperature float64                      `json:"temperature,omitempty"`
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

func (c *Client) GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error) {
	logger.Debug("openai: generating commit message", "model", c.cfg.Model, "temperature", temp)
	oaiMsgs := vscodeprompt.ToOpenAIMessages(msgs)

	base := strings.TrimRight(c.cfg.BaseURL, "/")
	url := base + "/chat/completions"

	payload, _ := json.Marshal(chatReq{
		Model:       c.cfg.Model,
		Messages:    oaiMsgs,
		Temperature: temp,
	})

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", logger.LogError(err, "openai: request failed", "url", url)
	}
	defer resp.Body.Close()

	logger.Debug("openai: received response", "status", resp.StatusCode)

	b, _ := io.ReadAll(resp.Body)
	var out chatResp
	if err := json.Unmarshal(b, &out); err != nil {
		logger.Error("openai: decode error", "error", err, "response", string(b))
		return "", fmt.Errorf("decode error: %v\nraw: %s", err, string(b))
	}
	if out.Error != nil {
		logger.Error("openai: API error", "message", out.Error.Message, "type", out.Error.Type)
		return "", fmt.Errorf("llm error: %s (%s)", out.Error.Message, out.Error.Type)
	}
	if len(out.Choices) == 0 {
		logger.Error("openai: empty choices")
		return "", fmt.Errorf("llm: empty choices")
	}
	logger.Info("openai: commit message generated successfully")
	return out.Choices[0].Message.Content, nil
}
