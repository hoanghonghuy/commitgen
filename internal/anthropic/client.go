package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

type Config struct {
	APIKey string
	Model  string
}

type Client struct {
	apiKey string
	model  string
	client *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
		client: &http.Client{},
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

func (c *Client) GenerateCommitMessage(ctx context.Context, msgs []vscodeprompt.VSCodeMessage, temperature float64) (string, error) {
	// Anthropic API uses a specific format:
	// System prompt is top-level.
	// Users/Assistants alternate.

	var systemPrompt string
	var anthropicMsgs []message

	for _, m := range msgs {
		role := "user"
		if m.Role == 3 { // System
			// Extract system prompt
			for _, part := range m.Content {
				systemPrompt += part.Text + "\n"
			}
			continue
		} else if m.Role == 2 { // Assistant
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
		MaxTokens: 1024,
		System:    strings.TrimSpace(systemPrompt),
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(body))
	}

	var msgResp messageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(msgResp.Content) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	return msgResp.Content[0].Text, nil
}
