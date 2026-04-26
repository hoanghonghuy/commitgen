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
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temp float64) (string, error) {
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

	b, _ := io.ReadAll(resp.Body)
	
	// Check if response is streaming (SSE format)
	responseStr := string(b)
	if strings.HasPrefix(responseStr, "data: ") {
		// Parse streaming response
		lines := strings.Split(responseStr, "\n")
		var content strings.Builder
		
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || line == "data: [DONE]" {
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				jsonStr := strings.TrimPrefix(line, "data: ")
				var chunk chatResp
				if err := json.Unmarshal([]byte(jsonStr), &chunk); err != nil {
					continue
				}
				if len(chunk.Choices) > 0 {
					// Streaming uses delta, non-streaming uses message
					if chunk.Choices[0].Delta.Content != "" {
						content.WriteString(chunk.Choices[0].Delta.Content)
					} else if chunk.Choices[0].Message.Content != "" {
						content.WriteString(chunk.Choices[0].Message.Content)
					}
				}
			}
		}
		
		result := content.String()
		if result == "" {
			return "", logger.LogError(fmt.Errorf("empty streaming response"), "openai: no content in response")
		}
		return result, nil
	}
	
	// Non-streaming response
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
		return "", logger.LogError(fmt.Errorf("empty choices"), "openai: no choices in response")
	}
	return out.Choices[0].Message.Content, nil
}
