package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/httpx"
	"github.com/hoanghonghuy/commitgen/internal/logger"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

type Config struct {
	APIKey string
	Model  string
}

const anthropicMaxTokens = 1024

type Client struct {
	apiKey string
	model  string
	client *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

type messageRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messageResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (c *Client) Generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	return c.generate(ctx, msgs, temperature)
}

func (c *Client) generate(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	// Anthropic API uses a specific format:
	// System prompt is top-level.
	// Users/Assistants alternate.

	var systemPrompt string
	var anthropicMsgs []message

	for _, m := range msgs {
		role := "user"
		if m.Role == vscodeprompt.RoleSystem {
			// Extract system prompt
			for _, part := range m.Content {
				systemPrompt += part.Text + "\n"
			}
			continue
		} else if m.Role == vscodeprompt.RoleAssistant {
			role = "assistant"
		}

		var contentBuilder strings.Builder
		for _, part := range m.Content {
			contentBuilder.WriteString(part.Text)
		}

		anthropicMsgs = append(anthropicMsgs, message{
			Role:    role,
			Content: contentBuilder.String(),
		})
	}

	reqBody := messageRequest{
		Model:     c.model,
		Messages:  anthropicMsgs,
		MaxTokens: anthropicMaxTokens,
		System:    strings.TrimSpace(systemPrompt),
	}

	headers := map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}

	var msgResp messageResponse
	if err := httpx.DoJSONRequest(ctx, c.client, "POST", "https://api.anthropic.com/v1/messages", headers, reqBody, &msgResp); err != nil {
		return "", logger.LogError(err, "anthropic: request failed")
	}

	if len(msgResp.Content) == 0 {
		return "", logger.LogError(fmt.Errorf("empty response"), "anthropic: no content")
	}

	return msgResp.Content[0].Text, nil
}
